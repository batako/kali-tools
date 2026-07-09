package ctx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunXStreamsAndSavesCommandLog(t *testing.T) {
	workspace := initXTestWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunX([]string{"x", "sh", "-c", "printf out; printf err >&2"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunX exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "out" {
		t.Fatalf("stdout = %q, want out", stdout.String())
	}
	if stderr.String() != "err" {
		t.Fatalf("stderr = %q, want err", stderr.String())
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.Status != "success" || log.ExitCode != 0 || log.Stdout != "out" || log.Stderr != "err" {
		t.Fatalf("log = %+v, want successful captured output", log)
	}
	if !strings.Contains(log.Command, "printf out") || log.Command != log.ExpandedCommand {
		t.Fatalf("command = %q expanded = %q", log.Command, log.ExpandedCommand)
	}
}

func TestRunXPreservesNonZeroExitCode(t *testing.T) {
	workspace := initXTestWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunX([]string{"x", "sh", "-c", "printf bad >&2; exit 7"}, &stdout, &stderr)
	if code != 7 {
		t.Fatalf("RunX exit code = %d, want 7", code)
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(logs))
	}
	if logs[0].Status != "failed" || logs[0].ExitCode != 7 || logs[0].Stderr != "bad" {
		t.Fatalf("log = %+v, want failed exit 7 with stderr", logs[0])
	}
}

func TestRunXExpandsPrimaryTargetIP(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunX([]string{"x", "sh", "-c", "printf %s \"$1\"", "sh", "$IP"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunX exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "10.10.10.10" {
		t.Fatalf("stdout = %q, want expanded IP", stdout.String())
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(logs))
	}
	if !strings.Contains(logs[0].Command, "$IP") {
		t.Fatalf("command = %q, want original $IP", logs[0].Command)
	}
	if !strings.Contains(logs[0].ExpandedCommand, "10.10.10.10") {
		t.Fatalf("expanded command = %q, want target IP", logs[0].ExpandedCommand)
	}
}

func TestRunXCommandNotFoundIsLogged(t *testing.T) {
	workspace := initXTestWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunX([]string{"x", "ctx-command-that-does-not-exist"}, &stdout, &stderr)
	if code != 127 {
		t.Fatalf("RunX exit code = %d, want 127", code)
	}
	if !strings.Contains(stderr.String(), "ctx-command-that-does-not-exist") {
		t.Fatalf("stderr = %q, want command name", stderr.String())
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(logs))
	}
	if logs[0].Status != "failed" || logs[0].ExitCode != 127 || !strings.Contains(logs[0].Stderr, "ctx-command-that-does-not-exist") {
		t.Fatalf("log = %+v, want command-not-found failure", logs[0])
	}
}

func TestRunLogListsAndShowsCommandLog(t *testing.T) {
	workspace := initXTestWorkspace(t)
	id, err := SaveCommandLog(workspace, CommandLog{
		Command:         "echo hi",
		ExpandedCommand: "echo hi",
		Status:          "success",
		ExitCode:        0,
		Stdout:          "hi\n",
		Stderr:          "",
		StartedAt:       "2026-07-08T00:00:00Z",
		EndedAt:         "2026-07-08T00:00:01Z",
	})
	if err != nil {
		t.Fatalf("SaveCommandLog() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "log"}, &out); err != nil {
		t.Fatalf("Run(ctx log) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "echo hi") || !strings.Contains(got, "success") {
		t.Fatalf("ctx log output = %q, want summary", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "log", "1"}, &out); err != nil {
		t.Fatalf("Run(ctx log 1) error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"id: 1",
		"command: echo hi",
		"expanded_command: echo hi",
		"stdout:\nhi\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ctx log %d output = %q, want %q", id, got, want)
		}
	}
}

func TestListCommandLogsOrdersByTimelineAscending(t *testing.T) {
	workspace := initXTestWorkspace(t)

	for _, log := range []CommandLog{
		{
			Command:         "second",
			ExpandedCommand: "second",
			Status:          "success",
			ExitCode:        0,
			StartedAt:       "2026-07-08T00:00:02Z",
			EndedAt:         "2026-07-08T00:00:03Z",
		},
		{
			Command:         "first",
			ExpandedCommand: "first",
			Status:          "success",
			ExitCode:        0,
			StartedAt:       "2026-07-08T00:00:00Z",
			EndedAt:         "2026-07-08T00:00:01Z",
		},
		{
			Command:         "third",
			ExpandedCommand: "third",
			Status:          "success",
			ExitCode:        0,
			StartedAt:       "2026-07-08T00:00:02Z",
			EndedAt:         "2026-07-08T00:00:04Z",
		},
	} {
		if _, err := SaveCommandLog(workspace, log); err != nil {
			t.Fatalf("SaveCommandLog(%s) error = %v", log.Command, err)
		}
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	got := []string{logs[0].Command, logs[1].Command, logs[2].Command}
	want := []string{"first", "second", "third"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("log order = %v, want %v", got, want)
		}
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "log"}, &out); err != nil {
		t.Fatalf("Run(ctx log) error = %v", err)
	}
	text := out.String()
	first := strings.Index(text, "first")
	second := strings.Index(text, "second")
	third := strings.Index(text, "third")
	if first == -1 || second == -1 || third == -1 || !(first < second && second < third) {
		t.Fatalf("ctx log output order = %q, want first second third", text)
	}
}

func TestRunWithIOCtxXUsesRunX(t *testing.T) {
	workspace := initXTestWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunWithIO([]string{"ctx", "x", "sh", "-c", "printf ctxx"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWithIO(ctx x) error = %v; stderr = %q", err, stderr.String())
	}
	if stdout.String() != "ctxx" {
		t.Fatalf("stdout = %q, want ctxx", stdout.String())
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 || logs[0].Command != `sh -c "printf ctxx"` {
		t.Fatalf("logs = %+v, want ctx x command log", logs)
	}
}

func TestRunWithIOCtxXReturnsExitCodeError(t *testing.T) {
	initXTestWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunWithIO([]string{"ctx", "x", "sh", "-c", "exit 9"}, &stdout, &stderr)
	var exitErr ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("RunWithIO(ctx x) error = %v, want ExitCodeError", err)
	}
	if exitErr.Code != 9 {
		t.Fatalf("exit code = %d, want 9", exitErr.Code)
	}
}

func TestRunXWithoutCommandPrintsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunX([]string{"x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("RunX exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "usage: ctx x <command>") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func initXTestWorkspace(t *testing.T) *Workspace {
	t.Helper()

	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})
	return workspace
}
