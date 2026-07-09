package ctx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPromptDataReturnsWorkspaceTargetAndLocalAddress(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	oldLocalAddress := promptLocalAddressFunc
	promptLocalAddressFunc = func(targetIP string) (string, string) {
		if targetIP != "10.10.10.10" {
			t.Fatalf("local address target = %q, want 10.10.10.10", targetIP)
		}
		return "10.8.0.2", "tun0"
	}
	t.Cleanup(func() { promptLocalAddressFunc = oldLocalAddress })

	data, err := LoadPromptData(root)
	if err != nil {
		t.Fatalf("LoadPromptData() error = %v", err)
	}
	if !data.Active ||
		data.WorkspaceID != workspace.ID ||
		data.WorkspaceName != filepath.Base(root) ||
		data.TargetName != "default" ||
		data.TargetIP != "10.10.10.10" ||
		data.LocalIP != "10.8.0.2" ||
		data.LocalInterface != "tun0" {
		t.Fatalf("prompt data = %+v", data)
	}
}

func TestLoadPromptDataOutsideWorkspaceIsInactive(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	data, err := LoadPromptData(t.TempDir())
	if err != nil {
		t.Fatalf("LoadPromptData() error = %v", err)
	}
	if data.Active {
		t.Fatalf("prompt data = %+v, want inactive", data)
	}
}

func TestWritePromptDataShellIsSafeAndComplete(t *testing.T) {
	data := PromptData{
		Active:         true,
		WorkspaceID:    "workspace-id",
		WorkspaceName:  "operator's lab",
		WorkspaceRoot:  "/tmp/operator's lab",
		LocalIP:        "10.8.0.2",
		LocalInterface: "tun0",
		TargetName:     "dc",
		TargetIP:       "10.10.10.10",
	}

	var out bytes.Buffer
	if err := WritePromptData(&out, data, "shell", ""); err != nil {
		t.Fatalf("WritePromptData(shell) error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"CTX_ACTIVE='1'",
		"CTX_WORKSPACE_NAME='operator'\"'\"'s lab'",
		"CTX_LOCAL_IP='10.8.0.2'",
		"CTX_LOCAL_INTERFACE='tun0'",
		"CTX_TARGET_NAME='dc'",
		"CTX_TARGET_IP='10.10.10.10'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("shell output = %q, want %q", got, want)
		}
	}
}

func TestWritePromptDataJSONAndField(t *testing.T) {
	data := PromptData{Active: true, LocalIP: "10.8.0.2", TargetIP: "10.10.10.10"}

	var out bytes.Buffer
	if err := WritePromptData(&out, data, "json", ""); err != nil {
		t.Fatalf("WritePromptData(json) error = %v", err)
	}
	var decoded PromptData
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded != data {
		t.Fatalf("decoded data = %+v, want %+v", decoded, data)
	}

	out.Reset()
	if err := WritePromptData(&out, data, "shell", "target-ip"); err != nil {
		t.Fatalf("WritePromptData(field) error = %v", err)
	}
	if got := out.String(); got != "10.10.10.10\n" {
		t.Fatalf("field output = %q", got)
	}
}

func TestRunPrompt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
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

	oldLocalAddress := promptLocalAddressFunc
	promptLocalAddressFunc = func(string) (string, string) { return "10.8.0.2", "tun0" }
	t.Cleanup(func() { promptLocalAddressFunc = oldLocalAddress })

	var out bytes.Buffer
	if err := Run([]string{"ctx", "prompt", "--field", "local-ip"}, &out); err != nil {
		t.Fatalf("Run(ctx prompt --field local-ip) error = %v", err)
	}
	if got := out.String(); got != "10.8.0.2\n" {
		t.Fatalf("prompt field output = %q", got)
	}
}
