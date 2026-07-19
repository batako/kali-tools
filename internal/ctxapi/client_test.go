package ctxapi

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"req/internal/ctxexec"
)

type fakeRunner struct {
	name   string
	args   []string
	stdin  string
	stdout string
	stderr string
	err    error
}

func (runner *fakeRunner) Run(name string, args []string, _ []string, stdin io.Reader, stdout, stderr io.Writer) error {
	runner.name = name
	runner.args = append([]string(nil), args...)
	if stdin != nil {
		input, _ := io.ReadAll(stdin)
		runner.stdin = string(input)
	}
	_, _ = io.WriteString(stdout, runner.stdout)
	_, _ = io.WriteString(stderr, runner.stderr)
	return runner.err
}

type testData struct {
	Value string `json:"value"`
}

type exitError struct{ code int }

func (err exitError) Error() string { return "command failed" }
func (err exitError) ExitCode() int { return err.code }

func TestCallUsesFixedExecutableAndVersionedJSONArguments(t *testing.T) {
	runner := &fakeRunner{stdout: `{"success":true,"format_version":"1.0","data":{"value":"ok"},"error":null}`}
	result, err := Call[testData](NewV1(runner), "prompt")
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if runner.name != ctxexec.ExecutablePath {
		t.Fatalf("executable = %q", runner.name)
	}
	wantArgs := []string{"prompt", "--format", "json", "--format-version", "1"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantArgs)
	}
	if result.Data.Value != "ok" || result.FormatVersion != "1.0" {
		t.Fatalf("result = %+v", result)
	}
}

func TestCallWithJSONWritesStandardInput(t *testing.T) {
	runner := &fakeRunner{stdout: `{"success":true,"format_version":"1.0","data":{"value":"ok"},"error":null}`}
	_, err := CallWithJSON[testData](NewV1(runner), map[string]string{"command": "scan"}, "log", "start")
	if err != nil {
		t.Fatalf("CallWithJSON() error = %v", err)
	}
	if runner.stdin != `{"command":"scan"}` {
		t.Fatalf("stdin = %q", runner.stdin)
	}
}

func TestCallReturnsStructuredAPIErrorOnNonzeroExit(t *testing.T) {
	runErr := exitError{code: 1}
	runner := &fakeRunner{
		stdout: `{"success":false,"format_version":"1.0","data":null,"error":{"code":"NOT_FOUND.WORKSPACE","message":"no active workspace","details":{}}}`,
		stderr: "diagnostic",
		err:    runErr,
	}
	_, err := Call[testData](NewV1(runner), "prompt")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Kind != ErrorAPI {
		t.Fatalf("error = %#v", err)
	}
	if apiErr.API.Code != "NOT_FOUND.WORKSPACE" || apiErr.ExitCode == nil || *apiErr.ExitCode != 1 || apiErr.Stderr != "diagnostic" || !errors.Is(apiErr, runErr) {
		t.Fatalf("API error = %+v", apiErr)
	}
}

func TestCallRejectsInvalidJSONWithStderr(t *testing.T) {
	runner := &fakeRunner{stdout: `{`, stderr: "usage failed", err: errors.New("exit code 2")}
	_, err := Call[testData](NewV1(runner), "prompt")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Kind != ErrorJSON || apiErr.Stderr != "usage failed" {
		t.Fatalf("error = %#v", err)
	}
	if !strings.Contains(err.Error(), "invalid JSON from ctx: usage failed") {
		t.Fatalf("error message = %q", err.Error())
	}
}

func TestCallRejectsProcessFailureWithSuccessEnvelope(t *testing.T) {
	runner := &fakeRunner{
		stdout: `{"success":true,"format_version":"1.0","data":{"value":"ok"},"error":null}`,
		err:    errors.New("exit code 1"),
	}
	_, err := Call[testData](NewV1(runner), "prompt")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Kind != ErrorProcess {
		t.Fatalf("error = %#v", err)
	}
}

func TestCallValidatesFormatVersionAndData(t *testing.T) {
	tests := []struct {
		name   string
		output string
		kind   ErrorKind
	}{
		{"missing version", `{"success":true,"format_version":null,"data":{},"error":null}`, ErrorFormat},
		{"wrong major", `{"success":true,"format_version":"2.0","data":{},"error":null}`, ErrorFormat},
		{"missing data", `{"success":true,"format_version":"1.0","data":null,"error":null}`, ErrorData},
		{"invalid data", `{"success":true,"format_version":"1.0","data":{"value":1},"error":null}`, ErrorData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Call[testData](NewV1(&fakeRunner{stdout: tt.output}), "prompt")
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Kind != tt.kind {
				t.Fatalf("error = %#v, want kind %q", err, tt.kind)
			}
		})
	}
}

func TestNewValidatesRequestedVersion(t *testing.T) {
	if _, err := New(&fakeRunner{}, "1.0"); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, version := range []string{"", "v1", "1.x", "1.0.0", "01"} {
		if _, err := New(&fakeRunner{}, version); err == nil {
			t.Fatalf("New(%q) error = nil", version)
		}
	}
}
