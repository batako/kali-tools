package ctx

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSummarizeWebDiscoveriesUsesLatestObservationAndAggregatesSources(t *testing.T) {
	discoveries := []WebDiscovery{
		{URL: "https://example.test/admin", Path: "/admin", StatusCode: 301, SourceTool: "gobuster", UpdatedAt: "2026-07-20T00:00:00Z"},
		{URL: "https://example.test/private", Path: "/private", StatusCode: 403, SourceTool: "gobuster", UpdatedAt: "2026-07-20T00:01:00Z"},
		{URL: "https://example.test/admin", Path: "/admin", StatusCode: 200, ContentLength: 512, ContentLengthValid: true, SourceTool: "ffuf", UpdatedAt: "2026-07-20T00:02:00Z"},
	}

	views := SummarizeWebDiscoveries(discoveries)
	if len(views) != 2 {
		t.Fatalf("SummarizeWebDiscoveries() len = %d, want 2", len(views))
	}
	if views[0].Path != "/admin" || views[0].StatusCode != 200 || views[0].ContentLength != 512 {
		t.Fatalf("latest /admin view = %+v", views[0])
	}
	if got := strings.Join(views[0].Sources, ","); got != "ffuf,gobuster" {
		t.Fatalf("/admin sources = %q, want ffuf,gobuster", got)
	}
	if views[1].Path != "/private" || views[1].StatusCode != 403 {
		t.Fatalf("second view = %+v, want /private 403", views[1])
	}
}

func TestWriteWebDiscoveryListGroupsOriginsAndShowsDetails(t *testing.T) {
	target := &Target{Name: "web", IP: "10.10.10.10"}
	discoveries := []WebDiscovery{
		{URL: "https://example.test/admin", Path: "/admin", StatusCode: 301, ContentLength: 169, ContentLengthValid: true, RedirectURL: "/admin/", RedirectURLValid: true, SourceTool: "gobuster"},
		{URL: "http://10.10.10.10/", Path: "/", StatusCode: 200, SourceTool: "ffuf"},
	}

	var out bytes.Buffer
	if err := WriteWebDiscoveryList(&out, target, discoveries); err != nil {
		t.Fatalf("WriteWebDiscoveryList() error = %v", err)
	}
	for _, want := range []string{
		"Target: web (10.10.10.10)",
		"http://10.10.10.10",
		"https://example.test",
		"STATUS", "PATH", "LENGTH", "REDIRECT", "SOURCES",
		"200", "/admin", "169", "/admin/", "gobuster", "ffuf",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
}

func TestWriteWebDiscoveryListSeparatesParameterKinds(t *testing.T) {
	target := &Target{Name: "web", IP: "10.10.10.10"}
	discoveries := []WebDiscovery{
		{ID: 7, DiscoveryType: "param-name", URL: "http://example.test/?admin=fuga", Path: "/", ParameterName: "admin", ParameterValue: "fuga", StatusCode: 200, SourceTool: "xffuf"},
		{ID: 8, DiscoveryType: "param-value", URL: "http://example.test/?hoge=debug", Path: "/", ParameterName: "hoge", ParameterValue: "debug", StatusCode: 302, SourceTool: "xffuf"},
	}
	var out bytes.Buffer
	if err := WriteWebDiscoveryList(&out, target, discoveries); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Parameter names", "Parameter values", "PARAMETER", "admin", "hoge", "debug"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
}

func TestRunWebDefaultsToListAndSupportsTargetSelection(t *testing.T) {
	workspace := initXTestWorkspace(t)
	primary, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatal(err)
	}
	secondary, err := AddTarget(workspace, "10.10.10.20", "secondary")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveWebDiscovery(workspace, primary, WebDiscovery{URL: "http://10.10.10.10/admin", Path: "/admin", StatusCode: 200, SourceTool: "gobuster", Wordlist: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveWebDiscovery(workspace, secondary, WebDiscovery{URL: "http://10.10.10.20/api", Path: "/api", StatusCode: 401, SourceTool: "ffuf", Wordlist: "test"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "web"}, &out); err != nil {
		t.Fatalf("Run(ctx web) error = %v", err)
	}
	if !strings.Contains(out.String(), "/admin") || strings.Contains(out.String(), "/api") {
		t.Fatalf("primary web output = %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"ctx", "web", "ls", "--target", "secondary"}, &out); err != nil {
		t.Fatalf("Run(ctx web ls --target secondary) error = %v", err)
	}
	if !strings.Contains(out.String(), "/api") || strings.Contains(out.String(), "/admin") {
		t.Fatalf("secondary web output = %q", out.String())
	}
}

func TestRunWebFiltersAndShowsParameterDiscovery(t *testing.T) {
	workspace := initXTestWorkspace(t)
	target, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveWebDiscovery(workspace, target, WebDiscovery{URL: "http://example.test/admin", Path: "/admin", StatusCode: 200, SourceTool: "gobuster", Wordlist: "test"}); err != nil {
		t.Fatal(err)
	}
	id, err := SaveWebDiscovery(workspace, target, WebDiscovery{DiscoveryType: "param-value", URL: "http://example.test/?hoge=debug", TemplateURL: "http://example.test/?hoge=FUZZ", Path: "/", ParameterName: "hoge", ParameterValue: "debug", FuzzPart: "value", StatusCode: 200, SourceTool: "xffuf", Wordlist: "values.txt"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"ctx", "web", "ls", "--type", "param"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hoge") || strings.Contains(out.String(), "/admin") {
		t.Fatalf("filtered output = %q", out.String())
	}
	out.Reset()
	if err := Run([]string{"ctx", "web", "show", strconv.FormatInt(id, 10)}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Type:", "param-value", "Template:", "?hoge=FUZZ", "Parameter:", "hoge", "Value:", "debug"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("detail output = %q, want %q", out.String(), want)
		}
	}
}

func TestRunWebClearRemovesAllDiscoveriesForSelectedTargetOnly(t *testing.T) {
	workspace := initXTestWorkspace(t)
	primary, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatal(err)
	}
	secondary, err := AddTarget(workspace, "10.10.10.20", "secondary")
	if err != nil {
		t.Fatal(err)
	}
	logID, err := SaveCommandLog(workspace, CommandLog{Command: "xgobuster", ExpandedCommand: "gobuster dir", Status: "success", StartedAt: "2026-07-20T00:00:00Z", EndedAt: "2026-07-20T00:01:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	for index, path := range []string{"/admin", "/api"} {
		discovery := WebDiscovery{URL: "http://10.10.10.10" + path, Path: path, StatusCode: 200, SourceTool: "gobuster"}
		if index == 0 {
			discovery.CommandLogID = logID
			discovery.CommandLogIDValid = true
		}
		if _, err := SaveWebDiscovery(workspace, primary, discovery); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SaveWebDiscovery(workspace, secondary, WebDiscovery{URL: "http://10.10.10.20/private", Path: "/private", StatusCode: 403, SourceTool: "ffuf"}); err != nil {
		t.Fatal(err)
	}
	if _, err := StartWebWordlistRun(workspace, primary, "http://10.10.10.10", "wordlists", "web-quick", "", "/usr/share/dirb/wordlists/common.txt", "2026-07-20T00:00:00Z", logID); err != nil {
		t.Fatal(err)
	}
	if _, err := StartWebWordlistRun(workspace, secondary, "http://10.10.10.20", "wordlists", "web-quick", "", "/usr/share/dirb/wordlists/common.txt", "2026-07-20T00:00:00Z", 0); err != nil {
		t.Fatal(err)
	}
	primaryCaches := webDiscoveryCachePaths(workspace, primary)
	secondaryCaches := webDiscoveryCachePaths(workspace, secondary)
	for _, path := range append(append([]string{}, primaryCaches...), secondaryCaches...) {
		if err := os.MkdirAll(filepath.Join(path, "state"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "state", "searched.words"), []byte("admin\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run([]string{"ctx", "web", "clear", "/admin"}, &bytes.Buffer{}); err == nil {
		t.Fatal("Run(ctx web clear /admin) error = nil, want individual deletion rejected")
	}
	oldStdin := workspaceStdin
	workspaceStdin = strings.NewReader("y\n")
	t.Cleanup(func() { workspaceStdin = oldStdin })

	var out bytes.Buffer
	if err := Run([]string{"ctx", "web", "clear"}, &out); err != nil {
		t.Fatalf("Run(ctx web clear) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Clear all web discovery data for target default (10.10.10.10)?") || !strings.Contains(got, "Discoveries: 2") || !strings.Contains(got, "Wordlist runs: 1") || !strings.Contains(got, "Wordlist cache: present") || !strings.Contains(got, "xgobuster and xffuf search progress") || !strings.Contains(got, "Cleared web discovery data for target: default") || !strings.Contains(got, "Wordlist cache: removed") {
		t.Fatalf("clear output = %q", got)
	}
	primaryDiscoveries, err := ListWebDiscoveries(workspace, primary)
	if err != nil {
		t.Fatal(err)
	}
	if len(primaryDiscoveries) != 0 {
		t.Fatalf("primary discoveries = %d, want 0", len(primaryDiscoveries))
	}
	secondaryDiscoveries, err := ListWebDiscoveries(workspace, secondary)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondaryDiscoveries) != 1 {
		t.Fatalf("secondary discoveries = %d, want 1", len(secondaryDiscoveries))
	}
	primaryRuns, err := ListWebWordlistRunsForTarget(workspace, primary)
	if err != nil {
		t.Fatal(err)
	}
	if len(primaryRuns) != 0 {
		t.Fatalf("primary wordlist runs = %d, want 0", len(primaryRuns))
	}
	secondaryRuns, err := ListWebWordlistRunsForTarget(workspace, secondary)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondaryRuns) != 1 {
		t.Fatalf("secondary wordlist runs = %d, want 1", len(secondaryRuns))
	}
	for _, path := range primaryCaches {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("primary cache %s stat error = %v, want not exist", path, err)
		}
	}
	for _, path := range secondaryCaches {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("secondary cache %s stat error = %v, want present", path, err)
		}
	}
	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].ID != logID {
		t.Fatalf("command logs after clear = %+v, want referenced log retained", logs)
	}

	out.Reset()
	workspaceStdin = strings.NewReader("yes\n")
	if err := Run([]string{"ctx", "web", "clear", "--target", "secondary"}, &out); err != nil {
		t.Fatalf("Run(ctx web clear --target secondary) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Cleared web discovery data for target: secondary") || !strings.Contains(got, "Discoveries: 1") || !strings.Contains(got, "Wordlist runs: 1") || !strings.Contains(got, "Wordlist cache: removed") {
		t.Fatalf("selected clear output = %q", got)
	}
	for _, path := range secondaryCaches {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("secondary cache %s stat error = %v, want not exist", path, err)
		}
	}
}

func TestRunWebClearCancelsWithoutExplicitConfirmation(t *testing.T) {
	workspace := initXTestWorkspace(t)
	target, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveWebDiscovery(workspace, target, WebDiscovery{URL: "http://10.10.10.10/admin", Path: "/admin", StatusCode: 200, SourceTool: "gobuster"}); err != nil {
		t.Fatal(err)
	}
	if _, err := StartWebWordlistRun(workspace, target, "http://10.10.10.10", "wordlists", "web-quick", "", "/usr/share/dirb/wordlists/common.txt", "2026-07-20T00:00:00Z", 0); err != nil {
		t.Fatal(err)
	}
	cachePaths := webDiscoveryCachePaths(workspace, target)
	for _, cachePath := range cachePaths {
		if err := os.MkdirAll(cachePath, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cachePath, "searched.words"), []byte("admin\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	oldStdin := workspaceStdin
	workspaceStdin = strings.NewReader("\n")
	t.Cleanup(func() { workspaceStdin = oldStdin })

	var out bytes.Buffer
	if err := Run([]string{"ctx", "web", "clear"}, &out); err != nil {
		t.Fatalf("Run(ctx web clear) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "[y/N]:") || !strings.Contains(got, "cancelled") {
		t.Fatalf("cancel output = %q", got)
	}
	discoveries, err := ListWebDiscoveries(workspace, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(discoveries) != 1 {
		t.Fatalf("discoveries after cancellation = %d, want 1", len(discoveries))
	}
	runs, err := ListWebWordlistRunsForTarget(workspace, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("wordlist runs after cancellation = %d, want 1", len(runs))
	}
	for _, cachePath := range cachePaths {
		if _, err := os.Stat(cachePath); err != nil {
			t.Fatalf("cache %s after cancellation stat error = %v, want present", cachePath, err)
		}
	}
}

func TestIsolateWebDiscoveryCachesRestoresEarlierCacheOnFailure(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "web-wordlists", "1")
	if err := os.MkdirAll(first, 0755); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(root, "xffuf-vhost", "1")
	if err := os.MkdirAll(second, 0755); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := isolateWebDiscoveryCaches([]string{first, second, filepath.Join(blocker, "xffuf")}); err == nil {
		t.Fatal("isolateWebDiscoveryCaches() error = nil, want failure")
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("first cache was not restored: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("second cache was not restored: %v", err)
	}
	for _, path := range []string{first, second} {
		matches, err := filepath.Glob(path + ".clearing-*")
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("isolated cache remains after rollback: %#v", matches)
		}
	}
}

func TestColorizeHTTPStatus(t *testing.T) {
	if got := ColorizeHTTPStatus(200, true); got != "\033[32m200\033[0m" {
		t.Fatalf("ColorizeHTTPStatus(200) = %q", got)
	}
	if got := ColorizeHTTPStatus(404, false); got != "404" {
		t.Fatalf("ColorizeHTTPStatus(404, false) = %q", got)
	}
}
