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

	firstOutput := out.String()
	out.Reset()
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace init) second error = %v", err)
	}
	if !strings.Contains(out.String(), "ctx workspace already initialized") {
		t.Fatalf("second output = %q, want already initialized", out.String())
	}
	if strings.Contains(out.String(), "\ninitialized ctx workspace") {
		t.Fatalf("second output = %q, should not report a new initialization", out.String())
	}

	firstID := strings.TrimSpace(strings.TrimPrefix(firstOutput, "initialized ctx workspace "))
	secondID := strings.TrimSpace(strings.TrimPrefix(out.String(), "ctx workspace already initialized "))
	if firstID != secondID {
		t.Fatalf("workspace id changed from %q to %q", firstID, secondID)
	}

	workspace, err := FindWorkspace(root)
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	if _, err := db.Exec(`DELETE FROM workspace WHERE id = ?`, workspace.ID); err != nil {
		db.Close()
		t.Fatalf("delete workspace record error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace init) repair error = %v", err)
	}
	if got, want := out.String(), "updated ctx workspace "+workspace.ID+"\n"; got != want {
		t.Fatalf("repair output = %q, want %q", got, want)
	}
	exists, err := WorkspaceRecordExists(workspace)
	if err != nil {
		t.Fatalf("WorkspaceRecordExists() error = %v", err)
	}
	if !exists {
		t.Fatal("workspace record was not restored")
	}

	out.Reset()
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace init) after repair error = %v", err)
	}
	if got, want := out.String(), "ctx workspace already initialized "+workspace.ID+"\n"; got != want {
		t.Fatalf("after repair output = %q, want %q", got, want)
	}

	if err := os.Remove(workspace.DatabasePath); err != nil {
		t.Fatalf("Remove(database) error = %v", err)
	}
	if err := os.WriteFile(workspace.DatabasePath, nil, 0644); err != nil {
		t.Fatalf("WriteFile(empty database) error = %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace init) empty database repair error = %v", err)
	}
	if got, want := out.String(), "updated ctx workspace "+workspace.ID+"\n"; got != want {
		t.Fatalf("empty database repair output = %q, want %q", got, want)
	}
	exists, err = WorkspaceRecordExists(workspace)
	if err != nil {
		t.Fatalf("WorkspaceRecordExists() after empty database error = %v", err)
	}
	if !exists {
		t.Fatal("workspace record was not restored in empty database")
	}

	if err := os.Remove(filepath.Join(root, MarkerFile)); err != nil {
		t.Fatalf("Remove(.ctx) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace init) marker restore error = %v", err)
	}
	if got, want := out.String(), "updated ctx workspace "+workspace.ID+"\n"; got != want {
		t.Fatalf("marker restore output = %q, want %q", got, want)
	}
	marker, err := os.ReadFile(filepath.Join(root, MarkerFile))
	if err != nil {
		t.Fatalf("ReadFile(restored .ctx) error = %v", err)
	}
	if got := strings.TrimSpace(string(marker)); got != workspace.ID {
		t.Fatalf("restored marker id = %q, want %q", got, workspace.ID)
	}
}

func TestWorkspaceDefaultViewInitializesCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace) error = %v", err)
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

func TestWorkspaceRemoveByIDWithShortYes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	chdirForTest(t, outside)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "rm", workspace.ID, "-y"}, &out); err != nil {
		t.Fatalf("Run(ctx workspace rm <id> -y) error = %v", err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("output = %q, -y should skip confirmation", out.String())
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
