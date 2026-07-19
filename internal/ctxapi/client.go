package ctxapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"req/internal/ctxexec"
)

type Runner interface {
	Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error
}

type Client struct {
	runner        Runner
	formatVersion string
}

type Result[T any] struct {
	Data          *T
	FormatVersion string
	Stderr        string
}

type APIError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

type ErrorKind string

const (
	ErrorAPI     ErrorKind = "api"
	ErrorProcess ErrorKind = "process"
	ErrorJSON    ErrorKind = "json"
	ErrorFormat  ErrorKind = "format"
	ErrorData    ErrorKind = "data"
	ErrorInput   ErrorKind = "input"
)

type Error struct {
	Kind     ErrorKind
	API      *APIError
	ExitCode *int
	Stderr   string
	Err      error
}

func (err *Error) Error() string {
	if err.API != nil && strings.TrimSpace(err.API.Message) != "" {
		return err.API.Message
	}
	if err.Err != nil {
		return err.Err.Error()
	}
	return "ctx command failed"
}

func (err *Error) Unwrap() error { return err.Err }

type envelope struct {
	Success       bool            `json:"success"`
	FormatVersion *string         `json:"format_version"`
	Data          json.RawMessage `json:"data"`
	Error         *APIError       `json:"error"`
}

func New(runner Runner, formatVersion string) (*Client, error) {
	formatVersion = strings.TrimSpace(formatVersion)
	if !validRequestedVersion(formatVersion) {
		return nil, fmt.Errorf("invalid ctx format version: %s", formatVersion)
	}
	return &Client{runner: runner, formatVersion: formatVersion}, nil
}

func NewV1(runner Runner) *Client {
	client, _ := New(runner, "1")
	return client
}

func Call[T any](client *Client, args ...string) (*Result[T], error) {
	return call[T](client, nil, args...)
}

func CallWithJSON[T any](client *Client, input any, args ...string) (*Result[T], error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, &Error{Kind: ErrorInput, Err: fmt.Errorf("encode ctx JSON input: %w", err)}
	}
	return call[T](client, bytes.NewReader(payload), args...)
}

func call[T any](client *Client, stdin io.Reader, args ...string) (*Result[T], error) {
	if client == nil || client.runner == nil {
		return nil, &Error{Kind: ErrorProcess, Err: errors.New("ctx API client has no runner")}
	}
	commandArgs := append([]string(nil), args...)
	commandArgs = append(commandArgs, "--format", "json", "--format-version", client.formatVersion)

	var stdout, stderr bytes.Buffer
	runErr := ctxexec.Run(client.runner, commandArgs, nil, stdin, &stdout, &stderr)
	stderrText := strings.TrimSpace(stderr.String())

	var response envelope
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		message := "invalid JSON from ctx"
		if stderrText != "" {
			message += ": " + stderrText
		}
		return nil, &Error{Kind: ErrorJSON, Stderr: stderrText, Err: errors.New(message)}
	}
	if !response.Success {
		apiError := response.Error
		if apiError == nil {
			apiError = &APIError{Message: "ctx command failed"}
		}
		return nil, &Error{Kind: ErrorAPI, API: apiError, ExitCode: exitCode(runErr), Stderr: stderrText, Err: runErr}
	}
	if runErr != nil {
		return nil, &Error{Kind: ErrorProcess, ExitCode: exitCode(runErr), Stderr: stderrText, Err: fmt.Errorf("ctx command failed: %w", runErr)}
	}
	if response.FormatVersion == nil || !versionMatches(client.formatVersion, *response.FormatVersion) {
		return nil, &Error{Kind: ErrorFormat, Stderr: stderrText, Err: errors.New("unsupported ctx JSON format version")}
	}
	if len(response.Data) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) {
		return nil, &Error{Kind: ErrorData, Stderr: stderrText, Err: errors.New("ctx response missing data")}
	}
	var data T
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return nil, &Error{Kind: ErrorData, Stderr: stderrText, Err: fmt.Errorf("invalid ctx response data: %w", err)}
	}
	return &Result[T]{Data: &data, FormatVersion: *response.FormatVersion, Stderr: stderrText}, nil
}

func exitCode(err error) *int {
	if err == nil {
		return nil
	}
	type exitCoder interface {
		ExitCode() int
	}
	var coded exitCoder
	if !errors.As(err, &coded) {
		return nil
	}
	code := coded.ExitCode()
	return &code
}

func validRequestedVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 || strconv.Itoa(value) != part {
			return false
		}
	}
	return true
}

func versionMatches(requested, actual string) bool {
	if !validRequestedVersion(actual) || !strings.Contains(actual, ".") {
		return false
	}
	if strings.Contains(requested, ".") {
		return actual == requested
	}
	return strings.HasPrefix(actual, requested+".")
}
