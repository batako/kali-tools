package ctx

import (
	"testing"
	"time"
)

func TestWebDiscoveryCanBeSavedAndListed(t *testing.T) {
	workspace := initXTestWorkspace(t)
	target, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	logID, err := SaveCommandLog(workspace, CommandLog{
		Command:         "xgobuster",
		ExpandedCommand: "gobuster dir -u http://10.10.10.10 -w /tmp/web.txt",
		Status:          "success",
		StartedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		EndedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("SaveCommandLog() error = %v", err)
	}

	discoveryID, err := SaveWebDiscovery(workspace, target, WebDiscovery{
		URL:                "http://10.10.10.10/admin",
		Path:               "/admin",
		StatusCode:         302,
		ContentLength:      128,
		ContentLengthValid: true,
		RedirectURL:        "http://10.10.10.10/login",
		RedirectURLValid:   true,
		SourceTool:         "gobuster",
		Wordlist:           "/tmp/web.txt",
		CommandLogID:       logID,
		CommandLogIDValid:  true,
		DiscoveredAt:       time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("SaveWebDiscovery() error = %v", err)
	}
	if discoveryID < 1 {
		t.Fatalf("discovery ID = %d, want positive ID", discoveryID)
	}

	if _, err := SaveWebDiscovery(workspace, target, WebDiscovery{
		URL:          "http://10.10.10.10/admin",
		Path:         "/admin",
		StatusCode:   200,
		SourceTool:   "gobuster",
		Wordlist:     "/tmp/web.txt",
		DiscoveredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveWebDiscovery() second error = %v", err)
	}

	discoveries, err := ListWebDiscoveries(workspace, target)
	if err != nil {
		t.Fatalf("ListWebDiscoveries() error = %v", err)
	}
	if len(discoveries) != 2 {
		t.Fatalf("discoveries = %#v, want two observations", discoveries)
	}
	if !discoveries[0].ContentLengthValid || discoveries[0].ContentLength != 128 {
		t.Fatalf("first discovery content length = %+v", discoveries[0])
	}
	if !discoveries[0].RedirectURLValid || discoveries[0].CommandLogID != logID {
		t.Fatalf("first discovery references = %+v", discoveries[0])
	}
	if discoveries[1].ContentLengthValid || discoveries[1].RedirectURLValid || discoveries[1].CommandLogIDValid {
		t.Fatalf("second discovery nullable fields = %+v", discoveries[1])
	}
}
