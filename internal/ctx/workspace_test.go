package ctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWorkspaceCreatesMarkerAndWorkspaceDirs(t *testing.T) {
	root := t.TempDir()
	ctxHome := filepath.Join(t.TempDir(), ".ctx")
	t.Setenv("CTX_HOME", ctxHome)

	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	marker, err := os.ReadFile(filepath.Join(root, MarkerFile))
	if err != nil {
		t.Fatalf("ReadFile(.ctx) error = %v", err)
	}
	if strings.TrimSpace(string(marker)) != workspace.ID {
		t.Fatalf("marker id = %q, want %q", strings.TrimSpace(string(marker)), workspace.ID)
	}

	for _, dir := range []string{"logs", "files", "scans"} {
		if info, err := os.Stat(filepath.Join(workspace.DataPath, dir)); err != nil || !info.IsDir() {
			t.Fatalf("workspace dir %s missing or not a directory, stat error = %v", dir, err)
		}
	}

	if _, err := os.Stat(workspace.DatabasePath); err != nil {
		t.Fatalf("database file missing, stat error = %v", err)
	}

	record, err := GetWorkspaceRecord(workspace)
	if err != nil {
		t.Fatalf("GetWorkspaceRecord() error = %v", err)
	}
	if record.ID != workspace.ID {
		t.Fatalf("database workspace id = %q, want %q", record.ID, workspace.ID)
	}
	if record.RootPath != root {
		t.Fatalf("database root path = %q, want %q", record.RootPath, root)
	}
}

func TestFindWorkspaceWalksUpParents(t *testing.T) {
	root := t.TempDir()
	ctxHome := filepath.Join(t.TempDir(), ".ctx")
	t.Setenv("CTX_HOME", ctxHome)

	created, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	found, err := FindWorkspace(nested)
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("workspace id = %q, want %q", found.ID, created.ID)
	}
	if found.RootPath != root {
		t.Fatalf("root path = %q, want %q", found.RootPath, root)
	}
}

func TestFindWorkspaceReturnsHelpfulErrorOutsideWorkspace(t *testing.T) {
	t.Parallel()

	_, err := FindWorkspace(t.TempDir())
	if err == nil {
		t.Fatal("FindWorkspace() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "ctx workspace not found") {
		t.Fatalf("error = %q, want workspace not found message", err.Error())
	}
}
