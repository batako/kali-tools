package ctx

import (
	"os"
	"path/filepath"
	"regexp"
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
	markerText := strings.TrimSpace(string(marker))
	if markerText != workspace.UUID {
		t.Fatalf("marker text = %q, want UUID %q", markerText, workspace.UUID)
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
		t.Fatalf("database workspace id = %d, want %d", record.ID, workspace.ID)
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

func TestNewWorkspaceIDUsesUUIDWithoutCtxPrefix(t *testing.T) {
	t.Parallel()

	id, err := newWorkspaceID()
	if err != nil {
		t.Fatalf("newWorkspaceID() error = %v", err)
	}

	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(id) {
		t.Fatalf("workspace id = %q, want UUID version 4", id)
	}
	if strings.HasPrefix(id, "ctx-") {
		t.Fatalf("workspace id = %q, must not have ctx prefix", id)
	}
}

func TestNewWorkspaceIDsDoNotRepeat(t *testing.T) {
	t.Parallel()

	const count = 1000
	ids := make(map[string]struct{}, count)
	for range count {
		id, err := newWorkspaceID()
		if err != nil {
			t.Fatalf("newWorkspaceID() error = %v", err)
		}
		if _, exists := ids[id]; exists {
			t.Fatalf("newWorkspaceID() generated duplicate %q", id)
		}
		ids[id] = struct{}{}
	}
}

func TestSameNamedDirectoriesGetDifferentWorkspaceIDs(t *testing.T) {
	ctxHome := filepath.Join(t.TempDir(), ".ctx")
	t.Setenv("CTX_HOME", ctxHome)

	firstRoot := filepath.Join(t.TempDir(), "project")
	secondRoot := filepath.Join(t.TempDir(), "project")
	for _, root := range []string{firstRoot, secondRoot} {
		if err := os.Mkdir(root, 0755); err != nil {
			t.Fatalf("Mkdir(%s) error = %v", root, err)
		}
	}

	first, err := InitWorkspace(firstRoot)
	if err != nil {
		t.Fatalf("InitWorkspace(first) error = %v", err)
	}
	second, err := InitWorkspace(secondRoot)
	if err != nil {
		t.Fatalf("InitWorkspace(second) error = %v", err)
	}

	if first.ID == second.ID {
		t.Fatalf("workspace ids are both %q, want different ids", first.ID)
	}
}

func TestInitWorkspaceRejectsLegacyMarkerFormats(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	for _, marker := range []string{
		"ctx-0123456789abcdef\n",
		"16\nb13583ca-3b64-42b9-bb8d-8a7d209847cf\n",
		"16\n",
	} {
		if err := os.WriteFile(filepath.Join(root, MarkerFile), []byte(marker), 0644); err != nil {
			t.Fatalf("WriteFile(.ctx) error = %v", err)
		}
		if _, err := InitWorkspace(root); err == nil {
			t.Fatalf("InitWorkspace() marker %q error = nil, want invalid marker", marker)
		}
	}
}

func TestRemoveWorkspaceDeletesMarkerDataAndDatabaseRecords(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddHost(workspace, "target.example", ""); err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if _, err := SaveCommandLog(workspace, CommandLog{
		Command:         "echo test",
		ExpandedCommand: "echo test",
		Status:          "success",
		StartedAt:       "2026-07-09T00:00:00Z",
		EndedAt:         "2026-07-09T00:00:01Z",
	}); err != nil {
		t.Fatalf("SaveCommandLog() error = %v", err)
	}
	if _, err := SaveNote(workspace, "workspace note"); err != nil {
		t.Fatalf("SaveNote() error = %v", err)
	}

	record, err := GetWorkspaceRecord(workspace)
	if err != nil {
		t.Fatalf("GetWorkspaceRecord() error = %v", err)
	}
	if err := RemoveWorkspace(*record); err != nil {
		t.Fatalf("RemoveWorkspace() error = %v", err)
	}

	for _, path := range []string{filepath.Join(root, MarkerFile), workspace.DataPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) error = %v, want not exist", path, err)
		}
	}

	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()
	for _, table := range []string{"workspaces", "targets", "hosts", "command_logs", "notes"} {
		var count int
		query := "SELECT COUNT(*) FROM " + table
		switch table {
		case "workspaces":
			query += " WHERE id = ?"
		case "targets", "command_logs", "notes":
			query += " WHERE workspace_id = ?"
		default:
			query += ""
		}
		if table == "hosts" {
			if err := db.QueryRow(query).Scan(&count); err != nil {
				t.Fatalf("count %s records error = %v", table, err)
			}
		} else {
			if err := db.QueryRow(query, workspace.ID).Scan(&count); err != nil {
				t.Fatalf("count %s records error = %v", table, err)
			}
		}
		if count != 0 {
			t.Fatalf("%s records = %d, want 0", table, count)
		}
	}
}

func TestRemoveWorkspaceRefusesMismatchedMarker(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	record, err := GetWorkspaceRecord(workspace)
	if err != nil {
		t.Fatalf("GetWorkspaceRecord() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, MarkerFile), []byte("b13583ca-3b64-42b9-bb8d-8a7d209847cf\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.ctx) error = %v", err)
	}

	err = RemoveWorkspace(*record)
	if err == nil || !strings.Contains(err.Error(), "refusing to remove") {
		t.Fatalf("RemoveWorkspace() error = %v, want marker mismatch", err)
	}
	if _, err := os.Stat(workspace.DataPath); err != nil {
		t.Fatalf("workspace data was removed: %v", err)
	}
}
