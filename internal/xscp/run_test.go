package xscp

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"req/internal/ctxexec"
)

type fakeRunner struct {
	outputs   map[string]string
	runInputs map[string][]byte
	runs      []string
}

func (runner *fakeRunner) LookPath(name string) (string, error) {
	if name == ctxexec.ExecutablePath || name == "scp" {
		return name, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeRunner) Run(name string, args []string, _ []string, stdin io.Reader, stdout, _ io.Writer) error {
	if name == ctxexec.ExecutablePath {
		name = "ctx"
	}
	key := name + " " + strings.Join(args, " ")
	if name == "ctx" {
		if stdin != nil {
			input, _ := io.ReadAll(stdin)
			runner.runInputs[key] = input
		}
		output, ok := runner.outputs[key]
		if !ok {
			return errors.New("unexpected ctx command: " + key)
		}
		_, _ = io.WriteString(stdout, output)
		return nil
	}
	runner.runs = append(runner.runs, key)
	return nil
}

func apiJSON(data string) string {
	return `{"success":true,"format_version":"1.0","data":` + data + `,"error":null}`
}

func TestParseOptions(t *testing.T) {
	options, err := parseOptions([]string{"upload", "./local.txt", "--port", "2222", "--service", "2"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Action != "upload" || options.Source != "./local.txt" || options.Destination != "local.txt" || options.Port != "2222" || options.Service != "2" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseOptionsUsesExplicitDestination(t *testing.T) {
	options, err := parseOptions([]string{"download", "/tmp/remote.txt", "./copy.txt", "root"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Destination != "./copy.txt" || options.Credential != "root" {
		t.Fatalf("options = %+v", options)
	}
}

func TestSCPArgsUpload(t *testing.T) {
	password := "secret"
	credential := &Credential{Username: "kali", Password: &password}
	got := (&App{}).scpArgs("upload", "./local.txt", "/tmp/remote.txt", credential, "10.0.0.5", 2222)
	want := []string{"-P", "2222", "./local.txt", "kali@10.0.0.5:/tmp/remote.txt"}
	if len(got) != len(want) {
		t.Fatalf("scpArgs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scpArgs() = %#v, want %#v", got, want)
		}
	}
}

func TestSCPArgsDownloadWithoutCredential(t *testing.T) {
	got := (&App{}).scpArgs("download", "./local.txt", "/tmp/remote.txt", nil, "10.0.0.5", 22)
	want := []string{"-P", "22", "10.0.0.5:/tmp/remote.txt", "./local.txt"}
	if len(got) != len(want) {
		t.Fatalf("scpArgs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scpArgs() = %#v, want %#v", got, want)
		}
	}
}

func TestSCPLogCommandDoesNotIncludePassword(t *testing.T) {
	password := "secret"
	command := scpLogCommand("upload", "./local.txt", "/tmp/remote.txt", &Credential{Username: "kali", Password: &password}, "10.0.0.5", 22)
	if command != "scp -P 22 ./local.txt kali@10.0.0.5:/tmp/remote.txt" {
		t.Fatalf("scpLogCommand() = %q", command)
	}
	if command == "secret" {
		t.Fatalf("scpLogCommand() leaked password")
	}
}

func TestRunUsesSharedJSONClientAndRecordsLog(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"ctx prompt --format json --format-version 1":            apiJSON(`{"active":true,"target_ip":"10.0.0.5"}`),
			"ctx credential ls ssh --format json --format-version 1": apiJSON(`{"credentials":[]}`),
			"ctx service ls --format json --format-version 1":        apiJSON(`{"services":[]}`),
			"ctx log start --format json --format-version 1":         apiJSON(`{"id":42}`),
			"ctx log finish 42 --format json --format-version 1":     apiJSON(`{"id":42}`),
		},
		runInputs: map[string][]byte{},
	}
	var stdout, stderr bytes.Buffer
	app := New(runner, strings.NewReader(""), &stdout, &stderr)
	app.logger = ctxCommandLogger{runner: runner}

	if err := app.run([]string{"xscp", "upload", "local.txt"}); err != nil {
		t.Fatalf("run() error = %v, stderr = %q", err, stderr.String())
	}
	if len(runner.runs) != 1 || runner.runs[0] != "scp -P 22 local.txt 10.0.0.5:local.txt" {
		t.Fatalf("runs = %#v", runner.runs)
	}
	if len(runner.runInputs["ctx log start --format json --format-version 1"]) == 0 {
		t.Fatal("log start JSON input was not sent")
	}
}
