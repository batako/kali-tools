package xsmb

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	paths       map[string]bool
	outputs     map[string]fakeOutput
	shareOutput string
	shareArgs   []string
	runs        []fakeRun
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

type memoryCredentialState struct{ id int64 }

func (state *memoryCredentialState) Load() (int64, error) { return state.id, nil }
func (state *memoryCredentialState) Save(id int64) error {
	state.id = id
	return nil
}

type memoryCommandLogger struct {
	startCommand  string
	startExpanded string
	finishID      int64
	finishStatus  string
	finishCode    int
}

func (logger *memoryCommandLogger) Start(command, expandedCommand, startedAt string) (int64, error) {
	logger.startCommand = command
	logger.startExpanded = expandedCommand
	return 42, nil
}

func (logger *memoryCommandLogger) Finish(id int64, status string, exitCode int, stdout, stderr, endedAt string) error {
	logger.finishID = id
	logger.finishStatus = status
	logger.finishCode = exitCode
	return nil
}

func (runner *fakeRunner) LookPath(file string) (string, error) {
	if runner.paths[file] {
		return "/fake/" + file, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeRunner) Output(name string, args ...string) ([]byte, []byte, error) {
	key := name + " " + strings.Join(args, " ")
	output, ok := runner.outputs[key]
	if !ok {
		return nil, nil, fmt.Errorf("unexpected command: %s", key)
	}
	return []byte(output.stdout), []byte(output.stderr), output.err
}

func (runner *fakeRunner) Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if name == "smbclient" && containsArg(args, "-g") {
		runner.shareArgs = append([]string(nil), args...)
		_, _ = io.WriteString(stdout, runner.shareOutput)
		return nil
	}
	runner.runs = append(runner.runs, fakeRun{
		name: name,
		args: append([]string(nil), args...),
		env:  append([]string(nil), env...),
	})
	return nil
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		paths: map[string]bool{
			"ctx":       true,
			"smbclient": true,
		},
		outputs: map[string]fakeOutput{
			"ctx prompt --format json --format-version 1": {
				stdout: apiJSON(`{"active":true,"target_ip":"2.3.4.5","target_name":"default"}`),
			},
			"ctx credential ls smb --format json --format-version 1": {
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"smb","username":"root","password":"toor"}]}`),
			},
			"ctx service ls --format json --format-version 1": {
				stdout: apiJSON(`{"services":[]}`),
			},
		},
		shareOutput: "Disk|public|\n",
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
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
		{[]string{"xsmb", "-h"}, "usage: xsmb [credential-id|username]"},
		{[]string{"xsmb", "--help"}, "Connect to the current ctx target using a stored SMB credential when available."},
		{[]string{"xsmb", "-V"}, "xsmb 1.0.0\n"},
		{[]string{"xsmb", "--version"}, "xsmb 1.0.0\n"},
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
	for _, command := range []string{"ctx", "smbclient"} {
		runner := newFakeRunner()
		runner.paths[command] = false
		var out, stderr bytes.Buffer
		err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"})
		if exitCode(err) != 1 {
			t.Fatalf("missing %s error = %v, want exit 1", command, err)
		}
		want := "xsmb: " + command + " is required"
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestPasswordCredentialUsesSMBClientPassword(t *testing.T) {
	runner := newFakeRunner()
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if runner.runs[0].name != "smbclient" {
		t.Fatalf("run name = %q, want smbclient", runner.runs[0].name)
	}
	if !reflect.DeepEqual(runner.runs[0].env, []string{"PASSWD=toor"}) {
		t.Fatalf("env = %#v, want PASSWD only", runner.runs[0].env)
	}
}

func TestPromptErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		want   string
	}{
		{"inactive", apiJSON(`{"active":false,"target_ip":"2.3.4.5"}`), "xsmb: no active workspace"},
		{"no target", apiJSON(`{"active":true,"target_ip":null}`), "xsmb: no primary target"},
		{"success false", apiErrorJSON("no active workspace"), "xsmb: no active workspace"},
		{"invalid json", `{`, "xsmb: invalid JSON from ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx prompt --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"})
			if exitCode(err) != 1 {
				t.Fatalf("error = %v, want exit 1", err)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestCredentialSelectionAndSMBClientPassword(t *testing.T) {
	runner := newFakeRunner()
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}

	if len(runner.runs) != 1 {
		t.Fatalf("runs = %#v, want one", runner.runs)
	}
	run := runner.runs[0]
	if run.name != "smbclient" {
		t.Fatalf("run name = %q, want smbclient", run.name)
	}
	wantArgs := []string{"//2.3.4.5/public", "-p", "445", "-U", "root"}
	if !reflect.DeepEqual(run.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", run.args, wantArgs)
	}
	if !reflect.DeepEqual(run.env, []string{"PASSWD=toor"}) {
		t.Fatalf("env = %#v, want PASSWD only", run.env)
	}
	for _, arg := range run.args {
		if arg == "toor" {
			t.Fatalf("password leaked in args: %#v", run.args)
		}
	}
	if !strings.Contains(out.String(), "Connecting to root@//2.3.4.5/public:445...") {
		t.Fatalf("stdout = %q, want connecting line", out.String())
	}
}

func TestCredentialNoPasswordUsesSMB(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[{"id":6,"scope":"smb","username":"root","password":null}]}`),
	}

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	run := runner.runs[0]
	if run.name != "smbclient" {
		t.Fatalf("run name = %q, want smbclient", run.name)
	}
	if !reflect.DeepEqual(run.args, []string{"//2.3.4.5/public", "-p", "445", "-U", "root"}) {
		t.Fatalf("args = %#v", run.args)
	}
	if len(run.env) != 0 {
		t.Fatalf("env = %#v, want none", run.env)
	}
}

func TestNoCredentialsUsesPlainSMB(t *testing.T) {
	runner := newFakeRunner()
	runner.paths["smbclient"] = true
	runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[]}`),
	}

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if len(runner.runs) != 1 {
		t.Fatalf("runs = %#v, want one", runner.runs)
	}
	run := runner.runs[0]
	if run.name != "smbclient" {
		t.Fatalf("run name = %q, want smbclient", run.name)
	}
	if !reflect.DeepEqual(run.args, []string{"//2.3.4.5/public", "-p", "445", "-N"}) {
		t.Fatalf("args = %#v", run.args)
	}
	if len(run.env) != 0 {
		t.Fatalf("env = %#v, want none", run.env)
	}
	if !strings.Contains(out.String(), "Connecting to //2.3.4.5/public:445...") {
		t.Fatalf("stdout = %q, want plain connecting line", out.String())
	}
}

func TestCredentialFiltersAndSelection(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[
			{"id":1,"scope":"http","username":"root","password":"ignored"},
			{"id":6,"scope":"smb","username":"fuga","password":null},
			{"id":7,"scope":"smb","username":"tarou","password":null}
		]}`),
	}
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader("2\n"), &out, &stderr).Run([]string{"xsmb"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(out.String(), "1) fuga@2.3.4.5") || !strings.Contains(out.String(), "2) tarou@2.3.4.5") {
		t.Fatalf("stdout = %q, want smb credential candidates", out.String())
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

func TestRunRecordsSMBLogWithoutPassword(t *testing.T) {
	runner := newFakeRunner()
	logger := &memoryCommandLogger{}
	var out, stderr bytes.Buffer
	app := New(runner, strings.NewReader(""), &out, &stderr)
	app.logger = logger

	if err := app.Run([]string{"xsmb"}); err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if logger.startCommand != "xsmb" || logger.startExpanded != "smbclient //2.3.4.5/public -p 445 -U root" {
		t.Fatalf("start log = %+v, want sanitized SMB command", logger)
	}
	if strings.Contains(logger.startExpanded, "toor") {
		t.Fatalf("password leaked in start log: %q", logger.startExpanded)
	}
	if logger.finishID != 42 || logger.finishStatus != "success" || logger.finishCode != 0 {
		t.Fatalf("finish log = %+v, want successful log", logger)
	}
}

func TestLastCredentialIsDefaultForEmptySelection(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[
			{"id":6,"scope":"smb","username":"fuga","password":null},
			{"id":7,"scope":"smb","username":"tarou","password":null}
		]}`),
	}
	state := &memoryCredentialState{}
	var firstOut, firstErr bytes.Buffer
	first := New(runner, strings.NewReader("2\n"), &firstOut, &firstErr)
	first.state = state
	if err := first.Run([]string{"xsmb"}); err != nil {
		t.Fatalf("first Run() error = %v, stderr = %q", err, firstErr.String())
	}
	if state.id != 7 {
		t.Fatalf("saved credential ID = %d, want 7", state.id)
	}

	runner.runs = nil
	var secondOut, secondErr bytes.Buffer
	second := New(runner, strings.NewReader("\r"), &secondOut, &secondErr)
	second.state = state
	if err := second.Run([]string{"xsmb"}); err != nil {
		t.Fatalf("second Run() error = %v, stderr = %q", err, secondErr.String())
	}
	if !strings.Contains(secondOut.String(), "2) tarou@2.3.4.5 (default)") {
		t.Fatalf("stdout = %q, want default credential", secondOut.String())
	}
	if got := selectedUsername(runner.runs[0].args); got != "tarou" {
		t.Fatalf("destination = %q, want tarou", got)
	}
}

func TestMissingCredentialUsernameUsesPlainSMB(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[{"id":6,"scope":"smb","username":"root","password":null}]}`),
	}
	var out, stderr bytes.Buffer
	if err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb", "guest"}); err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if !reflect.DeepEqual(runner.runs[0].args, []string{"//2.3.4.5/public", "-p", "445", "-U", "guest"}) {
		t.Fatalf("args = %#v, want username fallback", runner.runs[0].args)
	}
}

func TestShareDiscoveryUsesAnonymousMode(t *testing.T) {
	runner := newFakeRunner()
	var out, stderr bytes.Buffer
	if err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"}); err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	want := []string{"-L", "//2.3.4.5", "-p", "445", "-g", "-N"}
	if !reflect.DeepEqual(runner.shareArgs, want) {
		t.Fatalf("share discovery args = %#v, want %#v", runner.shareArgs, want)
	}
}

func TestMissingCredentialUsernameCanSelectShare(t *testing.T) {
	runner := newFakeRunner()
	runner.shareOutput = "Disk|public|\nDisk|private|\n"
	runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[{"id":6,"scope":"smb","username":"root","password":null}]}`),
	}
	var out, stderr bytes.Buffer
	if err := New(runner, strings.NewReader("2\n"), &out, &stderr).Run([]string{"xsmb", "smbuser"}); err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(out.String(), "1) public") || !strings.Contains(out.String(), "2) private") {
		t.Fatalf("stdout = %q, want share choices", out.String())
	}
	if !reflect.DeepEqual(runner.runs[0].args, []string{"//2.3.4.5/private", "-p", "445", "-U", "smbuser"}) {
		t.Fatalf("args = %#v, want selected share and username", runner.runs[0].args)
	}
}

func TestShareSelection(t *testing.T) {
	runner := newFakeRunner()
	runner.shareOutput = "Disk|public|\nDisk|private|\nIPC|IPC$|IPC Service\n"
	var out, stderr bytes.Buffer
	if err := New(runner, strings.NewReader("2\n"), &out, &stderr).Run([]string{"xsmb"}); err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(out.String(), "1) public") || !strings.Contains(out.String(), "2) private") {
		t.Fatalf("stdout = %q, want share choices", out.String())
	}
	if strings.Contains(out.String(), "IPC$") {
		t.Fatalf("stdout = %q, must not show IPC$", out.String())
	}
	if !reflect.DeepEqual(runner.runs[0].args, []string{"//2.3.4.5/private", "-p", "445", "-U", "root"}) {
		t.Fatalf("args = %#v, want selected share", runner.runs[0].args)
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
			runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[
					{"id":6,"scope":"smb","username":"fuga","password":null},
					{"id":7,"scope":"smb","username":"tarou","password":null}
				]}`),
			}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb", tt.arg})
			if err != nil {
				t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
			}
			if got := selectedUsername(runner.runs[0].args); got != tt.want {
				t.Fatalf("destination = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCredentialErrorsAndInvalidJSON(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		args   []string
		want   string
	}{
		{"missing id", apiJSON(`{"credentials":[{"id":6,"scope":"smb","username":"root","password":null}]}`), []string{"9"}, "xsmb: SMB credential not found: 9"},
		{"success false", apiErrorJSON("credential failed"), nil, "xsmb: credential failed"},
		{"invalid json", `{`, nil, "xsmb: invalid JSON from ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run(append([]string{"xsmb"}, tt.args...))
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
		{"no smb uses 445", apiJSON(`{"services":[]}`), "", []string{"//2.3.4.5/public", "-p", "445", "-U", "root"}},
		{"one smb", apiJSON(`{"services":[{"id":1,"port":139,"protocol":"tcp","service_name":"netbios-ssn"}]}`), "", []string{"//2.3.4.5/public", "-p", "139", "-U", "root"}},
		{"filters udp", apiJSON(`{"services":[
			{"id":1,"port":445,"protocol":"udp","service_name":"smb"},
			{"id":2,"port":80,"protocol":"tcp","service_name":"http"},
			{"id":3,"port":445,"protocol":"tcp","service_name":"microsoft-ds"}
		]}`), "", []string{"//2.3.4.5/public", "-p", "445", "-U", "root"}},
		{"multiple smb", apiJSON(`{"services":[
			{"id":1,"port":445,"protocol":"tcp","service_name":"smb"},
			{"id":2,"port":139,"protocol":"tcp","service_name":"netbios-ssn"}
		]}`), "2\n", []string{"//2.3.4.5/public", "-p", "139", "-U", "root"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"smb","username":"root","password":null}]}`),
			}
			runner.outputs["ctx service ls --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(tt.input), &out, &stderr).Run([]string{"xsmb"})
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
		{"success false", apiErrorJSON("service failed"), "", "xsmb: service failed"},
		{"invalid json", `{`, "", "xsmb: invalid JSON from ctx"},
		{"cancel", apiJSON(`{"services":[
			{"id":1,"port":445,"protocol":"tcp","service_name":"smb"},
			{"id":2,"port":139,"protocol":"tcp","service_name":"netbios-ssn"}
		]}`), "\n", "xsmb: cancelled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls smb --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"smb","username":"root","password":null}]}`),
			}
			runner.outputs["ctx service ls --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(tt.input), &out, &stderr).Run([]string{"xsmb"})
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
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xsmb"})
	if exitCode(err) != 1 {
		t.Fatalf("error = %v, want exit 1", err)
	}
	if !strings.Contains(stderr.String(), "xsmb: unsupported ctx JSON format version") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func selectedUsername(args []string) string {
	for i, arg := range args {
		if arg == "-U" && i+1 < len(args) {
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
