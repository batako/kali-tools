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

func TestExistingPrefixedWorkspaceIDRemainsReadable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	const existingID = "ctx-0123456789abcdef"
	if err := os.WriteFile(filepath.Join(root, MarkerFile), []byte(existingID+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.ctx) error = %v", err)
	}

	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	if workspace.ID != existingID {
		t.Fatalf("workspace id = %q, want existing id %q", workspace.ID, existingID)
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
	for _, table := range []string{"workspace", "target", "host", "command_log", "note"} {
		var count int
		query := "SELECT COUNT(*) FROM " + table + " WHERE "
		if table == "workspace" {
			query += "id = ?"
		} else {
			query += "workspace_id = ?"
		}
		if err := db.QueryRow(query, workspace.ID).Scan(&count); err != nil {
			t.Fatalf("count %s records error = %v", table, err)
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
	if err := os.WriteFile(filepath.Join(root, MarkerFile), []byte("another-workspace\n"), 0644); err != nil {
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

func TestWorkspaceRootMigrationKeepsMarkerIDAndAddsUniqueConstraint(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	if _, err := db.Exec(`DROP INDEX idx_workspace_root_path`); err != nil {
		db.Close()
		t.Fatalf("drop unique index error = %v", err)
	}
	const staleID = "stale-workspace-id"
	if _, err := db.Exec(`
		INSERT INTO workspace (id, root_path, created_at, updated_at)
		VALUES (?, ?, '2020-01-01 00:00:00', '2030-01-01 00:00:00')
	`, staleID, root); err != nil {
		db.Close()
		t.Fatalf("insert duplicate workspace error = %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO target (workspace_id, name, ip, is_primary)
		VALUES (?, 'stale-target', '192.0.2.1', 1)
	`, staleID); err != nil {
		db.Close()
		t.Fatalf("insert stale target error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	restored, status, err := InitWorkspaceWithStatus(root)
	if err != nil {
		t.Fatalf("InitWorkspaceWithStatus() error = %v", err)
	}
	if status != WorkspaceUpdated {
		t.Fatalf("workspace status = %v, want WorkspaceUpdated", status)
	}
	if restored.ID != workspace.ID {
		t.Fatalf("workspace id = %q, want marker id %q", restored.ID, workspace.ID)
	}

	db, err = openDatabase(workspace.DatabasePath)
	if err != nil {
		t.Fatalf("openDatabase() after migration error = %v", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspace WHERE root_path = ?`, root).Scan(&count); err != nil {
		t.Fatalf("count workspaces error = %v", err)
	}
	if count != 1 {
		t.Fatalf("workspace rows for root = %d, want 1", count)
	}
	var keptID string
	if err := db.QueryRow(`SELECT id FROM workspace WHERE root_path = ?`, root).Scan(&keptID); err != nil {
		t.Fatalf("load kept workspace error = %v", err)
	}
	if keptID != workspace.ID {
		t.Fatalf("kept workspace id = %q, want %q", keptID, workspace.ID)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM target WHERE workspace_id = ?`, staleID).Scan(&count); err != nil {
		t.Fatalf("count stale targets error = %v", err)
	}
	if count != 0 {
		t.Fatalf("stale targets = %d, want 0", count)
	}
	if _, err := db.Exec(`
		INSERT INTO workspace (id, root_path)
		VALUES ('another-id', ?)
	`, root); err == nil {
		t.Fatal("duplicate root_path insert succeeded, want unique constraint error")
	}
}

func TestWorkspaceMigrationRemovesLegacyNameColumn(t *testing.T) {
	root := t.TempDir()
	ctxHome := filepath.Join(t.TempDir(), ".ctx")
	t.Setenv("CTX_HOME", ctxHome)
	const workspaceID = "legacy-workspace-id"
	if err := os.WriteFile(filepath.Join(root, MarkerFile), []byte(workspaceID+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.ctx) error = %v", err)
	}

	db, err := openDatabase(filepath.Join(ctxHome, "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE workspace (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			root_path TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		db.Close()
		t.Fatalf("create legacy workspace table error = %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO workspace (id, name, root_path)
		VALUES (?, 'legacy-name', ?)
	`, workspaceID, root); err != nil {
		db.Close()
		t.Fatalf("insert legacy workspace error = %v", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX idx_workspace_root_path ON workspace(root_path)`); err != nil {
		db.Close()
		t.Fatalf("create legacy root index error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := ensureWorkspaceDirs(workspaceFromID(workspaceID, root).DataPath); err != nil {
		t.Fatalf("ensureWorkspaceDirs() error = %v", err)
	}

	workspace, status, err := InitWorkspaceWithStatus(root)
	if err != nil {
		t.Fatalf("InitWorkspaceWithStatus() error = %v", err)
	}
	if status != WorkspaceUpdated {
		t.Fatalf("workspace status = %v, want WorkspaceUpdated", status)
	}
	if workspace.ID != workspaceID {
		t.Fatalf("workspace id = %q, want %q", workspace.ID, workspaceID)
	}

	db, err = openDatabase(workspace.DatabasePath)
	if err != nil {
		t.Fatalf("openDatabase() after migration error = %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA table_info(workspace)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info error = %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan workspace column error = %v", err)
		}
		if name == "name" {
			t.Fatal("legacy workspace name column still exists")
		}
	}
}
