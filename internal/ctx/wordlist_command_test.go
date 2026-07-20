package ctx

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildWordlistCatalogIncludesFilesAndClassifiesKinds(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"dirb", "seclists/Discovery/Web-Content", "seclists/Discovery/DNS", "seclists/Discovery/Web-Content/ParamMiner", "seclists/Passwords"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"dirb/common.txt":                                                    "admin\nlogin\n",
		"seclists/Discovery/Web-Content/paths":                               "api\n",
		"seclists/Discovery/DNS/subdomains-top1million.txt":                  "www\n",
		"seclists/Discovery/Web-Content/ParamMiner/burp-parameter-names.txt": "next\n",
		"seclists/Passwords/README.md":                                       "metadata",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("missing.txt", filepath.Join(root, "broken.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "dirb"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	catalog, err := buildWordlistCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 6 {
		t.Fatalf("entries = %d, want 6 including broken link and duplicate collapse", len(catalog.Entries))
	}
	var sawPath, sawBroken bool
	for _, entry := range catalog.Entries {
		if strings.HasSuffix(entry.Path, "/paths") {
			sawPath = entry.Kind == WordlistKindDirectory && entry.Usable
		}
		if entry.Path == filepath.Join(root, "broken.txt") {
			sawBroken = !entry.Available && !entry.Usable
		}
		// The alias and its original are one resolved file; one logical path is enough.
	}
	if !sawPath || !sawBroken {
		t.Fatalf("catalog = %+v", catalog.Entries)
	}
}

func TestWordlistSuggestionsPreferTaskSpecificLists(t *testing.T) {
	entries := []WordlistEntry{
		{Path: "/lists/generic.txt", Kind: WordlistKindParameterValue, Usable: true},
		{Path: "/lists/url-redirect.txt", Kind: WordlistKindParameterValue, Usable: true},
		{Path: "/lists/burp-parameter-names.txt", Kind: WordlistKindParameterName, Usable: true},
	}
	got := recommendWordlists(entries, WordlistKindParameterValue)
	if len(got) != 2 || got[0].Path != "/lists/generic.txt" {
		t.Fatalf("suggestions = %+v", got)
	}
}

func TestWordlistOptionsWithoutCommandUseList(t *testing.T) {
	options, path, err := parseWordlistOptions([]string{"--kind", WordlistKindSubdomain})
	if err != nil {
		t.Fatal(err)
	}
	if options.Kind != WordlistKindSubdomain || path != "" {
		t.Fatalf("options = %+v, path = %q", options, path)
	}
}

func TestExtractTrustedWordlist(t *testing.T) {
	root := t.TempDir()
	content := []byte("password\nletmein\n")
	spec := writeTrustedWordlistFixture(t, root, content)
	var out bytes.Buffer
	extracted, err := extractTrustedWordlist(root, spec, wordlistExtractOptions{Yes: true}, bufio.NewScanner(strings.NewReader("")), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !extracted {
		t.Fatal("extracted = false, want true")
	}
	got, err := os.ReadFile(filepath.Join(root, spec.RelativeOutputPath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("output = %q, want %q", got, content)
	}
	if _, err := os.Stat(filepath.Join(root, spec.RelativeSourcePath)); err != nil {
		t.Fatalf("source should remain: %v", err)
	}
}

func TestExtractTrustedWordlistRejectsHashMismatch(t *testing.T) {
	root := t.TempDir()
	spec := writeTrustedWordlistFixture(t, root, []byte("password\n"))
	spec.SourceSHA256 = strings.Repeat("0", 64)
	_, err := extractTrustedWordlist(root, spec, wordlistExtractOptions{Yes: true}, bufio.NewScanner(strings.NewReader("")), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not a trusted version") {
		t.Fatalf("error = %v, want trusted-version failure", err)
	}
	if _, err := os.Stat(filepath.Join(root, spec.RelativeOutputPath)); !os.IsNotExist(err) {
		t.Fatalf("output should not exist, stat error = %v", err)
	}
}

func TestExtractTrustedWordlistCanRemoveSource(t *testing.T) {
	root := t.TempDir()
	spec := writeTrustedWordlistFixture(t, root, []byte("password\n"))
	_, err := extractTrustedWordlist(root, spec, wordlistExtractOptions{Yes: true, RemoveSource: true}, bufio.NewScanner(strings.NewReader("")), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, spec.RelativeSourcePath)); !os.IsNotExist(err) {
		t.Fatalf("source should be removed, stat error = %v", err)
	}
}

func TestParseWordlistExtractOptionsSupportsInternalSudoReexec(t *testing.T) {
	options, help, err := parseWordlistExtractOptions([]string{"--internal", "--yes", "--force", "--remove-source"})
	if err != nil {
		t.Fatal(err)
	}
	if help || !options.Internal || !options.Yes || !options.Force || !options.RemoveSource {
		t.Fatalf("options = %+v, help = %t", options, help)
	}
}

func TestWordlistExtractSudoArgsPreserveExplicitConfirmationChoice(t *testing.T) {
	withoutYes := strings.Join(wordlistExtractSudoArgs("/usr/bin/ctx", wordlistExtractOptions{}), " ")
	if strings.Contains(withoutYes, "--yes") {
		t.Fatalf("sudo args unexpectedly skip confirmation: %s", withoutYes)
	}
	withYes := strings.Join(wordlistExtractSudoArgs("/usr/bin/ctx", wordlistExtractOptions{Yes: true}), " ")
	if !strings.Contains(withYes, "--yes") {
		t.Fatalf("sudo args do not preserve explicit --yes: %s", withYes)
	}
}

func writeTrustedWordlistFixture(t *testing.T, root string, content []byte) wordlistExtractSpec {
	t.Helper()
	source := filepath.Join(root, "rockyou.txt.gz")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	hash, err := sha256File(source)
	if err != nil {
		t.Fatal(err)
	}
	return wordlistExtractSpec{
		ID:                 "rockyou",
		RelativeSourcePath: "rockyou.txt.gz",
		RelativeOutputPath: "rockyou.txt",
		Format:             "gzip",
		Kind:               WordlistKindPassword,
		SourceSHA256:       hash,
		OutputSize:         int64(len(content)),
	}
}
