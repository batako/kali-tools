package xgobuster

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"req/internal/ctx"
	"req/internal/ctxexec"
)

type ctxReadRunner struct {
	outputs map[string]string
}

func (runner *ctxReadRunner) LookPath(name string) (string, error) {
	return name, nil
}

func (runner *ctxReadRunner) Run(name string, args []string, _ io.Reader, stdout, _ io.Writer) error {
	if name != ctxexec.ExecutablePath {
		return errors.New("unexpected executable: " + name)
	}
	key := strings.Join(args, " ")
	output, ok := runner.outputs[key]
	if !ok {
		return errors.New("unexpected ctx command: " + key)
	}
	_, _ = io.WriteString(stdout, output)
	return nil
}

func ctxAPIJSON(data string) string {
	return `{"success":true,"format_version":"1.0","data":` + data + `,"error":null}`
}

func TestPromptAndServicesUseSharedJSONClient(t *testing.T) {
	runner := &ctxReadRunner{outputs: map[string]string{
		"prompt --format json --format-version 1":     ctxAPIJSON(`{"active":true,"target_ip":"10.0.0.5","workspace_path":"/tmp/workspace"}`),
		"service ls --format json --format-version 1": ctxAPIJSON(`{"services":[{"port":80,"protocol":"tcp","service_name":"http"}]}`),
	}}
	app := New(runner, strings.NewReader(""), io.Discard, io.Discard)

	prompt, err := app.prompt()
	if err != nil || prompt.TargetIP == nil || *prompt.TargetIP != "10.0.0.5" {
		t.Fatalf("prompt() = %+v, %v", prompt, err)
	}
	services, err := app.services()
	if err != nil || len(services) != 1 || services[0].Port != 80 {
		t.Fatalf("services() = %+v, %v", services, err)
	}
}

func TestParseOptionsUsesExplicitWordlistAndURL(t *testing.T) {
	options, err := parseOptions([]string{
		"-w", "/tmp/list.txt",
		"--url=https://example.test/app",
		"-x", "php",
	})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Wordlist != "/tmp/list.txt" {
		t.Fatalf("wordlist = %q, want explicit path", options.Wordlist)
	}
	if options.URL != "https://example.test/app" {
		t.Fatalf("URL = %q, want explicit URL", options.URL)
	}
	if strings.Join(options.Extra, " ") != "-x php" {
		t.Fatalf("extra = %#v, want gobuster options without wrapper options", options.Extra)
	}
}

func TestParseOptionsSupportsDNSMode(t *testing.T) {
	options, err := parseOptions([]string{"dns", "--domain", "example.test", "-w", "/tmp/dns.txt", "-t", "25"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if !options.DNS {
		t.Fatalf("DNS mode = false, want true")
	}
	if options.Domain != "example.test" {
		t.Fatalf("domain = %q, want example.test", options.Domain)
	}
	if options.Wordlist != "/tmp/dns.txt" {
		t.Fatalf("wordlist = %q, want explicit path", options.Wordlist)
	}
	if strings.Join(options.Extra, " ") != "-t 25" {
		t.Fatalf("extra = %#v, want gobuster options", options.Extra)
	}

	options, err = parseOptions([]string{"dns", "-d", "example.test"})
	if err != nil || options.Domain != "example.test" {
		t.Fatalf("domain option = %+v, %v", options, err)
	}
	options, err = parseOptions([]string{"dns", "--domain=example.test"})
	if err != nil || options.Domain != "example.test" {
		t.Fatalf("domain equals option = %+v, %v", options, err)
	}
}

func TestParseOptionsSupportsHostAndIP(t *testing.T) {
	options, err := parseOptions([]string{"--host", "example.test"})
	if err != nil || options.Host != "example.test" || options.IP {
		t.Fatalf("host options = %+v, %v", options, err)
	}
	options, err = parseOptions([]string{"--ip"})
	if err != nil || !options.IP || options.Host != "" {
		t.Fatalf("IP options = %+v, %v", options, err)
	}
}

func TestParseOptionsSupportsServiceSelection(t *testing.T) {
	options, err := parseOptions([]string{"--service", "2"})
	if err != nil || options.Service != 2 {
		t.Fatalf("service options = %+v, %v", options, err)
	}
	options, err = parseOptions([]string{"--service=3"})
	if err != nil || options.Service != 3 {
		t.Fatalf("service equals options = %+v, %v", options, err)
	}
}

func TestParseOptionsSupportsInsecureTLS(t *testing.T) {
	options, err := parseOptions([]string{"-k"})
	if err != nil || !options.Insecure {
		t.Fatalf("insecure options = %+v, %v", options, err)
	}
	options, err = parseOptions([]string{"--no-tls-validation"})
	if err != nil || !options.Insecure {
		t.Fatalf("long insecure options = %+v, %v", options, err)
	}
	options, err = parseOptions([]string{"--tls-verify"})
	if err != nil || !options.VerifyTLS {
		t.Fatalf("verify TLS options = %+v, %v", options, err)
	}
}

func TestParseOptionsSupportsCookies(t *testing.T) {
	options, err := parseOptions([]string{"-c", "PHPSESSID=abc; theme=dark"})
	if err != nil || options.Cookie != "PHPSESSID=abc; theme=dark" {
		t.Fatalf("cookie options = %+v, %v", options, err)
	}
	if got := strings.Join(effectiveExtra(options), " "); got != "--cookies PHPSESSID=abc; theme=dark" {
		t.Fatalf("effectiveExtra() = %q", got)
	}
}

func TestParseOptionsSupportsExcludedResponseLengths(t *testing.T) {
	options, err := parseOptions([]string{"--exclude-length", "274,512"})
	if err != nil || options.ExcludeLength != "274,512" {
		t.Fatalf("exclude length options = %+v, %v", options, err)
	}
	if got := strings.Join(effectiveExtra(options), " "); got != "--exclude-length 274,512" {
		t.Fatalf("effectiveExtra() = %q", got)
	}
}

func TestParseOptionsSupportsExcludedResponseStatuses(t *testing.T) {
	options, err := parseOptions([]string{"--exclude-status", "401,403"})
	if err != nil || options.ExcludeStatus != "401,403" {
		t.Fatalf("exclude status options = %+v, %v", options, err)
	}
	if got := strings.Join(effectiveExtra(options), " "); got != "--status-codes-blacklist 404,401,403" {
		t.Fatalf("effectiveExtra() = %q", got)
	}

	options, err = parseOptions([]string{"--exclude-status=404,401"})
	if err != nil || options.ExcludeStatus != "404,401" {
		t.Fatalf("exclude status equals option = %+v, %v", options, err)
	}
	if got := strings.Join(effectiveExtra(options), " "); got != "--status-codes-blacklist 404,401" {
		t.Fatalf("effectiveExtra() with explicit 404 = %q", got)
	}
}

func TestResponseExclusionsChangeSearchSignature(t *testing.T) {
	base := searchSignature(parsedOptions{})
	status := searchSignature(parsedOptions{ExcludeStatus: "401"})
	length := searchSignature(parsedOptions{ExcludeLength: "0"})
	if base == status || base == length || status == length {
		t.Fatalf("search signatures are not distinct: base=%q status=%q length=%q", base, status, length)
	}
}

func TestEffectiveExtraAddsInsecureTLSFlag(t *testing.T) {
	got := effectiveExtra(parsedOptions{Insecure: true})
	if strings.Join(got, " ") != "-k" {
		t.Fatalf("effectiveExtra() = %#v, want -k", got)
	}
}

func TestParseOptionsRejectsRemovedProgressFlags(t *testing.T) {
	for _, args := range [][]string{{"--next"}, {"--force"}, {"--profile", "web-quick"}, {"--profile=web-deep"}} {
		if _, err := parseOptions(args); err == nil || !strings.Contains(err.Error(), "was removed") {
			t.Fatalf("parseOptions(%#v) error = %v, want removed option error", args, err)
		}
	}
}

func TestParseOptionsSupportsStatus(t *testing.T) {
	options, err := parseOptions([]string{"--status"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if !options.Status {
		t.Fatalf("options = %+v, want status", options)
	}
}

func TestParseOptionsSupportsDNSCacheClear(t *testing.T) {
	options, err := parseOptions([]string{"dns", "--clear-cache", "-d", "example.test"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if !options.DNS || !options.ClearCache || options.Domain != "example.test" {
		t.Fatalf("options = %+v, want DNS cache clear", options)
	}
}

func TestParseDNSHosts(t *testing.T) {
	got := parseDNSHosts("Found: admin.example.test\nFound: admin.example.test.\nFound: example.test\n", "example.test")
	if strings.Join(got, ",") != "admin.example.test" {
		t.Fatalf("parseDNSHosts() = %#v, want one subdomain", got)
	}
}

func TestSearchModeDefaultsToDirectory(t *testing.T) {
	if got := searchModeFromOptions(parsedOptions{}); got != "directory" {
		t.Fatalf("searchModeFromOptions() = %q, want directory", got)
	}
}

func TestSearchModeUsesFileForExtensions(t *testing.T) {
	if got := searchModeFromOptions(parsedOptions{Extra: []string{"-x", "php,js"}}); got != "file" {
		t.Fatalf("searchModeFromOptions() = %q, want file", got)
	}
}

func TestEffectiveExtraUsesPresetExtensions(t *testing.T) {
	options := parsedOptions{PresetExtensions: "php,js"}
	got := effectiveExtra(options)
	if strings.Join(got, " ") != "-x php,js" {
		t.Fatalf("effectiveExtra() = %#v, want explicit extensions", got)
	}
}

func TestSearchSignatureSeparatesExtensionSearches(t *testing.T) {
	directory := searchSignature(parsedOptions{})
	file := searchSignature(parsedOptions{Extra: []string{"-x", "php"}})
	if directory == file {
		t.Fatalf("search signatures are equal: directory=%q file=%q", directory, file)
	}
}

func TestExtensionsFromExtra(t *testing.T) {
	got := extensionsFromExtra([]string{"-x", "php,.js,php"})
	if strings.Join(got, ",") != "php,js" {
		t.Fatalf("extensionsFromExtra() = %#v, want php and js", got)
	}
}

func TestWithoutExtensionsOption(t *testing.T) {
	got := withoutExtensionsOption([]string{"-t", "10", "-x", "php,js", "-r"})
	if strings.Join(got, " ") != "-t 10 -r" {
		t.Fatalf("withoutExtensionsOption() = %#v, want non-extension options", got)
	}
}

func TestParseDiscoveries(t *testing.T) {
	discoveries := parseDiscoveries(
		"admin                (Status: 302) [Size: 128] [--> http://10.0.0.1/admin/]\nlogin (Status: 200)\nnot a result\n",
		"http://10.0.0.1",
		"/usr/share/wordlists/dirb/common.txt",
		7,
	)
	if len(discoveries) != 2 {
		t.Fatalf("discoveries = %#v, want two results", discoveries)
	}
	if discoveries[0].URL != "http://10.0.0.1/admin" || discoveries[0].StatusCode != 302 || discoveries[0].ContentLength != 128 {
		t.Fatalf("first discovery = %+v", discoveries[0])
	}
	if discoveries[1].Path != "/login" || discoveries[1].ContentLengthValid {
		t.Fatalf("second discovery = %+v", discoveries[1])
	}
	if discoveries[0].CommandLogID != 7 || discoveries[0].SourceTool != "gobuster" {
		t.Fatalf("discovery metadata = %+v", discoveries[0])
	}
}

func TestParseDiscoveriesStoresURLRelativePath(t *testing.T) {
	discoveries := parseDiscoveries(
		"dashboard.php (Status: 200) [Size: 12]\n",
		"https://grep.thm/public/html/",
		"/tmp/words.txt",
		0,
	)
	if len(discoveries) != 1 {
		t.Fatalf("discoveries = %#v, want one result", discoveries)
	}
	if discoveries[0].URL != "https://grep.thm/public/html/dashboard.php" {
		t.Fatalf("URL = %q", discoveries[0].URL)
	}
	if discoveries[0].Path != "/public/html/dashboard.php" {
		t.Fatalf("Path = %q, want URL-root-relative path", discoveries[0].Path)
	}
}

func TestFilteredWordlistExcludesPreviouslySearchedWords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "words.txt")
	if err := os.WriteFile(path, []byte("admin\nlogin\nadmin\napi\n\n"), 0644); err != nil {
		t.Fatal(err)
	}
	words, err := filteredWordlist(path, map[string]struct{}{"admin": {}})
	if err != nil {
		t.Fatalf("filteredWordlist() error = %v", err)
	}
	if strings.Join(words, ",") != "login,api" {
		t.Fatalf("filtered words = %#v, want login and api", words)
	}
}

func TestFilteredWordlistCanShareStateWithinOneCommand(t *testing.T) {
	firstPath := filepath.Join(t.TempDir(), "first.txt")
	secondPath := filepath.Join(t.TempDir(), "second.txt")
	if err := os.WriteFile(firstPath, []byte("admin\nlogin\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("admin\napi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]struct{})
	first, err := filteredWordlist(firstPath, seen)
	if err != nil {
		t.Fatalf("filteredWordlist(first) error = %v", err)
	}
	for _, word := range first {
		seen[word] = struct{}{}
	}
	second, err := filteredWordlist(secondPath, seen)
	if err != nil {
		t.Fatalf("filteredWordlist(second) error = %v", err)
	}
	if strings.Join(second, ",") != "api" {
		t.Fatalf("second filtered words = %#v, want api", second)
	}
}

func TestSearchedWordStateIsPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "searched.words")
	if err := appendSearchedWords(path, []string{"admin", "api"}); err == nil {
		t.Fatal("appendSearchedWords() error = nil for missing directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := appendSearchedWords(path, []string{"admin", "api"}); err != nil {
		t.Fatalf("appendSearchedWords() error = %v", err)
	}
	seen, err := loadSearchedWords(path)
	if err != nil {
		t.Fatalf("loadSearchedWords() error = %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("seen = %#v, want two words", seen)
	}
}

func TestClearWebDiscoveryDataRemovesXgobusterSearchState(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := ctx.InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target, err := ctx.SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://10.10.10.10/"
	if _, err := ctx.SaveWebDiscovery(workspace, target, ctx.WebDiscovery{URL: url + "admin", Path: "/admin", StatusCode: 200, SourceTool: "gobuster"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.StartWebWordlistRun(workspace, target, url, "wordlists", "web-quick", "", "/usr/share/dirb/wordlists/common.txt", "2026-07-20T00:00:00Z", 0); err != nil {
		t.Fatal(err)
	}
	statePath, err := searchedBaseWordsPath(workspace, target.ID, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendSearchedWords(statePath, []string{"admin", "login"}); err != nil {
		t.Fatal(err)
	}

	cleared, err := ctx.ClearWebDiscoveryData(workspace, target)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Discoveries != 1 || cleared.WordlistRuns != 1 || !cleared.CachePresent {
		t.Fatalf("cleared summary = %+v", cleared)
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(statePath))); !os.IsNotExist(err) {
		t.Fatalf("xgobuster target cache stat error = %v, want not exist", err)
	}
	runs, err := ctx.ListWebWordlistRunsForTarget(workspace, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("wordlist runs = %d, want 0", len(runs))
	}
}

func TestCompatibleWebURLOnlyChangesIPv4Host(t *testing.T) {
	tests := []struct {
		current, previous string
		want              bool
	}{
		{"http://10.10.10.20/app/", "http://10.10.10.10/app/", true},
		{"https://10.10.10.20:8443/app/", "https://10.10.10.10:8443/app/", true},
		{"http://10.10.10.20/app/", "http://10.10.10.10/other/", false},
		{"http://target.thm/app/", "http://10.10.10.10/app/", false},
		{"http://10.10.10.20/app/", "https://10.10.10.10/app/", false},
	}
	for _, tt := range tests {
		if got := compatibleWebURL(tt.current, tt.previous); got != tt.want {
			t.Errorf("compatibleWebURL(%q, %q) = %t, want %t", tt.current, tt.previous, got, tt.want)
		}
	}
}

func TestLoadSearchedWordsForChangedIPv4URL(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := ctx.InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	target, err := ctx.SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	oldURL := "http://10.10.10.10/app/"
	path, err := searchedBaseWordsPath(workspace, target.ID, oldURL)
	if err != nil {
		t.Fatalf("searchedBaseWordsPath() error = %v", err)
	}
	if err := appendSearchedWords(path, []string{"admin", "login"}); err != nil {
		t.Fatalf("appendSearchedWords() error = %v", err)
	}

	seen, err := loadSearchedWordsForURLs(workspace, target.ID, []string{"http://10.10.10.20/app/", oldURL}, "base", "")
	if err != nil {
		t.Fatalf("loadSearchedWordsForURLs() error = %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("seen = %#v, want two words", seen)
	}
}

func TestClearSearchedWordsForURLsRemovesBaseLegacyAndExtensionState(t *testing.T) {
	workspace := &ctx.Workspace{DataPath: t.TempDir()}
	urls := []string{"http://10.10.10.10/", "http://10.10.10.20/"}
	extra := []string{"-x", "php"}
	for _, rawURL := range urls {
		base, err := searchedBaseWordsPath(workspace, 1, rawURL)
		if err != nil {
			t.Fatal(err)
		}
		legacy, err := searchedWordsPath(workspace, 1, rawURL, extra)
		if err != nil {
			t.Fatal(err)
		}
		extension, err := searchedExtensionWordsPath(workspace, 1, rawURL, "php")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{base, legacy, extension} {
			if err := appendSearchedWords(path, []string{"admin"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := clearSearchedWordsForURLs(workspace, 1, urls, extra); err != nil {
		t.Fatal(err)
	}
	seen, err := loadSearchedWordsForURLs(workspace, 1, urls, "base", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Fatalf("base state = %#v, want empty", seen)
	}
	seen, err = loadSearchedWordsForURLs(workspace, 1, urls, "extension", "php")
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Fatalf("extension state = %#v, want empty", seen)
	}
}

func TestDiscoverDNSWordlistsUsesCtxRecommendation(t *testing.T) {
	original := recommendWordlists
	t.Cleanup(func() { recommendWordlists = original })
	requested := ""
	recommendWordlists = func(kind string) ([]ctx.WordlistSelection, error) {
		requested = kind
		return []ctx.WordlistSelection{{Path: "/lists/subdomains.txt", Tier: ctx.WordlistTierQuick}}, nil
	}
	candidates, err := discoverDNSWordlists()
	if err != nil {
		t.Fatal(err)
	}
	if requested != ctx.WordlistKindSubdomain || len(candidates) != 1 || candidates[0].Profile != "dns-quick" || candidates[0].Type != "dns" {
		t.Fatalf("candidates = %+v, requested = %q", candidates, requested)
	}
}
