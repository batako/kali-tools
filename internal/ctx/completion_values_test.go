package ctx

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompletionValuesReturnCurrentWorkspaceData(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddTarget(workspace, "1.2.3.4", "target2"); err != nil {
		t.Fatalf("AddTarget() error = %v", err)
	}
	if _, err := AddHost(workspace, "dc01.example.local", "default"); err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if _, err := SaveCommandLog(workspace, CommandLog{
		Command:         "nmap target",
		ExpandedCommand: "nmap target",
		Status:          "success",
		StartedAt:       "2026-07-09T00:00:00Z",
		EndedAt:         "2026-07-09T00:00:01Z",
	}); err != nil {
		t.Fatalf("SaveCommandLog() error = %v", err)
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

	tests := []struct {
		kind string
		want []string
	}{
		{"target", []string{"default", "target2"}},
		{"host", []string{"dc01.example.local"}},
		{"log", []string{"1"}},
	}
	for _, tt := range tests {
		got, err := completionValues(tt.kind)
		if err != nil {
			t.Fatalf("completionValues(%s) error = %v", tt.kind, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("completionValues(%s) = %v, want %v", tt.kind, got, tt.want)
		}
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "completion", "values", "target"}, &out); err != nil {
		t.Fatalf("Run(ctx completion values target) error = %v", err)
	}
	if got := strings.Fields(out.String()); !reflect.DeepEqual(got, []string{"default", "target2"}) {
		t.Fatalf("completion output = %v", got)
	}

	descriptionTests := []struct {
		kind string
		want []string
	}{
		{"target", []string{"default:10.10.10.10 (primary)", "target2:1.2.3.4"}},
		{"host", []string{"dc01.example.local:default 10.10.10.10"}},
		{"log", []string{"1:nmap target"}},
	}
	for _, tt := range descriptionTests {
		got, err := completionDescriptions(tt.kind)
		if err != nil {
			t.Fatalf("completionDescriptions(%s) error = %v", tt.kind, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("completionDescriptions(%s) = %v, want %v", tt.kind, got, tt.want)
		}
	}

	out.Reset()
	if err := Run([]string{"ctx", "completion", "descriptions", "target"}, &out); err != nil {
		t.Fatalf("Run(ctx completion descriptions target) error = %v", err)
	}
	for _, want := range []string{"default:10.10.10.10 (primary)", "target2:1.2.3.4"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("description output = %q, want %q", out.String(), want)
		}
	}
}

func TestCompletionValuesReturnAllWorkspaceIDs(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	first, err := InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("InitWorkspace(first) error = %v", err)
	}
	second, err := InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("InitWorkspace(second) error = %v", err)
	}

	values, err := completionValues("workspace")
	if err != nil {
		t.Fatalf("completionValues(workspace) error = %v", err)
	}
	if len(values) != 2 || !containsString(values, workspaceIDString(first.ID)) || !containsString(values, workspaceIDString(second.ID)) {
		t.Fatalf("workspace completion values = %v", values)
	}

	descriptions, err := completionDescriptions("workspace")
	if err != nil {
		t.Fatalf("completionDescriptions(workspace) error = %v", err)
	}
	if len(descriptions) != 2 ||
		!strings.HasPrefix(descriptions[0], values[0]+":") ||
		!strings.Contains(strings.Join(descriptions, "\n"), first.RootPath) ||
		!strings.Contains(strings.Join(descriptions, "\n"), second.RootPath) {
		t.Fatalf("workspace completion descriptions = %v", descriptions)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestZshCompletionSpecEscapesValueAndFlattensDescription(t *testing.T) {
	got := zshCompletionSpec(`name:with\separator`, "first\nsecond")
	want := `name\:with\\separator:first second`
	if got != want {
		t.Fatalf("zshCompletionSpec() = %q, want %q", got, want)
	}
}
