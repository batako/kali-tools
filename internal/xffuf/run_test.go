package xffuf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"req/internal/ctx"
)

type resultRunner struct {
	count int
}

func (runner resultRunner) LookPath(string) (string, error) {
	return "/usr/bin/ffuf", nil
}

func (runner resultRunner) Run(_ string, args []string, _ io.Reader, _, _ io.Writer) error {
	resultPath := ""
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "-o" {
			resultPath = args[index+1]
			break
		}
	}
	if resultPath == "" {
		return fmt.Errorf("ffuf result path is missing")
	}

	output := ffufOutput{Results: make([]ffufResult, 0, runner.count)}
	for index := 0; index < runner.count; index++ {
		output.Results = append(output.Results, ffufResult{
			Input: map[string]string{"FUZZ": fmt.Sprintf("host-%d", index)},
		})
	}
	content, err := json.Marshal(output)
	if err != nil {
		return err
	}
	return os.WriteFile(resultPath, content, 0600)
}

func TestParseOptions(t *testing.T) {
	options, err := parseOptions([]string{"vhost", "--domain", "example.test", "--service", "2", "--suggest", "-H", "X-Test: one"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Domain != "example.test" || options.Service != 2 || !options.Suggest {
		t.Fatalf("parseOptions() = %+v", options)
	}
	if len(options.Extra) != 2 || options.Extra[0] != "-H" || options.Extra[1] != "X-Test: one" {
		t.Fatalf("parseOptions() extra = %#v", options.Extra)
	}
}

func TestParseOptionsRejectsRemovedProfile(t *testing.T) {
	for _, args := range [][]string{{"param", "--profile", "parameter-value-url"}, {"param", "--profile=parameter-value-file"}} {
		if _, err := parseOptions(args); err == nil || !strings.Contains(err.Error(), "--profile was removed") {
			t.Fatalf("parseOptions(%v) error = %v", args, err)
		}
	}
}

func TestParseOptionsRejectsRemovedProgressFlags(t *testing.T) {
	for _, flag := range []string{"--next", "--force"} {
		if _, err := parseOptions([]string{"vhost", flag}); err == nil || !strings.Contains(err.Error(), "was removed") {
			t.Fatalf("parseOptions(%q) error = %v, want removed option error", flag, err)
		}
	}
}

func TestParseOptionsKeepsManualFilterForValidation(t *testing.T) {
	options, err := parseOptions([]string{"vhost", "--suggest", "-fw", "125"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if !options.Suggest || len(manualFilters(options.Extra)) != 2 {
		t.Fatalf("parseOptions() = %+v", options)
	}
}

func TestManualMatchers(t *testing.T) {
	options, err := parseOptions([]string{"param", "-mc", "301,302,303,307,308", "-ms", "42"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if got := strings.Join(manualMatchers(options.Extra), " "); got != "-mc 301,302,303,307,308 -ms 42" {
		t.Fatalf("manualMatchers() = %q", got)
	}
	if useParamAutoFilter(options) {
		t.Fatal("useParamAutoFilter() = true with explicit matchers")
	}
}

func TestParseOptionsTrial(t *testing.T) {
	options, err := parseOptions([]string{"vhost", "--trial", "-fw", "125"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if !options.Trial || strings.Join(options.Extra, " ") != "-fw 125" {
		t.Fatalf("parseOptions() = %+v", options)
	}
	if _, err := parseOptions([]string{"vhost", "--trial", "--suggest"}); err == nil {
		t.Fatal("parseOptions() accepted --trial with --suggest")
	}
}

func TestConfirmTrial(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{name: "yes", input: "y\n", want: true},
		{name: "default no", input: "\n", want: false},
		{name: "explicit no", input: "no\n", want: false},
		{name: "invalid", input: "maybe\n", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			got, err := confirmTrial(strings.NewReader(test.input), &output, []string{"-fw", "125"})
			if (err != nil) != test.wantErr {
				t.Fatalf("confirmTrial() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("confirmTrial() = %t, want %t", got, test.want)
			}
			if !strings.Contains(output.String(), "-fw 125") {
				t.Fatalf("prompt = %q", output.String())
			}
		})
	}
}

func TestStats(t *testing.T) {
	result := stats([]int{125, 125, 125, 126, 125, 125, 125, 125, 126, 125})
	if result.Min != 125 || result.Max != 126 || result.Median != 125 || result.Mode != 125 || result.Stable != 8 || result.Total != 10 {
		t.Fatalf("stats() = %+v", result)
	}
}

func TestFilterProposalPrefersStableWordsOverReflectedSize(t *testing.T) {
	result := calibration{
		Size:   stats([]int{920, 923, 922, 919, 925, 930, 936, 921, 924, 928}),
		Words:  stats([]int{125, 125, 125, 125, 125, 125, 125, 125, 125, 125}),
		Lines:  stats([]int{25, 25, 25, 25, 25, 25, 25, 25, 25, 25}),
		Status: stats([]int{200, 200, 200, 200, 200, 200, 200, 200, 200, 200}),
	}
	filter := filterProposal(result, 90)
	if strings.Join(filter, " ") != "-fw 125" {
		t.Fatalf("filterProposal() = %#v", filter)
	}
}

func TestFilterProposalFallsBackToStableSize(t *testing.T) {
	result := calibration{
		Size:  stats([]int{8421, 8421, 8421, 8421, 8421, 8421, 8421, 8421, 8421, 8422}),
		Words: stats([]int{125, 126, 127, 128, 129, 130, 131, 132, 133, 134}),
		Lines: stats([]int{18, 19, 20, 21, 22, 23, 24, 25, 26, 27}),
	}
	filter := filterProposal(result, 90)
	if strings.Join(filter, " ") != "-fs 8421" {
		t.Fatalf("filterProposal() = %#v", filter)
	}
}

func TestCalibrationWordsUseVariedLengths(t *testing.T) {
	words := calibrationWords(10)
	if len(words) != 10 {
		t.Fatalf("len(calibrationWords()) = %d", len(words))
	}
	if len(words[0]) != 6 || len(words[len(words)-1]) != 48 {
		t.Fatalf("calibration word lengths = %d..%d", len(words[0]), len(words[len(words)-1]))
	}
	lengths := make(map[int]struct{}, len(words))
	for _, word := range words {
		lengths[len(word)] = struct{}{}
	}
	if len(lengths) != len(words) {
		t.Fatalf("calibration word lengths are not varied: %#v", words)
	}
}

func TestReadResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	content := `{"results":[{"input":{"FUZZ":"admin"},"status":200,"length":8421,"words":125,"lines":18,"host":"admin.example.test"}]}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	results, err := readResults(path)
	if err != nil {
		t.Fatalf("readResults() error = %v", err)
	}
	if len(results) != 1 || results[0].Input["FUZZ"] != "admin" || results[0].Status != 200 {
		t.Fatalf("readResults() = %#v", results)
	}
}

func TestRunScanRecordsSuspiciousResultsAsFailed(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := ctx.InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	target, err := ctx.SetPrimaryTargetIP(workspace, "192.0.2.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	words := make([]string, 101)
	for index := range words {
		words[index] = fmt.Sprintf("host-%d", index)
	}
	app := New(resultRunner{count: len(words)}, strings.NewReader(""), io.Discard, io.Discard)
	selection := ctx.WordlistSelection{
		Provider: ctx.WordlistProviderLists,
		Profile:  "vhost",
		Path:     "/usr/share/wordlists/seclists/Discovery/DNS/test.txt",
	}
	err = app.runScan(
		workspace,
		target,
		"http://192.0.2.10",
		"example.test",
		selection,
		words,
		options{},
		nil,
		filepath.Join(t.TempDir(), "searched.words"),
		[]string{"vhost"},
	)
	if err == nil || !strings.Contains(err.Error(), "too many vhost hits") {
		t.Fatalf("runScan() error = %v", err)
	}

	logs, err := ctx.ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 || logs[0].Status != "failed" || logs[0].ExitCode != 1 {
		t.Fatalf("command logs = %+v", logs)
	}
	if !strings.Contains(logs[0].Stderr, "too many vhost hits") {
		t.Fatalf("command log stderr = %q", logs[0].Stderr)
	}

	runs, err := ctx.ListWebWordlistRunsForTarget(workspace, target)
	if err != nil {
		t.Fatalf("ListWebWordlistRunsForTarget() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("wordlist runs = %+v", runs)
	}
}

func TestRunScanTrialDoesNotPersistResults(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := ctx.InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	target, err := ctx.SetPrimaryTargetIP(workspace, "192.0.2.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "searched.words")
	var stdout bytes.Buffer
	app := New(resultRunner{count: 1}, strings.NewReader(""), &stdout, io.Discard)
	err = app.runScan(
		workspace,
		target,
		"http://192.0.2.10",
		"example.test",
		ctx.WordlistSelection{
			Provider: ctx.WordlistProviderLists,
			Profile:  "vhost",
			Path:     "/usr/share/wordlists/seclists/Discovery/DNS/test.txt",
		},
		[]string{"admin"},
		options{Trial: true},
		[]string{"-fw", "125"},
		statePath,
		[]string{"vhost", "--trial", "-fw", "125"},
	)
	if err != nil {
		t.Fatalf("runScan() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Trial completed") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("trial cache exists: %v", err)
	}

	logs, err := ctx.ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("command logs = %+v", logs)
	}
	runs, err := ctx.ListWebWordlistRunsForTarget(workspace, target)
	if err != nil {
		t.Fatalf("ListWebWordlistRunsForTarget() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("wordlist runs = %+v", runs)
	}
	hosts, err := ctx.ListHosts(workspace)
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("hosts = %+v", hosts)
	}
}
