package ctx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceInitCreatesCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace init) error = %v", err)
	}
	if !strings.Contains(out.String(), "initialized ctx workspace") {
		t.Fatalf("output = %q, want initialized workspace", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, MarkerFile)); err != nil {
		t.Fatalf("workspace marker missing: %v", err)
	}
}

func TestWorkspaceRemoveUsesCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	chdirForTest(t, root)
	setWorkspaceInputForTest(t, "yes\n")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "rm"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace rm) error = %v", err)
	}
	if !strings.Contains(out.String(), "Remove workspace") || !strings.Contains(out.String(), "removed workspace: "+workspace.ID) {
		t.Fatalf("output = %q, want confirmation and removal", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, MarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("marker Stat() error = %v, want not exist", err)
	}
}

func TestWorkspaceRemoveOutsideWorkspaceListsAndSelects(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	alphaRoot := filepath.Join(base, "alpha")
	betaRoot := filepath.Join(base, "beta")
	outside := filepath.Join(base, "outside")
	for _, path := range []string{alphaRoot, betaRoot, outside} {
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatalf("Mkdir(%s) error = %v", path, err)
		}
	}
	alpha, err := InitWorkspace(alphaRoot)
	if err != nil {
		t.Fatalf("InitWorkspace(alpha) error = %v", err)
	}
	beta, err := InitWorkspace(betaRoot)
	if err != nil {
		t.Fatalf("InitWorkspace(beta) error = %v", err)
	}
	chdirForTest(t, outside)
	setWorkspaceInputForTest(t, "2\ny\n")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "rm"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace rm) error = %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"1  " + alpha.ID + "  " + alphaRoot,
		"2  " + beta.ID + "  " + betaRoot,
		"Select workspace to remove",
		"removed workspace: " + beta.ID,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want %q", text, want)
		}
	}
	if _, err := os.Stat(filepath.Join(betaRoot, MarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("beta marker Stat() error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(alphaRoot, MarkerFile)); err != nil {
		t.Fatalf("alpha marker was removed: %v", err)
	}

	records, err := ListWorkspaceRecords()
	if err != nil {
		t.Fatalf("ListWorkspaceRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != alpha.ID {
		t.Fatalf("workspaces = %+v, want only alpha", records)
	}
}

func TestWorkspaceRemoveByIDWithYes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	chdirForTest(t, outside)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "rm", workspace.ID, "--yes"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace rm <id> --yes) error = %v", err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("output = %q, --yes should skip confirmation", out.String())
	}
}

func TestWorkspaceList(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "ls"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace ls) error = %v", err)
	}
	if got, want := out.String(), workspace.ID+"  "+root+"\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func chdirForTest(t *testing.T, path string) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("Chdir(%s) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})
}

func setWorkspaceInputForTest(t *testing.T, input string) {
	t.Helper()
	old := workspaceStdin
	workspaceStdin = strings.NewReader(input)
	t.Cleanup(func() { workspaceStdin = old })
}
