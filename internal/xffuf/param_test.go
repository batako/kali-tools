package xffuf

import (
	"io"
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
		{"http://nahamstore.thm/?hoge=FUZZ", "param-value", "hoge", paramValueProfile},
		{"http://nahamstore.thm/?redirect=FUZZ", "param-value", "redirect", paramValueProfile},
		{"http://nahamstore.thm/?file=FUZZ", "param-value", "file", paramValueProfile},
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

func TestDiscoverParamWordlistsUsesCtxRecommendation(t *testing.T) {
	original := recommendWordlists
	t.Cleanup(func() { recommendWordlists = original })
	requested := ""
	recommendWordlists = func(kind string) ([]ctx.WordlistSelection, error) {
		requested = kind
		return []ctx.WordlistSelection{{Provider: "seclists", Path: "/lists/params.txt"}}, nil
	}
	candidates, err := discoverParamWordlists(paramTemplate{Type: "param-name"}, paramNameProfile)
	if err != nil {
		t.Fatal(err)
	}
	if requested != ctx.WordlistKindParameterName || len(candidates) != 1 || candidates[0].Profile != paramNameProfile || candidates[0].Type != "param-name" {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestDiscoverVhostWordlistsUsesCtxRecommendation(t *testing.T) {
	original := recommendWordlists
	t.Cleanup(func() { recommendWordlists = original })
	requested := ""
	recommendWordlists = func(kind string) ([]ctx.WordlistSelection, error) {
		requested = kind
		return []ctx.WordlistSelection{{Provider: "seclists", Path: "/lists/subdomains.txt"}}, nil
	}
	candidates, err := discoverWordlists()
	if err != nil {
		t.Fatal(err)
	}
	if requested != ctx.WordlistKindSubdomain || len(candidates) != 1 || candidates[0].Profile != "vhost" || candidates[0].Type != "vhost" {
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
	err = app.runParamScan(workspace, target, "http://nahamstore.thm/?hoge=FUZZ", paramTemplate{Type: "param-value", ParameterName: "hoge"}, ctx.WordlistSelection{Provider: ctx.WordlistProviderLists, Profile: paramValueProfile, Type: "param-value", Path: "/usr/share/wordlists/test.txt"}, []string{"host-0"}, options{Mode: "param", NoAutoFilter: true}, statePath, []string{"param"})
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
