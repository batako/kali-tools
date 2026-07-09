package ctx

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRunServiceListsPrimaryTargetScanResults(t *testing.T) {
	workspace := initXTestWorkspace(t)
	target, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	for _, service := range []Service{
		{Port: 22, Protocol: "tcp", State: "open", ServiceName: "ssh", Product: "OpenSSH", Version: "9.7", LastSeen: time.Now().UTC().Format(time.RFC3339Nano)},
		{Port: 443, Protocol: "tcp", State: "open", ServiceName: "https", Product: "nginx", Version: "1.25", LastSeen: time.Now().UTC().Format(time.RFC3339Nano)},
	} {
		if _, err := UpsertService(workspace, target, service); err != nil {
			t.Fatalf("UpsertService() error = %v", err)
		}
	}

	var output bytes.Buffer
	if err := Run([]string{"ctx", "service", "ls"}, &output); err != nil {
		t.Fatalf("Run(ctx service ls) error = %v", err)
	}
	got := output.String()
	for _, want := range []string{
		"Target: default (10.10.10.10)",
		"PORT",
		"22/tcp",
		"OpenSSH",
		"443/tcp",
		"nginx",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestRunServiceListsSelectedTarget(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	target, err := AddTarget(workspace, "10.10.10.20", "target2")
	if err != nil {
		t.Fatalf("AddTarget() error = %v", err)
	}
	if _, err := UpsertService(workspace, target, Service{
		Port: 80, Protocol: "tcp", State: "open", ServiceName: "http",
		LastSeen: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertService() error = %v", err)
	}

	var output bytes.Buffer
	if err := Run([]string{"ctx", "service", "ls", "--target", "target2"}, &output); err != nil {
		t.Fatalf("Run(ctx service ls --target) error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Target: target2 (10.10.10.20)") || !strings.Contains(got, "80/tcp") {
		t.Fatalf("output = %q, want selected target result", got)
	}
}

func TestRunServiceReportsNoScanResults(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	var output bytes.Buffer
	if err := Run([]string{"ctx", "service", "ls"}, &output); err != nil {
		t.Fatalf("Run(ctx service ls) error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "no scan results") {
		t.Fatalf("output = %q, want no scan results", got)
	}
}
