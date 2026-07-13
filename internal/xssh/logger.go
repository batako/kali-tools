package xssh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type commandLogger interface {
	Start(command, expandedCommand, startedAt string) (int64, error)
	Finish(id int64, status string, exitCode int, stdout, stderr, endedAt string) error
}

type noopCommandLogger struct{}

func (noopCommandLogger) Start(string, string, string) (int64, error) {
	return 0, nil
}

func (noopCommandLogger) Finish(int64, string, int, string, string, string) error {
	return nil
}

type ctxCommandLogger struct {
	runner commandRunner
}

type logIDData struct {
	ID int64 `json:"id"`
}

type logStartRequest struct {
	Command         string `json:"command"`
	ExpandedCommand string `json:"expanded_command"`
	StartedAt       string `json:"started_at"`
}

type logFinishRequest struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	EndedAt  string `json:"ended_at"`
}

func (logger ctxCommandLogger) Start(command, expandedCommand, startedAt string) (int64, error) {
	request := logStartRequest{
		Command:         command,
		ExpandedCommand: expandedCommand,
		StartedAt:       startedAt,
	}
	var response APIResponse[logIDData]
	if err := logger.run([]string{"log", "start", "--format", "json", "--format-version", "1"}, request, &response); err != nil {
		return 0, err
	}
	if !response.Success || response.Data == nil {
		return 0, ctxLogResponseError(response.Error)
	}
	if response.Data.ID < 1 {
		return 0, errors.New("ctx returned an invalid log ID")
	}
	return response.Data.ID, nil
}

func (logger ctxCommandLogger) Finish(id int64, status string, exitCode int, stdout, stderr, endedAt string) error {
	request := logFinishRequest{
		Status:   status,
		ExitCode: exitCode,
		Stdout:   stdout,
		Stderr:   stderr,
		EndedAt:  endedAt,
	}
	var response APIResponse[logIDData]
	if err := logger.run([]string{"log", "finish", fmt.Sprintf("%d", id), "--format", "json", "--format-version", "1"}, request, &response); err != nil {
		return err
	}
	if !response.Success {
		return ctxLogResponseError(response.Error)
	}
	return nil
}

func (logger ctxCommandLogger) run(args []string, request any, response any) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode ctx log request: %w", err)
	}
	var stdout, stderr bytes.Buffer
	runErr := logger.runner.Run("ctx", args, nil, bytes.NewReader(payload), &stdout, &stderr)
	if err := json.Unmarshal(stdout.Bytes(), response); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("invalid JSON from ctx: %s", strings.TrimSpace(stderr.String()))
		}
		return errors.New("invalid JSON from ctx")
	}
	if runErr != nil {
		return fmt.Errorf("ctx log command failed: %w", runErr)
	}
	return nil
}

func ctxLogResponseError(apiError *APIError) error {
	if apiError != nil && strings.TrimSpace(apiError.Message) != "" {
		return errors.New(apiError.Message)
	}
	return errors.New("ctx log command failed")
}

func commandExitCode(err error) int {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
