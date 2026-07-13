package xssh

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
	paths   map[string]bool
	outputs map[string]fakeOutput
	runs    []fakeRun
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
			"ctx":     true,
			"ssh":     true,
			"sshpass": true,
		},
		outputs: map[string]fakeOutput{
			"ctx prompt --format json --format-version 1": {
				stdout: apiJSON(`{"active":true,"target_ip":"2.3.4.5","target_name":"default"}`),
			},
			"ctx credential ls ssh --format json --format-version 1": {
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ssh","username":"root","password":"toor"}]}`),
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
		{[]string{"xssh", "-h"}, "usage: xssh [credential-id|username]"},
		{[]string{"xssh", "--help"}, "Connect to the current ctx target using a stored SSH credential when available."},
		{[]string{"xssh", "-V"}, "xssh 0.1.0\n"},
		{[]string{"xssh", "--version"}, "xssh 0.1.0\n"},
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
	for _, command := range []string{"ctx", "ssh"} {
		runner := newFakeRunner()
		runner.paths[command] = false
		var out, stderr bytes.Buffer
		err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh"})
		if exitCode(err) != 1 {
			t.Fatalf("missing %s error = %v, want exit 1", command, err)
		}
		want := "xssh: " + command + " is required"
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRequiresSSHPassOnlyWithPasswordCredential(t *testing.T) {
	runner := newFakeRunner()
	runner.paths["sshpass"] = false

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh"})
	if exitCode(err) != 1 {
		t.Fatalf("missing sshpass error = %v, want exit 1", err)
	}
	if !strings.Contains(stderr.String(), "xssh: sshpass is required") {
		t.Fatalf("stderr = %q, want sshpass required", stderr.String())
	}
}

func TestPromptErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		want   string
	}{
		{"inactive", apiJSON(`{"active":false,"target_ip":"2.3.4.5"}`), "xssh: no active workspace"},
		{"no target", apiJSON(`{"active":true,"target_ip":null}`), "xssh: no primary target"},
		{"success false", apiErrorJSON("no active workspace"), "xssh: no active workspace"},
		{"invalid json", `{`, "xssh: invalid JSON from ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx prompt --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh"})
			if exitCode(err) != 1 {
				t.Fatalf("error = %v, want exit 1", err)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestCredentialSelectionAndSSHPass(t *testing.T) {
	runner := newFakeRunner()
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}

	if len(runner.runs) != 1 {
		t.Fatalf("runs = %#v, want one", runner.runs)
	}
	run := runner.runs[0]
	if run.name != "sshpass" {
		t.Fatalf("run name = %q, want sshpass", run.name)
	}
	wantArgs := []string{"-e", "ssh", "-p", "22", "root@2.3.4.5"}
	if !reflect.DeepEqual(run.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", run.args, wantArgs)
	}
	if !reflect.DeepEqual(run.env, []string{"SSHPASS=toor"}) {
		t.Fatalf("env = %#v, want SSHPASS only", run.env)
	}
	for _, arg := range run.args {
		if arg == "toor" {
			t.Fatalf("password leaked in args: %#v", run.args)
		}
	}
	if !strings.Contains(out.String(), "Connecting to root@2.3.4.5:22...") {
		t.Fatalf("stdout = %q, want connecting line", out.String())
	}
}

func TestCredentialNoPasswordUsesSSH(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls ssh --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ssh","username":"root","password":null}]}`),
	}

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	run := runner.runs[0]
	if run.name != "ssh" {
		t.Fatalf("run name = %q, want ssh", run.name)
	}
	if !reflect.DeepEqual(run.args, []string{"-p", "22", "root@2.3.4.5"}) {
		t.Fatalf("args = %#v", run.args)
	}
	if len(run.env) != 0 {
		t.Fatalf("env = %#v, want none", run.env)
	}
}

func TestNoCredentialsUsesPlainSSH(t *testing.T) {
	runner := newFakeRunner()
	runner.paths["sshpass"] = false
	runner.outputs["ctx credential ls ssh --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[]}`),
	}

	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if len(runner.runs) != 1 {
		t.Fatalf("runs = %#v, want one", runner.runs)
	}
	run := runner.runs[0]
	if run.name != "ssh" {
		t.Fatalf("run name = %q, want ssh", run.name)
	}
	if !reflect.DeepEqual(run.args, []string{"-p", "22", "2.3.4.5"}) {
		t.Fatalf("args = %#v", run.args)
	}
	if len(run.env) != 0 {
		t.Fatalf("env = %#v, want none", run.env)
	}
	if !strings.Contains(out.String(), "Connecting to 2.3.4.5:22...") {
		t.Fatalf("stdout = %q, want plain connecting line", out.String())
	}
}

func TestCredentialFiltersAndSelection(t *testing.T) {
	runner := newFakeRunner()
	runner.outputs["ctx credential ls ssh --format json --format-version 1"] = fakeOutput{
		stdout: apiJSON(`{"credentials":[
			{"id":1,"scope":"http","username":"root","password":"ignored"},
			{"id":6,"scope":"ssh","username":"fuga","password":null},
			{"id":7,"scope":"ssh","username":"tarou","password":null}
		]}`),
	}
	var out, stderr bytes.Buffer
	err := New(runner, strings.NewReader("2\n"), &out, &stderr).Run([]string{"xssh"})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(out.String(), "[6] fuga@2.3.4.5") || !strings.Contains(out.String(), "[7] tarou@2.3.4.5") {
		t.Fatalf("stdout = %q, want ssh credential candidates", out.String())
	}
	if strings.Contains(out.String(), "ignored") {
		t.Fatalf("stdout leaked password: %q", out.String())
	}
	if got := runner.runs[0].args[len(runner.runs[0].args)-1]; got != "tarou@2.3.4.5" {
		t.Fatalf("destination = %q, want tarou", got)
	}
}

func TestCredentialIDAndUsername(t *testing.T) {
	for _, tt := range []struct {
		name string
		arg  string
		want string
	}{
		{"id", "7", "tarou@2.3.4.5"},
		{"username", "fuga", "fuga@2.3.4.5"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ssh --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[
					{"id":6,"scope":"ssh","username":"fuga","password":null},
					{"id":7,"scope":"ssh","username":"tarou","password":null}
				]}`),
			}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh", tt.arg})
			if err != nil {
				t.Fatalf("Run() error = %v, stderr = %q", err, stderr.String())
			}
			if got := runner.runs[0].args[len(runner.runs[0].args)-1]; got != tt.want {
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
		{"missing id", apiJSON(`{"credentials":[{"id":6,"scope":"ssh","username":"root","password":null}]}`), []string{"9"}, "xssh: SSH credential not found: 9"},
		{"missing username", apiJSON(`{"credentials":[{"id":6,"scope":"ssh","username":"root","password":null}]}`), []string{"guest"}, "xssh: SSH credential not found: guest"},
		{"success false", apiErrorJSON("credential failed"), nil, "xssh: credential failed"},
		{"invalid json", `{`, nil, "xssh: invalid JSON from ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ssh --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(""), &out, &stderr).Run(append([]string{"xssh"}, tt.args...))
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
		{"no ssh uses 22", apiJSON(`{"services":[]}`), "", []string{"-p", "22", "root@2.3.4.5"}},
		{"one ssh", apiJSON(`{"services":[{"id":1,"port":2222,"protocol":"tcp","service_name":"ssh"}]}`), "", []string{"-p", "2222", "root@2.3.4.5"}},
		{"filters udp", apiJSON(`{"services":[
			{"id":1,"port":22,"protocol":"udp","service_name":"ssh"},
			{"id":2,"port":2200,"protocol":"tcp","service_name":"http"},
			{"id":3,"port":2222,"protocol":"tcp","service_name":"SSH"}
		]}`), "", []string{"-p", "2222", "root@2.3.4.5"}},
		{"multiple ssh", apiJSON(`{"services":[
			{"id":1,"port":22,"protocol":"tcp","service_name":"ssh"},
			{"id":2,"port":2222,"protocol":"tcp","service_name":"ssh"}
		]}`), "2\n", []string{"-p", "2222", "root@2.3.4.5"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ssh --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ssh","username":"root","password":null}]}`),
			}
			runner.outputs["ctx service ls --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(tt.input), &out, &stderr).Run([]string{"xssh"})
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
		{"success false", apiErrorJSON("service failed"), "", "xssh: service failed"},
		{"invalid json", `{`, "", "xssh: invalid JSON from ctx"},
		{"cancel", apiJSON(`{"services":[
			{"id":1,"port":22,"protocol":"tcp","service_name":"ssh"},
			{"id":2,"port":2222,"protocol":"tcp","service_name":"ssh"}
		]}`), "\n", "xssh: cancelled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.outputs["ctx credential ls ssh --format json --format-version 1"] = fakeOutput{
				stdout: apiJSON(`{"credentials":[{"id":6,"scope":"ssh","username":"root","password":null}]}`),
			}
			runner.outputs["ctx service ls --format json --format-version 1"] = fakeOutput{stdout: tt.stdout}
			var out, stderr bytes.Buffer
			err := New(runner, strings.NewReader(tt.input), &out, &stderr).Run([]string{"xssh"})
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
	err := New(runner, strings.NewReader(""), &out, &stderr).Run([]string{"xssh"})
	if exitCode(err) != 1 {
		t.Fatalf("error = %v, want exit 1", err)
	}
	if !strings.Contains(stderr.String(), "xssh: unsupported ctx JSON format version") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func exitCode(err error) int {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 0
}
