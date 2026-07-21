package xsteg

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
	"req/internal/ctx"
)

var Version = "1.0.0"

const (
	reportSchemaVersion = 4
	maxCapturedOutput   = 2 << 20
	maxExtractedFile    = 100 << 20
	maxExtractedFiles   = 100
)

const usageText = `usage: xsteg [ls [path]]
       xsteg scan <path>
       xsteg extract <path> [--auto | --manual] [--wordlist <file>] [--no-crack]
       xsteg show <ID> [path]
       xsteg doctor

Detect and safely extract data hidden in local files.

commands:
  ls [path]             list saved reports (default: current directory)
  scan <path>           analyze files without extracting payloads
  extract <path>        extract payloads from a completed scan
  show <ID> [path]      show a saved report
  doctor                show backend and wordlist availability

extract options:
  --auto                automatically analyze a detected protected payload
  --manual              prompt for a passphrase when a payload is detected
  -w, --wordlist        use one password wordlist instead of ctx recommendations
  --no-crack            do not run password wordlists

options:
  -h, --help            show this help
  -V, --version         show version`

type commandRunner interface {
	LookPath(string) (string, error)
	Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
}

type realRunner struct{}

func (realRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }
func (realRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	return command.Run()
}

type App struct {
	runner commandRunner
	stdin  io.Reader
	reader *bufio.Reader
	stdout io.Writer
	stderr io.Writer
}

type options struct {
	Command    string
	Path       string
	ID         int
	Wordlist   string
	NoCrack    bool
	Mode       string
	Password   string
	SearchRoot string
}

var recommendWordlists = ctx.RecommendWordlists

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return New(realRunner{}, stdin, stdout, stderr).run(args)
}

func New(runner commandRunner, stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{runner: runner, stdin: stdin, reader: bufio.NewReader(stdin), stdout: stdout, stderr: stderr}
}

func (app *App) run(args []string) error {
	parsed, err := parseOptions(args)
	if err != nil {
		return err
	}
	switch parsed.Command {
	case "help":
		_, err := fmt.Fprintln(app.stdout, usageText)
		return err
	case "version":
		_, err := fmt.Fprintf(app.stdout, "xsteg %s\n", Version)
		return err
	case "ls":
		return app.list(parsed.Path)
	case "show":
		return app.show(parsed.ID, parsed.SearchRoot)
	case "doctor":
		return app.doctor()
	case "scan", "extract":
		return app.processPath(parsed)
	default:
		return fmt.Errorf("unknown xsteg command: %s", parsed.Command)
	}
}

func parseOptions(args []string) (options, error) {
	if len(args) <= 1 {
		return options{Command: "ls", Path: "."}, nil
	}
	switch args[1] {
	case "-h", "--help", "help":
		if len(args) != 2 {
			return options{}, errors.New("usage: xsteg --help")
		}
		return options{Command: "help"}, nil
	case "-V", "--version", "version":
		if len(args) != 2 {
			return options{}, errors.New("usage: xsteg --version")
		}
		return options{Command: "version"}, nil
	case "doctor":
		if len(args) != 2 {
			return options{}, errors.New("usage: xsteg doctor")
		}
		return options{Command: "doctor"}, nil
	case "ls":
		if len(args) > 3 {
			return options{}, errors.New("usage: xsteg ls [path]")
		}
		path := "."
		if len(args) == 3 {
			path = args[2]
		}
		return options{Command: "ls", Path: path}, nil
	case "show":
		if len(args) < 3 || len(args) > 4 {
			return options{}, errors.New("usage: xsteg show <ID> [path]")
		}
		id, err := strconv.Atoi(args[2])
		if err != nil || id < 1 {
			return options{}, fmt.Errorf("invalid report ID: %s", args[2])
		}
		root := "."
		if len(args) == 4 {
			root = args[3]
		}
		return options{Command: "show", ID: id, SearchRoot: root}, nil
	case "scan", "extract":
		parsed := options{Command: args[1]}
		for index := 2; index < len(args); index++ {
			switch args[index] {
			case "-w", "--wordlist":
				if parsed.Command != "extract" || index+1 >= len(args) {
					return options{}, fmt.Errorf("usage: xsteg %s <path>%s", parsed.Command, extractUsageSuffix(parsed.Command))
				}
				index++
				parsed.Wordlist = args[index]
			case "--no-crack":
				if parsed.Command != "extract" {
					return options{}, errors.New("--no-crack is only available with extract")
				}
				parsed.NoCrack = true
			case "--auto":
				if parsed.Command != "extract" {
					return options{}, errors.New("--auto is only available with extract")
				}
				if parsed.Mode == "manual" {
					return options{}, errors.New("--auto and --manual cannot be used together")
				}
				parsed.Mode = "auto"
			case "--manual":
				if parsed.Command != "extract" {
					return options{}, errors.New("--manual is only available with extract")
				}
				if parsed.Mode == "auto" {
					return options{}, errors.New("--auto and --manual cannot be used together")
				}
				parsed.Mode = "manual"
			default:
				if strings.HasPrefix(args[index], "-") || parsed.Path != "" {
					return options{}, fmt.Errorf("usage: xsteg %s <path>%s", parsed.Command, extractUsageSuffix(parsed.Command))
				}
				parsed.Path = args[index]
			}
		}
		if parsed.Path == "" {
			return options{}, fmt.Errorf("usage: xsteg %s <path>%s", parsed.Command, extractUsageSuffix(parsed.Command))
		}
		if parsed.Mode == "manual" && (parsed.Wordlist != "" || parsed.NoCrack) {
			return options{}, errors.New("--manual cannot be used with --wordlist or --no-crack")
		}
		if parsed.Wordlist != "" || parsed.NoCrack {
			parsed.Mode = "auto"
		}
		return parsed, nil
	default:
		return options{}, fmt.Errorf("unknown xsteg command: %s", args[1])
	}
}

func (app *App) choosePassphraseMode(parsed *options) error {
	if parsed.Mode == "" {
		_, _ = fmt.Fprintln(app.stdout, "Passphrase analysis:")
		_, _ = fmt.Fprintln(app.stdout, "  1) Auto   - try ctx password wordlists")
		_, _ = fmt.Fprintln(app.stdout, "  2) Manual - enter a known passphrase")
		_, _ = fmt.Fprintln(app.stdout, "  3) Skip   - do not analyze this payload")
		_, _ = fmt.Fprint(app.stdout, "Select [1]: ")
		answer, err := app.reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to read passphrase mode: %w", err)
		}
		answer = strings.TrimSpace(answer)
		switch answer {
		case "", "1", "auto":
			parsed.Mode = "auto"
		case "2", "manual":
			parsed.Mode = "manual"
		case "3", "skip":
			parsed.Mode = "skip"
		default:
			return fmt.Errorf("invalid passphrase mode %q; select 1, 2, or 3", answer)
		}
		if errors.Is(err, io.EOF) && answer == "" {
			return errors.New("passphrase mode requires interactive input; use --auto or --manual")
		}
	}
	if parsed.Mode == "manual" {
		password, err := app.readPassword("Passphrase: ")
		if err != nil {
			return err
		}
		parsed.Password = password
	}
	return nil
}

func (app *App) readPassword(prompt string) (string, error) {
	_, _ = fmt.Fprint(app.stdout, prompt)
	if input, ok := app.stdin.(*os.File); ok && term.IsTerminal(input.Fd()) {
		password, err := term.ReadPassword(input.Fd())
		_, _ = fmt.Fprintln(app.stdout)
		if err != nil {
			return "", fmt.Errorf("failed to read passphrase: %w", err)
		}
		return string(password), nil
	}
	password, err := app.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("failed to read passphrase: %w", err)
	}
	if errors.Is(err, io.EOF) && password == "" {
		return "", errors.New("failed to read passphrase: input closed")
	}
	return strings.TrimRight(password, "\r\n"), nil
}

func extractUsageSuffix(command string) string {
	if command == "extract" {
		return " [--auto | --manual] [--wordlist <file>] [--no-crack]"
	}
	return ""
}

func (app *App) processPath(parsed options) error {
	inputs, err := collectInputFiles(parsed.Path)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no regular files found: %s", parsed.Path)
	}
	failed := 0
	var singleFailure string
	for _, input := range inputs {
		report, processErr := app.processFile(input, parsed)
		if processErr != nil {
			failed++
			message := fmt.Sprintf("[FAILED] %s\n  %v", input, processErr)
			if report != nil {
				message += fmt.Sprintf("\n  report: %s", filepath.Join(report.OutputPath, "report.json"))
			}
			if len(inputs) == 1 {
				singleFailure = message
			} else {
				_, _ = fmt.Fprintln(app.stderr, message)
			}
			continue
		}
		_, _ = fmt.Fprintf(app.stdout, "[%s] %s\n", strings.ToUpper(report.Status), report.SourcePath)
		for _, finding := range report.Findings {
			_, _ = fmt.Fprintf(app.stdout, "  %s: %s", finding.Backend, finding.Summary)
			if finding.OriginalName != "" {
				_, _ = fmt.Fprintf(app.stdout, " [%s]", finding.OriginalName)
			}
			if finding.Path != "" {
				_, _ = fmt.Fprintf(app.stdout, " -> %s", finding.Path)
			}
			if finding.Password != "" {
				_, _ = fmt.Fprintf(app.stdout, " (password: %s)", finding.Password)
			}
			_, _ = fmt.Fprintln(app.stdout)
		}
		_, _ = fmt.Fprintf(app.stdout, "  report: %s\n", filepath.Join(report.OutputPath, "report.json"))
	}
	if failed > 0 {
		if singleFailure != "" {
			return errors.New(singleFailure)
		}
		return fmt.Errorf("analysis failed for %d of %d files", failed, len(inputs))
	}
	return nil
}

func (app *App) processFile(path string, parsed options) (*Report, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	digest, err := hashFile(absolute)
	if err != nil {
		return nil, err
	}
	outputRoot, existing, err := resolveOutputRoot(absolute, digest)
	if err != nil {
		return nil, err
	}
	report := Report{
		Version:      reportSchemaVersion,
		SourcePath:   absolute,
		SourceSHA256: digest,
		Size:         info.Size(),
		Mode:         parsed.Command,
		Status:       "running",
		OutputPath:   outputRoot,
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	analysisReused := false
	if existing != nil {
		if existing.Version == reportSchemaVersion && parsed.Mode != "manual" {
			report.Wordlists = existing.Wordlists
		}
		if canReuseReport(existing, parsed) {
			return existing, nil
		}
		if parsed.Command == "extract" && canReuseAnalysis(existing) {
			report.MIME = existing.MIME
			report.ScanCompleted = true
			for _, tool := range existing.Tools {
				if isAnalysisTool(tool) {
					report.Tools = append(report.Tools, tool)
				}
			}
			for _, finding := range existing.Findings {
				if finding.Kind != "extracted" {
					report.Findings = append(report.Findings, finding)
				}
			}
			analysisReused = true
		}
	}
	if parsed.Command == "extract" && !analysisReused {
		return nil, fmt.Errorf("no completed scan found for %s; run xsteg scan %s first", absolute, absolute)
	}
	if err := os.MkdirAll(filepath.Join(outputRoot, "files"), 0700); err != nil {
		return nil, err
	}
	if err := saveReport(&report); err != nil {
		return nil, err
	}

	if parsed.Command == "scan" {
		report.MIME, _ = detectMIME(absolute)
		app.runAnalysisTools(&report)
		report.ScanCompleted = true
	} else if !analysisReused {
		report.MIME, _ = detectMIME(absolute)
	}
	if parsed.Command == "extract" {
		if err := app.runExtractionTools(&report, parsed); err != nil {
			report.Status = "failed"
			report.Warnings = append(report.Warnings, err.Error())
			report.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
			_ = saveReport(&report)
			return &report, err
		}
	}
	if report.Status == "running" {
		if hasExtractedFinding(&report) {
			report.Status = "extracted"
		} else if len(report.Findings) > 0 {
			report.Status = "findings"
		} else {
			report.Status = "no-findings"
		}
	}
	report.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveReport(&report); err != nil {
		return nil, err
	}
	return &report, nil
}

func canReuseAnalysis(report *Report) bool {
	if report.Version != reportSchemaVersion || !report.ScanCompleted || report.MIME == "" || report.Status == "running" {
		return false
	}
	for _, tool := range report.Tools {
		if tool.Name == "file" {
			return true
		}
	}
	return false
}

func isAnalysisTool(tool ToolResult) bool {
	switch tool.Name {
	case "file", "exiftool", "strings", "steghide", "stegseek-seed", "zsteg":
		return true
	case "binwalk":
		return filepath.Base(tool.OutputFile) == "binwalk.txt"
	default:
		return false
	}
}

func (app *App) runAnalysisTools(report *Report) {
	app.captureTool(report, "file", []string{"--brief", "--mime-type", "--", report.SourcePath}, "file.txt", false)
	app.captureTool(report, "exiftool", []string{"-j", "-G", "-a", "-u", "--", report.SourcePath}, "metadata.json", false)
	binwalk := app.captureTool(report, "binwalk", []string{"-B", report.SourcePath}, "binwalk.txt", false)
	if binwalk != nil && hasEmbeddedBinwalkOffset(binwalk.Summary) {
		report.Findings = append(report.Findings, Finding{Backend: "binwalk", Kind: "embedded", Summary: "embedded file signatures detected"})
	}
	app.captureTool(report, "strings", []string{"-a", "-n", "8", "--", report.SourcePath}, "strings.txt", false)

	if supportsZsteg(report.SourcePath, report.MIME) {
		result := app.captureTool(report, "zsteg", []string{"-a", report.SourcePath}, "zsteg.txt", true)
		if result != nil && zstegSelectors(result.Summary) != nil {
			report.Findings = append(report.Findings, Finding{Backend: "zsteg", Kind: "candidate", Summary: "LSB payload candidates detected"})
		}
	}
	if supportsSteghide(report.SourcePath, report.MIME) {
		result := app.captureTool(report, "steghide", []string{"info", "-p", "", report.SourcePath}, "steghide.txt", false)
		if result != nil && strings.Contains(strings.ToLower(result.Summary), "embedded file") {
			report.Findings = append(report.Findings, Finding{Backend: "steghide", Kind: "candidate", Summary: "embedded payload metadata detected"})
		} else {
			app.probeSteghideSeed(report)
		}
	}
}

func (app *App) runExtractionTools(report *Report, parsed options) error {
	app.extractBinwalk(report)
	app.extractZsteg(report)
	protectedCandidate := requiresPassphraseAnalysis(report)
	emptyCandidate := hasEmptyPassphrasePayload(report)
	if protectedCandidate {
		_, _ = fmt.Fprintf(app.stdout, "Detected: StegHide payload candidate in %s\n", report.SourcePath)
		if err := app.choosePassphraseMode(&parsed); err != nil {
			return err
		}
		if parsed.Mode == "skip" {
			report.Status = "skipped"
		} else if _, err := app.extractSteghide(report, parsed); err != nil {
			return err
		}
	}
	if supportsSteghide(report.SourcePath, report.MIME) && emptyCandidate && !protectedCandidate {
		if parsed.Mode == "manual" {
			if err := app.choosePassphraseMode(&parsed); err != nil {
				return err
			}
		}
		if _, err := app.extractSteghide(report, parsed); err != nil {
			return err
		}
	}
	if strings.HasPrefix(report.MIME, "text/") {
		app.extractStegSnow(report)
	}
	if !report.ScanCompleted && supportsZsteg(report.SourcePath, report.MIME) {
		report.Warnings = append(report.Warnings, "zsteg extraction needs selectors from xsteg scan; run scan first for complete PNG/BMP extraction")
	}
	return nil
}

func hasExtractedFinding(report *Report) bool {
	for _, finding := range report.Findings {
		if finding.Kind == "extracted" {
			return true
		}
	}
	return false
}

func (app *App) probeSteghideSeed(report *Report) {
	if _, err := app.runner.LookPath("stegseek"); err != nil {
		report.Warnings = append(report.Warnings, "stegseek is not available; protected StegHide payload detection was skipped")
		return
	}
	ctxRun, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = maxCapturedOutput, maxCapturedOutput
	progress := newSeedProgress(app.stdout)
	progress.start()
	args := []string{"--seed", report.SourcePath, "/dev/null", "-f", "-a", "-n"}
	err := app.runner.Run(ctxRun, "stegseek", args, nil, &progressCapture{capture: &stdout, progress: progress}, &progressCapture{capture: &stderr, progress: progress})
	combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	outputPath := filepath.Join(report.OutputPath, "stegseek-seed.txt")
	_ = os.WriteFile(outputPath, []byte(combined+"\n"), 0600)
	status := "no-finding"
	if strings.Contains(strings.ToLower(combined), "found (possible) seed") {
		status = "candidate"
		report.Findings = append(report.Findings, Finding{Backend: "stegseek", Kind: "password-required", Summary: "protected StegHide payload candidate detected"})
	} else if err != nil && !isStegseekSeedExhausted(combined) {
		status = "failed"
		report.Warnings = append(report.Warnings, "StegSeek seed detection failed: "+singleLine(combined))
	}
	progress.finish(status)
	report.Tools = append(report.Tools, ToolResult{Name: "stegseek-seed", Status: status, Command: append([]string{"stegseek"}, args...), OutputFile: outputPath, Summary: combined})
}

type seedProgress struct {
	output   io.Writer
	terminal bool
	mu       sync.Mutex
	latest   int
	carry    string
}

type progressCapture struct {
	capture  io.Writer
	progress *seedProgress
}

var seedPercent = regexp.MustCompile(`(?:^|\s)(\d{1,3})%`)

func newSeedProgress(output io.Writer) *seedProgress {
	progress := &seedProgress{output: output, latest: -1}
	if file, ok := output.(*os.File); ok {
		progress.terminal = term.IsTerminal(file.Fd())
	}
	return progress
}

func (progress *seedProgress) start() {
	if progress.terminal {
		_, _ = fmt.Fprint(progress.output, "Checking StegHide payload:   0%")
	} else {
		_, _ = fmt.Fprintln(progress.output, "Checking StegHide payload...")
	}
}

func (capture *progressCapture) Write(value []byte) (int, error) {
	written, err := capture.capture.Write(value)
	capture.progress.update(value)
	return written, err
}

func (progress *seedProgress) update(value []byte) {
	progress.mu.Lock()
	defer progress.mu.Unlock()
	text := progress.carry + string(value)
	if len(text) > 64 {
		progress.carry = text[len(text)-64:]
	} else {
		progress.carry = text
	}
	matches := seedPercent.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return
	}
	percentage, err := strconv.Atoi(matches[len(matches)-1][1])
	if err != nil || percentage == progress.latest {
		return
	}
	progress.latest = percentage
	if progress.terminal {
		_, _ = fmt.Fprintf(progress.output, "\rChecking StegHide payload: %3d%%", percentage)
	}
}

func (progress *seedProgress) finish(status string) {
	progress.mu.Lock()
	defer progress.mu.Unlock()
	result := "none detected"
	if status == "candidate" {
		result = "candidate detected"
	} else if status == "failed" {
		result = "check failed"
	}
	if progress.terminal {
		percentage := progress.latest
		if percentage < 0 {
			percentage = 0
		}
		_, _ = fmt.Fprintf(progress.output, "\rChecking StegHide payload: %3d%% - %s\n", percentage, result)
	} else {
		_, _ = fmt.Fprintf(progress.output, "StegHide payload check: %s\n", result)
	}
}

func requiresPassphraseAnalysis(report *Report) bool {
	for _, finding := range report.Findings {
		if finding.Backend == "stegseek" && finding.Kind == "password-required" {
			return true
		}
	}
	return false
}

func hasEmptyPassphrasePayload(report *Report) bool {
	for _, finding := range report.Findings {
		if finding.Backend == "steghide" && finding.Kind == "candidate" {
			return true
		}
	}
	return false
}

func (app *App) captureTool(report *Report, name string, args []string, filename string, optional bool) *ToolResult {
	if _, err := app.runner.LookPath(name); err != nil {
		status := "unavailable"
		if !optional {
			report.Warnings = append(report.Warnings, name+" is not available")
		}
		report.Tools = append(report.Tools, ToolResult{Name: name, Status: status})
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = maxCapturedOutput, maxCapturedOutput
	err := app.runner.Run(ctx, name, args, nil, &stdout, &stderr)
	combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	outputPath := filepath.Join(report.OutputPath, filename)
	_ = os.WriteFile(outputPath, []byte(combined+"\n"), 0600)
	status := "success"
	if err != nil {
		status = "failed"
	}
	result := ToolResult{Name: name, Status: status, Command: append([]string{name}, args...), OutputFile: outputPath, Summary: combined}
	report.Tools = append(report.Tools, result)
	return &report.Tools[len(report.Tools)-1]
}

func (app *App) extractBinwalk(report *Report) {
	if _, err := app.runner.LookPath("binwalk"); err != nil {
		return
	}
	directory := filepath.Join(report.OutputPath, "files", "binwalk")
	_ = os.MkdirAll(directory, 0700)
	result := app.captureTool(report, "binwalk", []string{"-z", "-C", directory, "-j", strconv.Itoa(maxExtractedFile), "-n", strconv.Itoa(maxExtractedFiles), report.SourcePath}, "binwalk-extract.txt", false)
	if result == nil {
		return
	}
	files, warnings := sanitizeExtractedTree(directory)
	report.Warnings = append(report.Warnings, warnings...)
	for _, path := range files {
		report.Findings = append(report.Findings, Finding{Backend: "binwalk", Kind: "extracted", Summary: "carved embedded data", Path: path})
	}
}

func (app *App) extractZsteg(report *Report) {
	if _, err := app.runner.LookPath("zsteg"); err != nil {
		return
	}
	var summary string
	for _, result := range report.Tools {
		if result.Name == "zsteg" {
			summary = result.Summary
		}
	}
	selectors := zstegSelectors(summary)
	if len(selectors) > 20 {
		selectors = selectors[:20]
	}
	for _, selector := range selectors {
		directory := filepath.Join(report.OutputPath, "files", "zsteg")
		_ = os.MkdirAll(directory, 0700)
		outputPath := filepath.Join(directory, safeName(selector)+".bin")
		file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		var stderr limitedBuffer
		stderr.limit = maxCapturedOutput
		runErr := app.runner.Run(ctx, "zsteg", []string{"-E", selector, report.SourcePath}, nil, &limitWriter{writer: file, remaining: maxExtractedFile}, &stderr)
		cancel()
		_ = file.Close()
		if runErr != nil {
			_ = os.Remove(outputPath)
			continue
		}
		report.Findings = append(report.Findings, Finding{Backend: "zsteg", Kind: "extracted", Summary: selector, Path: outputPath})
	}
}

func (app *App) extractSteghide(report *Report, parsed options) (bool, error) {
	directory := filepath.Join(report.OutputPath, "files", "steghide")
	_ = os.MkdirAll(directory, 0700)
	outputPath := filepath.Join(directory, "payload.bin")
	if parsed.Mode == "manual" {
		return app.extractSteghideManual(report, outputPath, parsed.Password)
	}
	if _, err := app.runner.LookPath("stegseek"); err != nil {
		if app.extractSteghideEmptyPassword(report, outputPath) {
			return true, nil
		}
		return false, errors.New("StegHide payload detected, but stegseek is unavailable and the empty passphrase was rejected")
	}
	wordlists := []string{"/dev/null"}
	if parsed.Wordlist != "" {
		absolute, err := filepath.Abs(parsed.Wordlist)
		if err != nil {
			return false, err
		}
		wordlists = append(wordlists, absolute)
	} else if !parsed.NoCrack {
		selections, err := recommendWordlists(ctx.WordlistKindPassword)
		if err != nil {
			report.Warnings = append(report.Warnings, err.Error()+"; run xwordlist extract if rockyou.txt is compressed")
		} else {
			for _, selection := range selections {
				wordlists = append(wordlists, selection.Path)
			}
		}
	}
	if parsed.NoCrack {
		wordlists = wordlists[:1]
	}
	for _, wordlist := range wordlists {
		if wordlistAttempted(report.Wordlists, wordlist) {
			continue
		}
		_ = os.Remove(outputPath)
		ctxRun, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		var stdout, stderr limitedBuffer
		stdout.limit, stderr.limit = maxCapturedOutput, maxCapturedOutput
		args := []string{"--crack", report.SourcePath, wordlist, outputPath, "-f", "-a", "-n"}
		err := app.runner.Run(ctxRun, "stegseek", args, nil, &stdout, &stderr)
		cancel()
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if info, statErr := os.Stat(outputPath); err == nil && statErr == nil && info.Mode().IsRegular() {
			password := parseStegseekPassword(combined)
			originalName := parseEmbeddedName(combined)
			outputPath = preserveEmbeddedName(outputPath, originalName)
			report.Wordlists = append(report.Wordlists, WordlistRun{Path: wordlist, Status: "found"})
			report.Findings = append(report.Findings, Finding{Backend: "stegseek", Kind: "extracted", Summary: "steghide payload extracted", OriginalName: originalName, Path: outputPath, Password: password})
			_ = saveReport(report)
			return true, nil
		}
		status := "failed"
		if isStegseekExhausted(combined) {
			status = "exhausted"
		}
		report.Wordlists = append(report.Wordlists, WordlistRun{Path: wordlist, Status: status})
		_ = saveReport(report)
		if status == "failed" {
			return false, fmt.Errorf("StegSeek failed with %s: %s", wordlist, singleLine(combined))
		}
	}
	return false, errors.New("no valid passphrase was found; no payload was extracted")
}

func (app *App) extractSteghideManual(report *Report, outputPath, password string) (bool, error) {
	if _, err := app.runner.LookPath("steghide"); err != nil {
		return false, errors.New("steghide is not available for manual extraction")
	}
	originalName := app.inspectSteghideManual(report.SourcePath, password)
	if safeName := sanitizeEmbeddedName(originalName); safeName != "" {
		outputPath = filepath.Join(filepath.Dir(outputPath), safeName)
	}
	_ = os.Remove(outputPath)
	ctxRun, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var output limitedBuffer
	output.limit = maxCapturedOutput
	err := app.runner.Run(ctxRun, "steghide", []string{"extract", "-sf", report.SourcePath, "-p", password, "-xf", outputPath, "-f"}, nil, &output, &output)
	if info, statErr := os.Stat(outputPath); err == nil && statErr == nil && info.Mode().IsRegular() {
		report.Findings = append(report.Findings, Finding{Backend: "steghide", Kind: "extracted", Summary: "payload extracted with manually entered passphrase", OriginalName: originalName, Path: outputPath})
		return true, nil
	}
	return false, errors.New("the manually entered passphrase was rejected; no payload was extracted")
}

func (app *App) inspectSteghideManual(sourcePath, password string) string {
	ctxRun, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var output limitedBuffer
	output.limit = maxCapturedOutput
	_ = app.runner.Run(ctxRun, "steghide", []string{"info", "-p", password, sourcePath}, nil, &output, &output)
	return parseEmbeddedName(output.String())
}

func (app *App) extractSteghideEmptyPassword(report *Report, outputPath string) bool {
	if _, err := app.runner.LookPath("steghide"); err != nil {
		return false
	}
	ctxRun, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var output limitedBuffer
	output.limit = maxCapturedOutput
	err := app.runner.Run(ctxRun, "steghide", []string{"extract", "-sf", report.SourcePath, "-p", "", "-xf", outputPath, "-f"}, nil, &output, &output)
	if err == nil {
		if info, statErr := os.Stat(outputPath); statErr == nil && info.Mode().IsRegular() {
			report.Findings = append(report.Findings, Finding{Backend: "steghide", Kind: "extracted", Summary: "payload extracted with empty passphrase", Path: outputPath})
			return true
		}
	}
	return false
}

func (app *App) extractStegSnow(report *Report) {
	if _, err := app.runner.LookPath("stegsnow"); err != nil {
		return
	}
	directory := filepath.Join(report.OutputPath, "files", "stegsnow")
	_ = os.MkdirAll(directory, 0700)
	outputPath := filepath.Join(directory, "message.txt")
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	ctxRun, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	runErr := app.runner.Run(ctxRun, "stegsnow", []string{"-C", report.SourcePath}, nil, &limitWriter{writer: file, remaining: maxExtractedFile}, io.Discard)
	cancel()
	_ = file.Close()
	if runErr != nil {
		_ = os.Remove(outputPath)
		return
	}
	if info, err := os.Stat(outputPath); err == nil && info.Size() > 0 {
		report.Findings = append(report.Findings, Finding{Backend: "stegsnow", Kind: "extracted", Summary: "whitespace-hidden message extracted", Path: outputPath})
	}
}

func (app *App) doctor() error {
	tools := []struct {
		Name     string
		Required bool
		Purpose  string
	}{
		{"file", true, "MIME detection"},
		{"exiftool", true, "metadata analysis"},
		{"binwalk", true, "embedded signatures and carving"},
		{"strings", true, "printable-string analysis"},
		{"steghide", true, "steghide inspection and fallback extraction"},
		{"stegseek", true, "steghide passphrase cracking"},
		{"zsteg", false, "PNG/BMP LSB analysis"},
		{"stegsnow", false, "text whitespace steganography"},
	}
	_, _ = fmt.Fprintln(app.stdout, "BACKEND   STATUS       PURPOSE")
	for _, tool := range tools {
		status := "ready"
		if _, err := app.runner.LookPath(tool.Name); err != nil {
			status = "optional"
			if tool.Required {
				status = "missing"
			}
		}
		_, _ = fmt.Fprintf(app.stdout, "%-9s %-12s %s\n", tool.Name, status, tool.Purpose)
	}
	selections, err := recommendWordlists(ctx.WordlistKindPassword)
	if err != nil {
		_, _ = fmt.Fprintf(app.stdout, "wordlists missing      %s\n", err)
	} else {
		_, _ = fmt.Fprintf(app.stdout, "wordlists ready        %d password lists\n", len(selections))
	}
	return nil
}

func (app *App) list(path string) error {
	reports, err := listReports(path)
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		_, err := fmt.Fprintln(app.stdout, "No xsteg reports found.")
		return err
	}
	_, _ = fmt.Fprintln(app.stdout, "ID  STATUS    MODE     FINDINGS  SOURCE")
	for _, item := range reports {
		_, _ = fmt.Fprintf(app.stdout, "%2d  %-8s  %-7s  %8d  %s\n", item.ID, item.Report.Status, item.Report.Mode, len(item.Report.Findings), item.Report.SourcePath)
	}
	return nil
}

func (app *App) show(id int, path string) error {
	reports, err := listReports(path)
	if err != nil {
		return err
	}
	if id < 1 || id > len(reports) {
		return fmt.Errorf("unknown xsteg report ID: %d", id)
	}
	report := reports[id-1].Report
	_, _ = fmt.Fprintf(app.stdout, "Source: %s\nSHA-256: %s\nMIME: %s\nMode: %s\nStatus: %s\nOutput: %s\n", report.SourcePath, report.SourceSHA256, report.MIME, report.Mode, report.Status, report.OutputPath)
	if len(report.Findings) == 0 {
		_, _ = fmt.Fprintln(app.stdout, "Findings: none")
	} else {
		_, _ = fmt.Fprintln(app.stdout, "Findings:")
		for _, finding := range report.Findings {
			_, _ = fmt.Fprintf(app.stdout, "- %s: %s", finding.Backend, finding.Summary)
			if finding.OriginalName != "" {
				_, _ = fmt.Fprintf(app.stdout, " [%s]", finding.OriginalName)
			}
			if finding.Path != "" {
				_, _ = fmt.Fprintf(app.stdout, " -> %s", finding.Path)
			}
			if finding.Password != "" {
				_, _ = fmt.Fprintf(app.stdout, " (password: %s)", finding.Password)
			}
			_, _ = fmt.Fprintln(app.stdout)
		}
	}
	for _, warning := range report.Warnings {
		_, _ = fmt.Fprintf(app.stdout, "Warning: %s\n", warning)
	}
	return nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int64
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	if buffer.limit <= 0 {
		return len(value), nil
	}
	remaining := buffer.limit - int64(buffer.Len())
	if remaining <= 0 {
		return len(value), nil
	}
	write := value
	if int64(len(write)) > remaining {
		write = write[:remaining]
	}
	_, _ = buffer.Buffer.Write(write)
	return len(value), nil
}

type limitWriter struct {
	writer    io.Writer
	remaining int64
}

func (writer *limitWriter) Write(value []byte) (int, error) {
	if writer.remaining <= 0 {
		return 0, fmt.Errorf("extracted file exceeds %d bytes", maxExtractedFile)
	}
	write := value
	if int64(len(write)) > writer.remaining {
		write = write[:writer.remaining]
	}
	written, err := writer.writer.Write(write)
	writer.remaining -= int64(written)
	if err == nil && written < len(value) {
		err = fmt.Errorf("extracted file exceeds %d bytes", maxExtractedFile)
	}
	return written, err
}

var zstegLine = regexp.MustCompile(`(?m)^([^\s]+)\s+\.\.\s+(?:file:|text:)`)
var stegseekPassword = regexp.MustCompile(`(?i)found passphrase:\s*"([^"]*)"`)
var embeddedName = regexp.MustCompile(`(?i)(?:original filename:|embedded file)\s*"([^"]+)"`)

func zstegSelectors(output string) []string {
	seen := make(map[string]struct{})
	var selectors []string
	for _, match := range zstegLine.FindAllStringSubmatch(output, -1) {
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		selectors = append(selectors, match[1])
	}
	return selectors
}

func parseStegseekPassword(output string) string {
	match := stegseekPassword.FindStringSubmatch(output)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func parseEmbeddedName(output string) string {
	match := embeddedName.FindStringSubmatch(output)
	if len(match) == 2 {
		return filepath.Base(match[1])
	}
	return ""
}

func sanitizeEmbeddedName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	var builder strings.Builder
	for _, character := range name {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._-", character) {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	return strings.TrimLeft(builder.String(), ".")
}

func preserveEmbeddedName(currentPath, originalName string) string {
	name := sanitizeEmbeddedName(originalName)
	if name == "" {
		return currentPath
	}
	target := filepath.Join(filepath.Dir(currentPath), name)
	if target == currentPath {
		return currentPath
	}
	_ = os.Remove(target)
	if err := os.Rename(currentPath, target); err != nil {
		return currentPath
	}
	return target
}

func isStegseekExhausted(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "could not find a valid passphrase") || strings.Contains(lower, "no valid passphrase")
}

func isStegseekSeedExhausted(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "could not find a valid seed")
}

func wordlistAttempted(runs []WordlistRun, path string) bool {
	for _, run := range runs {
		if run.Path == path && (run.Status == "exhausted" || run.Status == "found") {
			return true
		}
	}
	return false
}

func supportsSteghide(path, mime string) bool {
	switch mime {
	case "image/jpeg", "image/bmp", "image/x-ms-bmp", "audio/wav", "audio/wave", "audio/x-wav", "audio/vnd.wave", "audio/basic":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".bmp", ".wav", ".au":
		return true
	default:
		return false
	}
}

func supportsZsteg(path, mime string) bool {
	if mime == "image/png" || mime == "image/bmp" || mime == "image/x-ms-bmp" {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".bmp":
		return true
	default:
		return false
	}
}

func canReuseReport(report *Report, parsed options) bool {
	if report.Version != reportSchemaVersion || report.Status == "running" || report.Status == "failed" || report.Mode != parsed.Command {
		return false
	}
	if parsed.Command == "scan" {
		return true
	}
	if parsed.Mode == "manual" {
		return false
	}
	for _, finding := range report.Findings {
		if finding.Backend == "stegseek" && finding.Kind == "extracted" {
			return true
		}
	}
	if parsed.Wordlist != "" {
		absolute, err := filepath.Abs(parsed.Wordlist)
		return err == nil && wordlistAttempted(report.Wordlists, absolute)
	}
	if parsed.NoCrack {
		return wordlistAttempted(report.Wordlists, "/dev/null")
	}
	return false
}

func hasEmbeddedBinwalkOffset(output string) bool {
	rootDescription := ""
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "0" {
			rootDescription = strings.ToLower(strings.Join(fields[2:], " "))
			break
		}
	}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		offset, err := strconv.ParseInt(fields[0], 10, 64)
		if err == nil && offset > 0 && !isLegitimateJPEGExifSignature(rootDescription, fields, offset) {
			return true
		}
	}
	return false
}

func isLegitimateJPEGExifSignature(rootDescription string, fields []string, offset int64) bool {
	if !strings.Contains(rootDescription, "jpeg image data") || !strings.Contains(rootDescription, "exif") {
		return false
	}
	if offset > 64 || len(fields) < 3 {
		return false
	}
	description := strings.ToLower(strings.Join(fields[2:], " "))
	return strings.Contains(description, "tiff image data")
}

func safeName(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "payload"
	}
	return builder.String()
}

func singleLine(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 200 {
		return value[:200] + "..."
	}
	return value
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func detectMIME(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	prefix := make([]byte, 512)
	count, err := file.Read(prefix)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return http.DetectContentType(prefix[:count]), nil
}

func resolveOutputRoot(sourcePath, digest string) (string, *Report, error) {
	base := sourcePath + ".xsteg"
	emptyCandidate := ""
	for number := 1; ; number++ {
		candidate := base
		if number > 1 {
			candidate += "." + strconv.Itoa(number)
		}
		report, err := loadReport(filepath.Join(candidate, "report.json"))
		if err == nil {
			if report.SourceSHA256 == digest {
				return candidate, report, nil
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", nil, err
		}
		if _, statErr := os.Lstat(candidate); errors.Is(statErr, os.ErrNotExist) {
			if emptyCandidate != "" {
				return emptyCandidate, nil, nil
			}
			return candidate, nil, nil
		} else if statErr != nil {
			return "", nil, statErr
		} else if isEmptyDirectory(candidate) && emptyCandidate == "" {
			emptyCandidate = candidate
		}
	}
}

func isEmptyDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	directory, err := os.Open(path)
	if err != nil {
		return false
	}
	defer directory.Close()
	_, err = directory.Readdirnames(1)
	return errors.Is(err, io.EOF)
}

func saveReport(report *Report) error {
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	temporary, err := os.CreateTemp(report.OutputPath, ".report-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, filepath.Join(report.OutputPath, "report.json"))
}

func loadReport(path string) (*Report, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		return nil, fmt.Errorf("invalid xsteg report %s: %w", path, err)
	}
	return &report, nil
}

func collectInputFiles(path string) ([]string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode().IsRegular() {
		return []string{absolute}, nil
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("input is not a regular file or directory: %s", path)
	}
	var files []string
	err = filepath.WalkDir(absolute, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if current != absolute && (isOutputDirectory(name) || name == ".git" || name == ".ctx" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files = append(files, current)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

var outputDirectoryPattern = regexp.MustCompile(`\.xsteg(?:\.\d+)?$`)

func isOutputDirectory(name string) bool { return outputDirectoryPattern.MatchString(name) }

func listReports(path string) ([]listedReport, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode().IsRegular() && filepath.Base(absolute) == "report.json" {
		report, err := loadReport(absolute)
		if err != nil {
			return nil, err
		}
		return []listedReport{{ID: 1, Path: absolute, Report: *report}}, nil
	}
	var reports []listedReport
	err = filepath.WalkDir(absolute, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != absolute && (entry.Name() == ".git" || entry.Name() == ".ctx" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "report.json" || !isOutputDirectory(filepath.Base(filepath.Dir(current))) {
			return nil
		}
		report, err := loadReport(current)
		if err != nil {
			return err
		}
		reports = append(reports, listedReport{Path: current, Report: *report})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Report.SourcePath < reports[j].Report.SourcePath })
	for index := range reports {
		reports[index].ID = index + 1
	}
	return reports, nil
}

func sanitizeExtractedTree(root string) ([]string, []string) {
	var files, warnings []string
	count := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, err.Error())
			return nil
		}
		if path == root {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			warnings = append(warnings, err.Error())
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			_ = os.Remove(path)
			warnings = append(warnings, "removed unsafe extracted entry: "+path)
			return nil
		}
		if info.Mode().IsRegular() {
			count++
			if count > maxExtractedFiles || info.Size() > maxExtractedFile {
				_ = os.Remove(path)
				warnings = append(warnings, "removed extracted file exceeding limits: "+path)
				return nil
			}
			_ = os.Chmod(path, 0600)
			files = append(files, path)
		}
		return nil
	})
	return files, warnings
}
