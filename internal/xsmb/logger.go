package xsmb

import (
	"errors"
	"fmt"

	"req/internal/ctxapi"
)

type commandLogger interface {
	Start(command, expandedCommand, startedAt string) (int64, error)
	Finish(id int64, status string, exitCode int, stdout, stderr, endedAt string) error
}

type noopCommandLogger struct{}

func (noopCommandLogger) Start(string, string, string) (int64, error) { return 0, nil }
func (noopCommandLogger) Finish(int64, string, int, string, string, string) error {
	return nil
}

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

func (logger ctxCommandLogger) Start(command, expandedCommand, startedAt string) (int64, error) {
	request := logStartRequest{Command: command, ExpandedCommand: expandedCommand, StartedAt: startedAt}
	response, err := ctxapi.CallWithJSON[logIDData](ctxapi.NewV1(logger.runner), request, "log", "start")
	if err != nil {
		return 0, err
	}
	if response.Data.ID < 1 {
		return 0, errors.New("ctx returned an invalid log ID")
	}
	return response.Data.ID, nil
}

func (logger ctxCommandLogger) Finish(id int64, status string, exitCode int, stdout, stderr, endedAt string) error {
	request := logFinishRequest{Status: status, ExitCode: exitCode, Stdout: stdout, Stderr: stderr, EndedAt: endedAt}
	if _, err := ctxapi.CallWithJSON[logIDData](ctxapi.NewV1(logger.runner), request, "log", "finish", fmt.Sprintf("%d", id)); err != nil {
		return err
	}
	return nil
}

func commandExitCode(err error) int {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
