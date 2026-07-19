package ctx

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func runLogAPIRequest(t *testing.T, args []string, input string) APIResponse {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := RunWithInput(args, strings.NewReader(input), &stdout, &stderr); err != nil {
		t.Fatalf("RunWithInput(%v) error = %v, stderr = %q", args, err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	return decodeAPIResponse(t, stdout.Bytes())
}

func logResponseID(t *testing.T, response APIResponse) int64 {
	t.Helper()
	data := responseDataMap(t, response)
	id, ok := data["id"].(float64)
	if !ok || id < 1 {
		t.Fatalf("data.id = %#v, want positive integer", data["id"])
	}
	return int64(id)
}

func TestLogAPIStartAppliesDefaultsAndCreatesRunningLog(t *testing.T) {
	workspace := initXTestWorkspace(t)
	before := time.Now().UTC()
	response := runLogAPIRequest(t,
		[]string{"ctx", "log", "start", "--format", "json", "--format-version", "1.0"},
		`{"command":"custom scan"}`,
	)
	id := logResponseID(t, response)

	log, err := GetCommandLog(workspace, strconv.FormatInt(id, 10))
	if err != nil {
		t.Fatalf("GetCommandLog() error = %v", err)
	}
	startedAt, err := time.Parse(time.RFC3339Nano, log.StartedAt)
	if err != nil {
		t.Fatalf("started_at = %q, want RFC3339: %v", log.StartedAt, err)
	}
	if log.Command != "custom scan" || log.ExpandedCommand != log.Command || log.Status != "running" || log.EndedAt != "" {
		t.Fatalf("log = %+v, want default running log", log)
	}
	if startedAt.Before(before) || startedAt.After(time.Now().UTC()) {
		t.Fatalf("started_at = %s, want current UTC time", startedAt)
	}
}

func TestLogAPIFinishStoresEachTerminalState(t *testing.T) {
	workspace := initXTestWorkspace(t)
	tests := []struct {
		status     string
		exitCode   int
		storedExit int
		stdout     string
		stderr     string
		startedAt  string
		endedAt    string
	}{
		{status: "success", exitCode: 0, storedExit: 0, stdout: "ok\n", startedAt: "2026-07-19T00:00:00Z", endedAt: "2026-07-19T00:00:01Z"},
		{status: "failed", exitCode: 7, storedExit: 7, stderr: "failed\n", startedAt: "2026-07-19T00:01:00Z", endedAt: "2026-07-19T00:01:01Z"},
		{status: "interrupted", exitCode: -1, storedExit: 0, stderr: "stopped\n", startedAt: "2026-07-19T00:02:00Z", endedAt: "2026-07-19T00:02:01Z"},
	}

	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			startBody := fmt.Sprintf(`{"command":"tool %s","expanded_command":"tool --state %s","started_at":%q}`, test.status, test.status, test.startedAt)
			start := runLogAPIRequest(t, []string{"ctx", "log", "start", "--format", "json"}, startBody)
			id := logResponseID(t, start)

			finishBody := fmt.Sprintf(`{"status":%q,"exit_code":%d,"stdout":%q,"stderr":%q,"ended_at":%q}`, test.status, test.exitCode, test.stdout, test.stderr, test.endedAt)
			finish := runLogAPIRequest(t, []string{"ctx", "log", "finish", strconv.FormatInt(id, 10), "--format", "json"}, finishBody)
			if got := logResponseID(t, finish); got != id {
				t.Fatalf("finish id = %d, want %d", got, id)
			}

			log, err := GetCommandLog(workspace, strconv.FormatInt(id, 10))
			if err != nil {
				t.Fatalf("GetCommandLog() error = %v", err)
			}
			if log.Status != test.status || log.ExitCode != test.storedExit || log.Stdout != test.stdout || log.Stderr != test.stderr || log.StartedAt != test.startedAt || log.EndedAt != test.endedAt {
				t.Fatalf("log = %+v, want terminal state %+v", log, test)
			}
		})
	}
}

func TestLogAPIFinishAppliesOptionalFieldDefaults(t *testing.T) {
	workspace := initXTestWorkspace(t)
	start := runLogAPIRequest(t, []string{"ctx", "log", "start", "--format", "json"}, `{"command":"tool"}`)
	id := logResponseID(t, start)
	before := time.Now().UTC()
	runLogAPIRequest(t, []string{"ctx", "log", "finish", strconv.FormatInt(id, 10), "--format", "json"}, `{"status":"failed"}`)

	log, err := GetCommandLog(workspace, strconv.FormatInt(id, 10))
	if err != nil {
		t.Fatalf("GetCommandLog() error = %v", err)
	}
	endedAt, err := time.Parse(time.RFC3339Nano, log.EndedAt)
	if err != nil {
		t.Fatalf("ended_at = %q, want RFC3339: %v", log.EndedAt, err)
	}
	if log.ExitCode != 0 || log.Stdout != "" || log.Stderr != "" || endedAt.Before(before) || endedAt.After(time.Now().UTC()) {
		t.Fatalf("log = %+v, want finish defaults", log)
	}
}
