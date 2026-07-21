package xftp

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"req/internal/ctxexec"
)

type fakeRunner struct {
	paths     map[string]bool
	outputs   map[string]fakeOutput
	runInputs map[string][]byte
	runs      []fakeRun
}

type fakeOutput struct {
	stdout string
	stderr string
	err    error
}

type fakeRun struct {
	name string
	args []string
	env  []string
}

type memoryCommandLogger struct {
	startCommand  string
	startExpanded string
	startAt       string
	finishID      int64
	finishStatus  string
	finishCode    int
	finishStdout  string
	finishStderr  string
	finishAt      string
}

func (logger *memoryCommandLogger) Start(command, expandedCommand, startedAt string) (int64, error) {
	logger.startCommand = command
	logger.startExpanded = expandedCommand
	logger.startAt = startedAt
	return 42, nil
}

func (logger *memoryCommandLogger) Finish(id int64, status string, exitCode int, stdout, stderr, endedAt string) error {
	logger.finishID = id
	logger.finishStatus = status
	logger.finishCode = exitCode
	logger.finishStdout = stdout
	logger.finishStderr = stderr
	logger.finishAt = endedAt
	return nil
}

func (runner *fakeRunner) LookPath(file string) (string, error) {
	file = fakeCommandName(file)
	if runner.paths[file] {
		return "/fake/" + file, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeRunner) Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	name = fakeCommandName(name)
	key := name + " " + strings.Join(args, " ")
	if name == "ctx" {
		if stdin != nil {
			input, _ := io.ReadAll(stdin)
			if runner.runInputs == nil {
				runner.runInputs = map[string][]byte{}
			}
			runner.runInputs[key] = input
		}
		if output, ok := runner.outputs[key]; ok {
			_, _ = io.WriteString(stdout, output.stdout)
			_, _ = io.WriteString(stderr, output.stderr)
			return output.err
		}
	}
	runner.runs = append(runner.runs, fakeRun{
		name: name,
		args: append([]string(nil), args...),
		env:  append([]string(nil), env...),
	})
	return nil
}

func fakeCommandName(name string) string {
	if name == ctxexec.ExecutablePath {
		return "ctx"
	}
	return name
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		paths: map[string]bool{
			"ctx":  true,
			"lftp": true,
		},
		outputs: map[string]fakeOutput{
			"ctx prompt --format json --format-version 1": {
				stdout: apiJSON(`{"active":true,"target_ip":"2.3.4.5","target_name":"default"}`),
			},
			"ctx credential ls ftp --format json --format-version 1": {
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ftp","username":"root","password":"toor"}]}`),
			},
			"ctx service ls --format json --format-version 1": {
				stdout: apiJSON(`{"services":[]}`),
			},
		},
	}
}

func apiJSON(data string) string {
	return `{"success":true,"format_version":"1.0","data":` + data + `,"error":null}`
}

func apiErrorJSON(message string) string {
	return `{"success":false,"format_version":"1.0","data":null,"error":{"code":"INVALID_REQUEST","message":"` + message + `","details":{}}}`
}

func TestRunHelpAndVersion(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"xftp", "-h"}, "usage: xftp [credential-id|username]"},
		{[]string{"xftp", "--help"}, "Connect to the current ctx target using a stored FTP credential when available."},
		{[]string{"xftp", "-V"}, "xftp 1.1.0\n"},
		{[]string{"xftp", "--version"}, "xftp 1.1.0\n"},
	} {
		var out, stderr bytes.Buffer
		err := New(newFakeRunner(), strings.NewReader(""), &out, &stderr).Run(tt.args)
		if err != nil {
			t.Fatalf("Run(%v) error = %v", tt.args, err)
		}
		if !strings.Contains(out.String(), tt.want) {
			t.Fatalf("Run(%v) output = %q, want %q", tt.args, out.String(), tt.want)
		}
	}
}

func TestRunRequiresCommands(t *testing.T) {
	for _, command := range []string{"ctx", "lftp"} {
		runner := newFakeRunner()
		runner.paths[command] = false
		var out, stderr bytes.Buffer
		err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp"})
		if exitCode(err) != 1 {
			t.Fatalf("missing %s error = %v, want exit 1", command, err)
		}
		want := "xftp: " + command + " is required"
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestPasswordCredentialUsesLFTPEnvPassword(t *testing.T) {
	runner := newFakeRunner()
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if runner.runs[0].name != "lftp" {
		t.Fatalf("run name = %q, want lftp", runner.runs[0].name)
	}
	if !reflect.DeepEqual(runner.runs[0].env, []string{"LFTP_PASSWORD=toor"}) {
		t.Fatalf("env = %#v, want LFTP_PASSWORD only", runner.runs[0].env)
	}
}

func TestPromptErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		want   string
	}{
		{"inactive", apiJSON(`{"active":false,"target_ip":"2.3.4.5"}`), "xftp: no active workspace"},
		{"no target", apiJSON(`{"active":true,"target_ip":null}`), "xftp: no primary target"},
		{"success false", apiErrorJSON("no active workspace"), "xftp: no active workspace"},
		{"invalid json", `{`, "xftp: invalid JSON from ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx prompt --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp"})
			if exitCode(err) != 1 {
				t.Fatalf("error = %v, want exit 1", err)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestCredentialSelectionAndLFTPPassword(t *testing.T) {
	runner := newFakeRunner()
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}

	if len(runner.runs) != 1 {
		t.Fatalf("runs = %#v, want one", runner.runs)
	}
	run := runner.runs[0]
	if run.name != "lftp" {
		t.Fatalf("run name = %q, want lftp", run.name)
	}
	wantArgs := []string{"--env-password", "-e", lftpAuthenticationCommand, "-u", "root", "-p", "21", "2.3.4.5"}
	if !reflect.DeepEqual(run.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", run.args, wantArgs)
	}
	if !reflect.DeepEqual(run.env, []string{"LFTP_PASSWORD=toor"}) {
		t.Fatalf("env = %#v, want LFTP_PASSWORD only", run.env)
	}
	for _, arg := range run.args {
		if arg == "toor" {
			t.Fatalf("password leaked in args: %#v", run.args)
		}
	}
	if !strings.Contains(out.String(), "Connecting to root@2.3.4.5:21...") {
		t.Fatalf("stdout = %q, want connecting line", out.String())
	}
}

func TestCredentialNoPasswordUsesFTP(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ftp","username":"root","password":null}]}`),
	}

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	run := runner.runs[0]
	if run.name != "lftp" {
		t.Fatalf("run name = %q, want lftp", run.name)
	}
	if !reflect.DeepEqual(run.args, []string{"-u", "root", "-p", "21", "2.3.4.5", "-e", lftpAuthenticationCommand}) {
		t.Fatalf("args = %#v", run.args)
	}
	if len(run.env) != 0 {
		t.Fatalf("env = %#v, want none", run.env)
	}
}

func TestNoCredentialsUsesPlainFTP(t *testing.T) {
	runner := newFakeRunner()
	runner.paths["lftp"] = true
	runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[]}`),
	}

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if len(runner.runs) != 1 {
		t.Fatalf("runs = %#v, want one", runner.runs)
	}
	run := runner.runs[0]
	if run.name != "lftp" {
		t.Fatalf("run name = %q, want lftp", run.name)
	}
	if !reflect.DeepEqual(run.args, []string{"-p", "21", "2.3.4.5", "-e", lftpAnonymousCommand}) {
		t.Fatalf("args = %#v", run.args)
	}
	if len(run.env) != 0 {
		t.Fatalf("env = %#v, want none", run.env)
	}
	if !strings.Contains(out.String(), "Connecting to 2.3.4.5:21...") {
		t.Fatalf("stdout = %q, want plain connecting line", out.String())
	}
}

func TestCredentialFiltersAndSelection(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[
			{"id":1,"scope":"http","username":"root","password":"ignored"},
			{"id":6,"scope":"ftp","username":"fuga","password":null},
			{"id":7,"scope":"ftp","username":"tarou","password":null}
		]}`),
	}
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader("2\n"), &out, &stderr).Run([]string{"xftp"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(out.String(), "1) fuga@2.3.4.5") || !strings.Contains(out.String(), "2) tarou@2.3.4.5") {
		t.Fatalf("stdout = %q, want lftp credential candidates", out.String())
	}
	if strings.Contains(out.String(), "[6]") || strings.Contains(out.String(), "[7]") {
		t.Fatalf("stdout = %q, must not show credential IDs", out.String())
	}
	if strings.Contains(out.String(), "ignored") {
		t.Fatalf("stdout leaked password: %q", out.String())
	}
	if got := selectedUsername(runner.runs[0].args); got != "tarou" {
		t.Fatalf("destination = %q, want tarou", got)
	}
}

func TestRunRecordsFTPLogWithoutPassword(t *testing.T) {
	runner := newFakeRunner()
	logger := &memoryCommandLogger{}
	var out, stderr bytes.Buffer
	app := New(runner, strings.NewReader(""), &out, &stderr)
	app.logger = logger

	if err := app.Run([]string{"xftp"}); err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if logger.startCommand != "xftp" || logger.startExpanded != "lftp -p 21 root@2.3.4.5" {
		t.Fatalf("start log = %+v, want sanitized FTP command", logger)
	}
	if strings.Contains(logger.startExpanded, "toor") {
		t.Fatalf("password leaked in start log: %q", logger.startExpanded)
	}
	if logger.finishID != 42 || logger.finishStatus != "success" || logger.finishCode != 0 {
		t.Fatalf("finish log = %+v, want successful log", logger)
	}
}

func TestCtxCommandLoggerUsesSharedJSONClient(t *testing.T) {
	runner := newFakeRunner()
	startKey := "ctx log start --format json --format-version 1"
	finishKey := "ctx log finish 42 --format json --format-version 1"
	runner.outputs[startKey] = fakeOutput{stdout: apiJSON(`{"id":42}`)}
	runner.outputs[finishKey] = fakeOutput{stdout: apiJSON(`{"id":42}`)}
	logger := ctxCommandLogger{runner: runner}

	id, err := logger.Start("xftp", "lftp -p 21 root@2.3.4.5", "2026-07-13T00:00:00Z")
	if err != nil || id != 42 {
		t.Fatalf("Start() = %d, %v; want ID 42", id, err)
	}
	if len(runner.runInputs[startKey]) == 0 {
		t.Fatal("log start JSON input was not sent")
	}
	if err := logger.Finish(id, "success", 0, "connected\n", "", "2026-07-13T00:05:00Z"); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
}

func TestCredentialIDAndUsername(t *testing.T) {
	for _, tt := range []struct {
		name string
		arg  string
		want string
	}{
		{"id", "7", "tarou"},
		{"username", "fuga", "fuga"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[
					{"id":6,"scope":"ftp","username":"fuga","password":null},
					{"id":7,"scope":"ftp","username":"tarou","password":null}
				]}`),
			}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp", tt.arg})
			if err != nil {
				t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
			}
			if got := selectedUsername(runner.runs[0].args); got != tt.want {
				t.Fatalf("destination = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLastCredentialIsDefaultForEmptySelection(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[
			{"id":6,"scope":"ftp","username":"fuga","password":null},
			{"id":7,"scope":"ftp","username":"tarou","password":null}
		]}`),
	}
	state := &memoryCredentialState{}

	var firstOut, firstErr bytes.Buffer
	first := New(runner, strings.NewReader("2\n"), &firstOut, &firstErr)
	first.state = state
	if err := first.Run([]string{"xftp"}); err != nil {
		t.Fatalf("first Run() error = %v, stderr = %q", err, firstErr.String())
	}
	if state.id != 7 {
		t.Fatalf("saved credential ID = %d, want 7", state.id)
	}

	runner.runs = nil
	var secondOut, secondErr bytes.Buffer
	second := New(runner, strings.NewReader("\r"), &secondOut, &secondErr)
	second.state = state
	if err := second.Run([]string{"xftp"}); err != nil {
		t.Fatalf("second Run() error = %v, stderr = %q", err, secondErr.String())
	}
	if !strings.Contains(secondOut.String(), "2) tarou@2.3.4.5 (default)") {
		t.Fatalf("stdout = %q, want default credential", secondOut.String())
	}
	if got := selectedUsername(runner.runs[0].args); got != "tarou" {
		t.Fatalf("destination = %q, want tarou", got)
	}
}

func TestMissingCredentialUsernameUsesPlainFTP(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ftp","username":"root","password":null}]}`),
	}

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp", "ftpuser"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if len(runner.runs) != 1 {
		t.Fatalf("runs = %#v, want one", runner.runs)
	}
	run := runner.runs[0]
	if run.name != "lftp" {
		t.Fatalf("run name = %q, want lftp", run.name)
	}
	if !reflect.DeepEqual(run.args, []string{"-u", "ftpuser", "-p", "21", "2.3.4.5", "-e", lftpAuthenticationCommand}) {
		t.Fatalf("args = %#v, want username fallback", run.args)
	}
	if len(run.env) != 0 {
		t.Fatalf("env = %#v, want none", run.env)
	}
	if !strings.Contains(out.String(), "Connecting to ftpuser@2.3.4.5:21...") {
		t.Fatalf("stdout = %q, want connecting line", out.String())
	}
}

func TestCredentialErrorsAndInvalidJSON(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		args   []string
		want   string
	}{
		{"missing id", apiJSON(`{"credentials":[{"id":6,"scope":"ftp","username":"root","password":null}]}`), []string{"9"}, "xftp: FTP credential not found: 9"},
		{"success false", apiErrorJSON("credential failed"), nil, "xftp: credential failed"},
		{"invalid json", `{`, nil, "xftp: invalid JSON from ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run(append([]string{"xftp"}, tt.args...))
			if exitCode(err) != 1 {
				t.Fatalf("error = %v, want exit 1", err)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestServicePortResolution(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		input  string
		want   []string
	}{
		{"no ftp uses 21", apiJSON(`{"services":[]}`), "", []string{"-u", "root", "-p", "21", "2.3.4.5", "-e", lftpAuthenticationCommand}},
		{"one ftp", apiJSON(`{"services":[{"id":1,"port":2121,"protocol":"tcp","service_name":"ftp"}]}`), "", []string{"-u", "root", "-p", "2121", "2.3.4.5", "-e", lftpAuthenticationCommand}},
		{"filters udp", apiJSON(`{"services":[
			{"id":1,"port":21,"protocol":"udp","service_name":"ftp"},
			{"id":2,"port":2100,"protocol":"tcp","service_name":"http"},
			{"id":3,"port":2121,"protocol":"tcp","service_name":"FTP"}
		]}`), "", []string{"-u", "root", "-p", "2121", "2.3.4.5", "-e", lftpAuthenticationCommand}},
		{"multiple ftp", apiJSON(`{"services":[
			{"id":1,"port":21,"protocol":"tcp","service_name":"ftp"},
			{"id":2,"port":2121,"protocol":"tcp","service_name":"ftp"}
		]}`), "2\n", []string{"-u", "root", "-p", "2121", "2.3.4.5", "-e", lftpAuthenticationCommand}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ftp","username":"root","password":null}]}`),
			}
			runner.outputs["ctx service ls --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(tt.input), &out, &stderr).Run([]string{"xftp"})
			if err != nil {
				t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
			}
			if !reflect.DeepEqual(runner.runs[0].args, tt.want) {
				t.Fatalf("args = %#v, want %#v", runner.runs[0].args, tt.want)
			}
		})
	}
}

func TestServiceErrorsAndSelectionCancel(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		input  string
		want   string
	}{
		{"success false", apiErrorJSON("service failed"), "", "xftp: service failed"},
		{"invalid json", `{`, "", "xftp: invalid JSON from ctx"},
		{"cancel", apiJSON(`{"services":[
			{"id":1,"port":21,"protocol":"tcp","service_name":"ftp"},
			{"id":2,"port":2121,"protocol":"tcp","service_name":"ftp"}
		]}`), "\n", "xftp: cancelled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ftp --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ftp","username":"root","password":null}]}`),
			}
			runner.outputs["ctx service ls --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(tt.input), &out, &stderr).Run([]string{"xftp"})
			if exitCode(err) != 1 {
				t.Fatalf("error = %v, want exit 1", err)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestUnsupportedFormatVersion(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx prompt --format json --format-version 1"] = fakeOutput{
		stdout: `{"success":true,"format_version":"2.0","data":{"active":true,"target_ip":"2.3.4.5"},"error":null}`,
	}
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xftp"})
	if exitCode(err) != 1 {
		t.Fatalf("error = %v, want exit 1", err)
	}
	if !strings.Contains(stderr.String(), "xftp: unsupported ctx JSON format version") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func selectedUsername(args []string) string {
	for i, arg := range args {
		if arg == "-u" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func exitCode(err error) int {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 0
}

type memoryCredentialState struct {
	id int64
}

func (state *memoryCredentialState) Load() (int64, error) {
	return state.id, nil
}

func (state *memoryCredentialState) Save(id int64) error {
	state.id = id
	return nil
}
