package xscp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type commandLogger interface {
	Start(string, string, string) (int64, error)
	Finish(int64, string, int, string, string, string) error
}
type noopCommandLogger struct{}

func (noopCommandLogger) Start(string, string, string) (int64, error)             { return 0, nil }
func (noopCommandLogger) Finish(int64, string, int, string, string, string) error { return nil }

type ctxCommandLogger struct{ runner commandRunner }
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

func (logger ctxCommandLogger) Start(command, expanded, started string) (int64, error) {
	var response APIResponse[logIDData]
	if err := logger.run([]string{"log", "start", "--format", "json", "--format-version", "1"}, logStartRequest{command, expanded, started}, &response); err != nil {
		return 0, err
	}
	if !response.Success || response.Data == nil {
		return 0, errors.New("ctx log command failed")
	}
	return response.Data.ID, nil
}
func (logger ctxCommandLogger) Finish(id int64, status string, code int, stdout, stderr, ended string) error {
	var response APIResponse[logIDData]
	err := logger.run([]string{"log", "finish", fmt.Sprintf("%d", id), "--format", "json", "--format-version", "1"}, logFinishRequest{status, code, stdout, stderr, ended}, &response)
	if err != nil {
		return err
	}
	if !response.Success {
		return errors.New("ctx log command failed")
	}
	return nil
}
func (logger ctxCommandLogger) run(args []string, request, response any) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	var stdout, stderr bytes.Buffer
	if err := logger.runner.Run("ctx", args, nil, bytes.NewReader(payload), &stdout, &stderr); err != nil {
		return fmt.Errorf("ctx log command failed: %w", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), response); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("invalid JSON from ctx: %s", strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}
