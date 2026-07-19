package ctxexec

import (
	"errors"
	"io"
	"path/filepath"
)

// ExecutablePath is fixed at build time so official commands do not resolve ctx through PATH.
var ExecutablePath = "/usr/local/bin/ctx"

type LookPathRunner interface {
	LookPath(file string) (string, error)
}

type OutputRunner interface {
	Output(name string, args ...string) ([]byte, []byte, error)
}

type RunRunner interface {
	Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error
}

func Path() (string, error) {
	if !filepath.IsAbs(ExecutablePath) {
		return "", errors.New("ctx executable path must be absolute")
	}
	return filepath.Clean(ExecutablePath), nil
}

func LookPath(runner LookPathRunner) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	return runner.LookPath(path)
}

func Output(runner OutputRunner, args ...string) ([]byte, []byte, error) {
	path, err := Path()
	if err != nil {
		return nil, nil, err
	}
	return runner.Output(path, args...)
}

func Run(runner RunRunner, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return runner.Run(path, args, env, stdin, stdout, stderr)
}
