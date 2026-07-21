package xsteg

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"req/internal/ctx"
)

type fakeRunner struct {
	available     map[string]bool
	calls         []fakeCall
	wordlist      string
	seedCandidate bool
}

type fakeCall struct {
	name string
	args []string
}

func (runner *fakeRunner) LookPath(name string) (string, error) {
	if runner.available[name] {
		return "/usr/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeRunner) Run(_ context.Context, name string, args []string, stdin io.Reader, stdout, _ io.Writer) error {
	runner.calls = append(runner.calls, fakeCall{name: name, args: append([]string(nil), args...)})
	switch name {
	case "file":
		_, _ = io.WriteString(stdout, "image/jpeg\n")
	case "exiftool":
		_, _ = io.WriteString(stdout, "[]\n")
	case "binwalk":
		_, _ = io.WriteString(stdout, "DECIMAL HEXADECIMAL DESCRIPTION\n0 0x0 JPEG image data\n")
	case "strings":
		_, _ = io.WriteString(stdout, "visible string\n")
	case "steghide":
		if len(args) > 0 && args[0] == "extract" {
			password := ""
			for index, argument := range args {
				if argument == "-p" && index+1 < len(args) {
					password = args[index+1]
				}
			}
			if password != "hunter2" {
				return errors.New("wrong passphrase")
			}
			for index, argument := range args {
				if argument == "-xf" && index+1 < len(args) {
					return os.WriteFile(args[index+1], []byte("manual secret"), 0600)
				}
			}
		}
		if len(args) > 0 && args[0] == "info" {
			for index, argument := range args {
				if argument == "-p" && index+1 < len(args) && args[index+1] == "hunter2" {
					_, _ = io.WriteString(stdout, "embedded file \"creds.txt\"")
					return nil
				}
			}
			_, _ = io.WriteString(stdout, "could not extract any data with that passphrase")
			return errors.New("exit status 1")
		}
		_, _ = io.WriteString(stdout, "embedded file secret.txt\n")
	case "stegseek":
		if len(args) > 0 && args[0] == "--seed" {
			if runner.seedCandidate {
				_, _ = io.WriteString(stdout, "Found (possible) seed: \"12345678\"\n")
				return nil
			}
			_, _ = io.WriteString(stdout, "Could not find a valid seed.\n")
			return errors.New("exit status 1")
		}
		if len(args) >= 4 && args[2] == runner.wordlist {
			if err := os.WriteFile(args[3], []byte("secret"), 0600); err != nil {
				return err
			}
			_, _ = io.WriteString(stdout, "Found passphrase: \"hunter2\"\n")
			return nil
		}
		_, _ = io.WriteString(stdout, "Could not find a valid passphrase.\n")
		return errors.New("exit status 1")
	}
	return nil
}

func TestParseOptionsDefaultsToList(t *testing.T) {
	parsed, err := parseOptions([]string{"xsteg"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "ls" || parsed.Path != "." {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseExtractOptions(t *testing.T) {
	parsed, err := parseOptions([]string{"xsteg", "extract", "image.jpg", "--wordlist", "passwords.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "extract" || parsed.Path != "image.jpg" || parsed.Wordlist != "passwords.txt" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if _, err := parseOptions([]string{"xsteg", "scan", "image.jpg", "--no-crack"}); err == nil {
		t.Fatal("scan accepted extract-only option")
	}
	if _, err := parseOptions([]string{"xsteg", "extract", "image.jpg", "--auto", "--manual"}); err == nil {
		t.Fatal("extract accepted conflicting modes")
	}
	if _, err := parseOptions([]string{"xsteg", "extract", "image.jpg", "--manual", "--wordlist", "passwords.txt"}); err == nil {
		t.Fatal("manual mode accepted a wordlist")
	}
}

func TestChoosePassphraseMode(t *testing.T) {
	var output strings.Builder
	app := New(&fakeRunner{}, strings.NewReader("2\nhunter2\n"), &output, io.Discard)
	parsed := options{Command: "extract"}
	if err := app.choosePassphraseMode(&parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Mode != "manual" || parsed.Password != "hunter2" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if !strings.Contains(output.String(), "Auto") || !strings.Contains(output.String(), "Manual") {
		t.Fatalf("prompt = %q", output.String())
	}
}

func TestMIMERoutingDoesNotRequireExtension(t *testing.T) {
	if !supportsSteghide("payload", "image/jpeg") {
		t.Fatal("JPEG MIME was not routed to steghide")
	}
	if !supportsZsteg("payload", "image/png") {
		t.Fatal("PNG MIME was not routed to zsteg")
	}
}

func TestExtractReportIsReusedOnlyForAttemptedManualWordlist(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.txt")
	second := filepath.Join(directory, "second.txt")
	report := &Report{Version: reportSchemaVersion, Mode: "extract", Status: "findings", Wordlists: []WordlistRun{{Path: first, Status: "exhausted"}}}
	if !canReuseReport(report, options{Command: "extract", Wordlist: first}) {
		t.Fatal("attempted manual wordlist was not reusable")
	}
	if canReuseReport(report, options{Command: "extract", Wordlist: second}) {
		t.Fatal("untried manual wordlist reused an old report")
	}
}

func TestExtractUsesWordlistAndCreatesReport(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "image.jpg")
	if err := os.WriteFile(sourcePath, append([]byte{0xff, 0xd8, 0xff}, []byte("image")...), 0600); err != nil {
		t.Fatal(err)
	}
	wordlist := filepath.Join(directory, "passwords.txt")
	if err := os.WriteFile(wordlist, []byte("hunter2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		available:     map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true},
		wordlist:      wordlist,
		seedCandidate: true,
	}
	var stdout, stderr strings.Builder
	app := New(runner, strings.NewReader(""), &stdout, &stderr)
	if err := app.run([]string{"xsteg", "scan", sourcePath}); err != nil {
		t.Fatal(err)
	}
	if err := app.run([]string{"xsteg", "extract", sourcePath, "--wordlist", wordlist}); err != nil {
		t.Fatalf("run() error = %v, stderr = %q", err, stderr.String())
	}
	report, err := loadReport(filepath.Join(sourcePath+".xsteg", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "extracted" || report.Mode != "extract" {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Findings) == 0 {
		t.Fatal("no findings saved")
	}
	found := false
	for _, finding := range report.Findings {
		if finding.Backend == "stegseek" && finding.Password == "hunter2" {
			found = true
			if content, err := os.ReadFile(finding.Path); err != nil || string(content) != "secret" {
				t.Fatalf("payload = %q, error = %v", content, err)
			}
		}
	}
	if !found {
		t.Fatalf("findings = %+v", report.Findings)
	}
	if !strings.Contains(stdout.String(), "password: hunter2") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestExtractUsesCtxRecommendedWordlists(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "image.jpg")
	if err := os.WriteFile(sourcePath, []byte{0xff, 0xd8, 0xff, 0x00}, 0600); err != nil {
		t.Fatal(err)
	}
	wordlist := filepath.Join(directory, "recommended.txt")
	if err := os.WriteFile(wordlist, []byte("password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	original := recommendWordlists
	recommendWordlists = func(kind string) ([]ctx.WordlistSelection, error) {
		if kind != ctx.WordlistKindPassword {
			t.Fatalf("kind = %q", kind)
		}
		return []ctx.WordlistSelection{{Path: wordlist}}, nil
	}
	t.Cleanup(func() { recommendWordlists = original })
	runner := &fakeRunner{available: map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true}, wordlist: wordlist, seedCandidate: true}
	app := New(runner, strings.NewReader(""), io.Discard, io.Discard)
	if err := app.run([]string{"xsteg", "scan", sourcePath}); err != nil {
		t.Fatal(err)
	}
	if err := app.run([]string{"xsteg", "extract", sourcePath, "--auto"}); err != nil {
		t.Fatal(err)
	}
	used := false
	for _, call := range runner.calls {
		if call.name == "stegseek" && len(call.args) >= 3 && call.args[2] == wordlist {
			used = true
		}
	}
	if !used {
		t.Fatalf("calls = %+v", runner.calls)
	}
}

func TestManualExtractionUsesEnteredPassphrase(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "image.jpg")
	if err := os.WriteFile(sourcePath, []byte{0xff, 0xd8, 0xff, 0x00}, 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{available: map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true}, seedCandidate: true}
	app := New(runner, strings.NewReader("hunter2\n"), io.Discard, io.Discard)
	if err := app.run([]string{"xsteg", "scan", sourcePath}); err != nil {
		t.Fatal(err)
	}
	if err := app.run([]string{"xsteg", "extract", sourcePath, "--manual"}); err != nil {
		t.Fatal(err)
	}
	report, err := loadReport(filepath.Join(sourcePath+".xsteg", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var extracted *Finding
	for index := range report.Findings {
		if report.Findings[index].Kind == "extracted" {
			extracted = &report.Findings[index]
		}
	}
	if extracted == nil || extracted.OriginalName != "creds.txt" || filepath.Base(extracted.Path) != "creds.txt" {
		t.Fatalf("findings = %+v", report.Findings)
	}
	content, err := os.ReadFile(extracted.Path)
	if err != nil || string(content) != "manual secret" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}

func TestWrongManualPassphraseFails(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "image.jpg")
	if err := os.WriteFile(sourcePath, []byte{0xff, 0xd8, 0xff, 0x00}, 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{available: map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true}, seedCandidate: true}
	var stdout, stderr strings.Builder
	app := New(runner, strings.NewReader("wrong-password\n"), &stdout, &stderr)
	if err := app.run([]string{"xsteg", "scan", sourcePath}); err != nil {
		t.Fatal(err)
	}
	err := app.run([]string{"xsteg", "extract", sourcePath, "--manual"})
	if err == nil || !strings.Contains(err.Error(), "passphrase was rejected") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(stdout.String(), "COMPLETE") || strings.Contains(stdout.String(), "EXTRACTED") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	report, loadErr := loadReport(filepath.Join(sourcePath+".xsteg", "report.json"))
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if report.Status != "failed" || hasExtractedFinding(report) {
		t.Fatalf("report = %+v", report)
	}
}

func TestExtractPromptsOnlyForDetectedProtectedPayload(t *testing.T) {
	directory := t.TempDir()
	plainPath := filepath.Join(directory, "plain.jpg")
	if err := os.WriteFile(plainPath, []byte{0xff, 0xd8, 0xff, 0x00}, 0600); err != nil {
		t.Fatal(err)
	}
	available := map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true}
	var plainOutput strings.Builder
	if err := New(&fakeRunner{available: available}, strings.NewReader(""), &plainOutput, io.Discard).run([]string{"xsteg", "scan", plainPath}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plainOutput.String(), "Passphrase analysis") {
		t.Fatalf("plain file prompted: %q", plainOutput.String())
	}
	report, err := loadReport(filepath.Join(plainPath+".xsteg", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "no-findings" {
		t.Fatalf("status = %q", report.Status)
	}

	protectedPath := filepath.Join(directory, "protected.jpg")
	if err := os.WriteFile(protectedPath, []byte{0xff, 0xd8, 0xff, 0x01}, 0600); err != nil {
		t.Fatal(err)
	}
	protectedRunner := &fakeRunner{available: available, seedCandidate: true}
	if err := New(protectedRunner, strings.NewReader(""), io.Discard, io.Discard).run([]string{"xsteg", "scan", protectedPath}); err != nil {
		t.Fatal(err)
	}
	var protectedOutput strings.Builder
	if err := New(protectedRunner, strings.NewReader("3\n"), &protectedOutput, io.Discard).run([]string{"xsteg", "extract", protectedPath}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(protectedOutput.String(), "Passphrase analysis") || !strings.Contains(protectedOutput.String(), "Skip") {
		t.Fatalf("protected file did not prompt: %q", protectedOutput.String())
	}
	report, err = loadReport(filepath.Join(protectedPath+".xsteg", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "skipped" {
		t.Fatalf("status = %q", report.Status)
	}
}

func TestExtractRequiresScan(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "assumed.jpg")
	if err := os.WriteFile(sourcePath, []byte{0xff, 0xd8, 0xff, 0x03}, 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		available:     map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true},
		seedCandidate: true,
	}
	err := New(runner, strings.NewReader(""), io.Discard, io.Discard).run([]string{"xsteg", "extract", sourcePath})
	if err == nil || !strings.Contains(err.Error(), "no completed scan found") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(sourcePath + ".xsteg"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("extract created output directory: %v", statErr)
	}
	for _, call := range runner.calls {
		t.Fatalf("extract ran a tool without a scan: %+v", call)
	}
}

func TestScanReusesOrphanEmptyOutputDirectory(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "orphan.jpg")
	if err := os.WriteFile(sourcePath, []byte{0xff, 0xd8, 0xff, 0x04}, 0600); err != nil {
		t.Fatal(err)
	}
	outputRoot := sourcePath + ".xsteg"
	if err := os.Mkdir(outputRoot, 0700); err != nil {
		t.Fatal(err)
	}
	digest, err := hashFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	resolved, report, err := resolveOutputRoot(sourcePath, digest)
	if err != nil {
		t.Fatal(err)
	}
	if report != nil || resolved != outputRoot {
		t.Fatalf("resolved = %q, report = %+v", resolved, report)
	}
}

func TestExtractReusesCompletedScanDetection(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "protected.jpg")
	if err := os.WriteFile(sourcePath, []byte{0xff, 0xd8, 0xff, 0x02}, 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		available:     map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true},
		seedCandidate: true,
	}
	if err := New(runner, strings.NewReader(""), io.Discard, io.Discard).run([]string{"xsteg", "scan", sourcePath}); err != nil {
		t.Fatal(err)
	}
	if err := New(runner, strings.NewReader("3\n"), io.Discard, io.Discard).run([]string{"xsteg", "extract", sourcePath}); err != nil {
		t.Fatal(err)
	}
	seedRuns := 0
	for _, call := range runner.calls {
		if call.name == "stegseek" && len(call.args) > 0 && call.args[0] == "--seed" {
			seedRuns++
		}
	}
	if seedRuns != 1 {
		t.Fatalf("seed runs = %d, calls = %+v", seedRuns, runner.calls)
	}
}

func TestEmbeddedNameIsSanitized(t *testing.T) {
	if got := sanitizeEmbeddedName("../../creds.txt"); got != "creds.txt" {
		t.Fatalf("name = %q", got)
	}
	if got := parseEmbeddedName("Original filename: \"secret.txt\""); got != "secret.txt" {
		t.Fatalf("parsed name = %q", got)
	}
}

func TestBinwalkIgnoresJPEGExifTIFF(t *testing.T) {
	output := "DECIMAL HEXADECIMAL DESCRIPTION\n0 0x0 JPEG image data, EXIF standard\n12 0xC TIFF image data, big-endian, offset of first image directory: 8\n"
	if hasEmbeddedBinwalkOffset(output) {
		t.Fatal("JPEG EXIF TIFF was reported as an embedded file")
	}
}

func TestBinwalkFindsNonNestedSignature(t *testing.T) {
	output := "DECIMAL HEXADECIMAL DESCRIPTION\n0 0x0 JPEG image data\n4096 0x1000 Zip archive data\n"
	if !hasEmbeddedBinwalkOffset(output) {
		t.Fatal("non-nested ZIP signature was not reported")
	}
}

func TestScanDoesNotRunExtractionBackends(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "note.txt")
	if err := os.WriteFile(sourcePath, []byte("plain text"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{available: map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "stegseek": true, "stegsnow": true}}
	if err := New(runner, strings.NewReader(""), io.Discard, io.Discard).run([]string{"xsteg", "scan", sourcePath}); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if call.name == "stegseek" || call.name == "stegsnow" {
			t.Fatalf("scan ran extraction backend: %+v", call)
		}
	}
}

func TestListAndShowReports(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "image.jpg.xsteg")
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	report := Report{Version: 1, SourcePath: filepath.Join(directory, "image.jpg"), SourceSHA256: "abc", Mode: "scan", Status: "complete", OutputPath: output}
	if err := saveReport(&report); err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	app := New(&fakeRunner{}, strings.NewReader(""), &stdout, io.Discard)
	if err := app.run([]string{"xsteg", "ls", directory}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), report.SourcePath) {
		t.Fatalf("list = %q", stdout.String())
	}
	stdout.Reset()
	if err := app.run([]string{"xsteg", "show", "1", directory}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "SHA-256: abc") {
		t.Fatalf("show = %q", stdout.String())
	}
}

func TestCollectInputFilesSkipsReports(t *testing.T) {
	directory := t.TempDir()
	for _, path := range []string{filepath.Join(directory, "one.png"), filepath.Join(directory, "one.png.xsteg", "report.json")} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := collectInputFiles(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "one.png" {
		t.Fatalf("files = %#v", files)
	}
}

func TestSanitizeExtractedTreeRemovesSymlinks(t *testing.T) {
	directory := t.TempDir()
	regular := filepath.Join(directory, "safe.txt")
	if err := os.WriteFile(regular, []byte("safe"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "outside")
	if err := os.Symlink("/etc/passwd", link); err != nil {
		t.Fatal(err)
	}
	files, warnings := sanitizeExtractedTree(directory)
	if len(files) != 1 || files[0] != regular || len(warnings) == 0 {
		t.Fatalf("files = %#v, warnings = %#v", files, warnings)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe link remains: %v", err)
	}
}

func TestDoctorShowsOptionalBackends(t *testing.T) {
	runner := &fakeRunner{available: map[string]bool{"file": true, "exiftool": true, "binwalk": true, "strings": true, "steghide": true, "stegseek": true}}
	original := recommendWordlists
	recommendWordlists = func(string) ([]ctx.WordlistSelection, error) { return []ctx.WordlistSelection{{Path: "/words"}}, nil }
	t.Cleanup(func() { recommendWordlists = original })
	var output strings.Builder
	if err := New(runner, strings.NewReader(""), &output, io.Discard).run([]string{"xsteg", "doctor"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "zsteg     optional") || !strings.Contains(output.String(), "wordlists ready") {
		t.Fatalf("doctor = %q", output.String())
	}
}
