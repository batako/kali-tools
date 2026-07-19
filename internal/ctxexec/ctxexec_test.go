package ctxexec

import (
	"bytes"
	"io"
	"testing"
)

type fakeRunner struct {
	name string
}

func (runner *fakeRunner) LookPath(file string) (string, error) {
	runner.name = file
	return file, nil
}

func (runner *fakeRunner) Output(name string, _ ...string) ([]byte, []byte, error) {
	runner.name = name
	return nil, nil, nil
}

func (runner *fakeRunner) Run(name string, _ []string, _ []string, _ io.Reader, _, _ io.Writer) error {
	runner.name = name
	return nil
}

func TestOperationsUseInjectedAbsolutePath(t *testing.T) {
	original := ExecutablePath
	ExecutablePath = "/opt/ctx/bin/ctx"
	t.Cleanup(func() { ExecutablePath = original })

	runner := &fakeRunner{}
	if _, err := LookPath(runner); err != nil || runner.name != ExecutablePath {
		t.Fatalf("LookPath() name = %q, error = %v", runner.name, err)
	}
	if _, _, err := Output(runner, "prompt"); err != nil || runner.name != ExecutablePath {
		t.Fatalf("Output() name = %q, error = %v", runner.name, err)
	}
	if err := Run(runner, []string{"log"}, nil, bytes.NewReader(nil), io.Discard, io.Discard); err != nil || runner.name != ExecutablePath {
		t.Fatalf("Run() name = %q, error = %v", runner.name, err)
	}
}

func TestRelativeExecutablePathIsRejected(t *testing.T) {
	original := ExecutablePath
	ExecutablePath = "ctx"
	t.Cleanup(func() { ExecutablePath = original })

	runner := &fakeRunner{}
	if _, _, err := Output(runner, "prompt"); err == nil {
		t.Fatal("Output() error = nil, want relative path rejection")
	}
	if runner.name != "" {
		t.Fatalf("runner invoked with %q", runner.name)
	}
}
