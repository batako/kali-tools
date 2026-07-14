package ctx

import (
	"testing"
	"time"
)

func TestWebWordlistRunHistory(t *testing.T) {
	workspace := initXTestWorkspace(t)
	target, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	runID, err := StartWebWordlistRun(workspace, target, "http://10.10.10.10", "seclists", "web-standard", "", "/tmp/common.txt", startedAt, 0)
	if err != nil {
		t.Fatalf("StartWebWordlistRun() error = %v", err)
	}
	if err := FinishWebWordlistRun(workspace, runID, "success", startedAt); err != nil {
		t.Fatalf("FinishWebWordlistRun() error = %v", err)
	}
	runs, err := ListWebWordlistRuns(workspace, target, "http://10.10.10.10")
	if err != nil {
		t.Fatalf("ListWebWordlistRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "success" || runs[0].Wordlist != "/tmp/common.txt" {
		t.Fatalf("runs = %+v", runs)
	}
	secondID, err := StartWebWordlistRun(workspace, target, "http://10.10.10.10", "seclists", "web-standard", "", "/tmp/common.txt", startedAt, 0)
	if err != nil || secondID != runID {
		t.Fatalf("StartWebWordlistRun() force update = %d, %v; want %d", secondID, err, runID)
	}
	thirdID, err := StartWebWordlistRun(workspace, target, "http://10.10.10.10", "seclists", "web-standard", "-x\x00php", "/tmp/common.txt", startedAt, 0)
	if err != nil || thirdID == runID {
		t.Fatalf("StartWebWordlistRun() distinct search = %d, %v; want a new run", thirdID, err)
	}
}
