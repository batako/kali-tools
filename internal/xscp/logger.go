package xscp

import (
	"errors"
	"fmt"

	"req/internal/ctxapi"
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
	response, err := ctxapi.CallWithJSON[logIDData](ctxapi.NewV1(logger.runner), logStartRequest{command, expanded, started}, "log", "start")
	if err != nil {
		return 0, err
	}
	if response.Data.ID < 1 {
		return 0, errors.New("ctx returned an invalid log ID")
	}
	return response.Data.ID, nil
}
func (logger ctxCommandLogger) Finish(id int64, status string, code int, stdout, stderr, ended string) error {
	if _, err := ctxapi.CallWithJSON[logIDData](ctxapi.NewV1(logger.runner), logFinishRequest{status, code, stdout, stderr, ended}, "log", "finish", fmt.Sprintf("%d", id)); err != nil {
		return err
	}
	return nil
}
