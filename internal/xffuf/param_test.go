package xffuf

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"req/internal/ctx"
)

func TestParseParamTemplateAndInferProfile(t *testing.T) {
	tests := []struct {
		url, kind, name, profile string
	}{
		{"http://nahamstore.thm/?FUZZ=fuga", "param-name", "", paramNameProfile},
		{"http://nahamstore.thm/?hoge=FUZZ", "param-value", "hoge", paramValueGenericProfile},
		{"http://nahamstore.thm/?redirect=FUZZ", "param-value", "redirect", "parameter-value-url"},
		{"http://nahamstore.thm/?file=FUZZ", "param-value", "file", "parameter-value-file"},
	}
	for _, test := range tests {
		parsed, err := parseParamTemplate(test.url)
		if err != nil {
			t.Fatalf("parseParamTemplate(%q): %v", test.url, err)
		}
		if parsed.Type != test.kind || parsed.ParameterName != test.name || inferParamProfile(parsed) != test.profile {
			t.Fatalf("parseParamTemplate(%q) = %+v, profile %q", test.url, parsed, inferParamProfile(parsed))
		}
	}
	for _, invalid := range []string{"http://example.test/", "http://example.test/FUZZ", "http://example.test/?a=FUZZ&b=FUZZ"} {
		if _, err := parseParamTemplate(invalid); err == nil {
			t.Fatalf("parseParamTemplate(%q) accepted invalid URL", invalid)
		}
	}
}

func TestDiscoverParamWordlistsUsesWordlistsProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wordlists")
	realSeclists := filepath.Join(t.TempDir(), "seclists")
	path := filepath.Join(realSeclists, "Discovery", "Web-Content", "burp-parameter-names.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("admin\ndebug\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realSeclists, filepath.Join(root, "seclists")); err != nil {
		t.Fatal(err)
	}
	candidates, err := discoverParamWordlistsFromRoot(root, paramTemplate{Type: "param-name"}, paramNameProfile)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "seclists", "Discovery", "Web-Content", "burp-parameter-names.txt")
	if len(candidates) != 1 || candidates[0].Provider != ctx.WordlistProviderLists || candidates[0].Path != wantPath {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestDiscoverVhostWordlistsUsesWordlistsProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wordlists")
	path := filepath.Join(root, "seclists", "Discovery", "DNS", "subdomains-top1million-5000.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("www\nadmin\n"), 0644); err != nil {
		t.Fatal(err)
	}
	candidates, err := discoverVhostWordlistsFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Provider != ctx.WordlistProviderLists || candidates[0].Path != path {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestRunParamScanPersistsStructuredDiscovery(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := ctx.InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target, err := ctx.SetPrimaryTargetIP(workspace, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "searched.words")
	app := New(resultRunner{count: 1}, strings.NewReader(""), io.Discard, io.Discard)
	err = app.runParamScan(workspace, target, "http://nahamstore.thm/?hoge=FUZZ", paramTemplate{Type: "param-value", ParameterName: "hoge"}, ctx.WordlistSelection{Provider: ctx.WordlistProviderLists, Profile: paramValueGenericProfile, Type: "param-value", Path: "/usr/share/wordlists/test.txt"}, []string{"host-0"}, options{Mode: "param", NoAutoFilter: true}, statePath, []string{"param"})
	if err != nil {
		t.Fatal(err)
	}
	discoveries, err := ctx.ListWebDiscoveries(workspace, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(discoveries) != 1 {
		t.Fatalf("discoveries = %+v", discoveries)
	}
	got := discoveries[0]
	if got.DiscoveryType != "param-value" || got.ParameterName != "hoge" || got.ParameterValue != "host-0" || got.TemplateURL == "" || got.FuzzPart != "value" {
		t.Fatalf("discovery = %+v", got)
	}
}
