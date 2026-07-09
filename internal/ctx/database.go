package ctx

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type WorkspaceRecord struct {
	ID        string
	RootPath  string
	CreatedAt string
	UpdatedAt string
}

func EnsureDatabase(workspace *Workspace) error {
	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureWorkspaceDatabase(db, workspace); err != nil {
		return err
	}

	return nil
}

func GetWorkspaceRecord(workspace *Workspace) (*WorkspaceRecord, error) {
	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := ensureWorkspaceDatabase(db, workspace); err != nil {
		return nil, err
	}

	var record WorkspaceRecord
	err = db.QueryRow(`
		SELECT id, root_path, created_at, updated_at
		FROM workspace
		WHERE id = ?
	`, workspace.ID).Scan(&record.ID, &record.RootPath, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace: %w", err)
	}
	return &record, nil
}

func WorkspaceRecordExists(workspace *Workspace) (bool, error) {
	if _, err := os.Stat(workspace.DatabasePath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to inspect database %s: %w", workspace.DatabasePath, err)
	}

	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var tableExists int
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'workspace'
		)
	`).Scan(&tableExists); err != nil {
		return false, fmt.Errorf("failed to inspect workspace table: %w", err)
	}
	if tableExists == 0 {
		return false, nil
	}

	var recordExists int
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workspace WHERE id = ?)`, workspace.ID).Scan(&recordExists); err != nil {
		return false, fmt.Errorf("failed to inspect workspace record: %w", err)
	}
	return recordExists == 1, nil
}

func ListWorkspaceRecords() ([]WorkspaceRecord, error) {
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		return nil, err
	}
	if err := migrateWorkspaceRoots(db, nil); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT id, root_path, created_at, updated_at
		FROM workspace
		ORDER BY root_path ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	defer rows.Close()

	var records []WorkspaceRecord
	for rows.Next() {
		var record WorkspaceRecord
		if err := rows.Scan(&record.ID, &record.RootPath, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to read workspace: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	return records, nil
}

func RemoveWorkspace(record WorkspaceRecord) error {
	markerPath := filepath.Join(record.RootPath, MarkerFile)
	markerID, err := readWorkspaceID(markerPath)
	switch {
	case err == nil && markerID != record.ID:
		return fmt.Errorf("refusing to remove workspace %s: %s belongs to workspace %s", record.ID, markerPath, markerID)
	case err != nil && !os.IsNotExist(err):
		return err
	}

	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		return err
	}
	if err := migrateWorkspaceRoots(db, nil); err != nil {
		return err
	}

	result, err := db.Exec(`DELETE FROM workspace WHERE id = ?`, record.ID)
	if err != nil {
		return fmt.Errorf("failed to remove workspace from database: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect removed workspace: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("workspace not found: %s", record.ID)
	}

	if err := os.RemoveAll(filepath.Join(dataRoot(), "workspaces", record.ID)); err != nil {
		return fmt.Errorf("failed to remove workspace data: %w", err)
	}
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove workspace marker: %w", err)
	}
	return nil
}

func openDatabase(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %s: %w", path, err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return db, nil
}

func createSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS workspace (
			id TEXT PRIMARY KEY,
			root_path TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS target (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			ip TEXT NOT NULL,
			os_name TEXT,
			os_accuracy INTEGER,
			os_source TEXT,
			is_primary INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
			UNIQUE (workspace_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS host (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			target_id INTEGER,
			hostname TEXT NOT NULL,
			source TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES target(id) ON DELETE SET NULL,
			UNIQUE (workspace_id, hostname)
		)`,
		`CREATE TABLE IF NOT EXISTS service (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			target_id INTEGER,
			port INTEGER NOT NULL,
			protocol TEXT NOT NULL,
			state TEXT,
			reason TEXT,
			service_name TEXT,
			product TEXT,
			version TEXT,
			extrainfo TEXT,
			tunnel TEXT,
			hostname TEXT,
			cpe TEXT,
			scripts_json TEXT,
			last_seen TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES target(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS credential (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			target_id INTEGER,
			service_id INTEGER,
			username TEXT,
			password TEXT,
			hash TEXT,
			source TEXT,
			verified INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES target(id) ON DELETE SET NULL,
			FOREIGN KEY (service_id) REFERENCES service(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS finding (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			target_id INTEGER,
			service_id INTEGER,
			title TEXT NOT NULL,
			severity TEXT,
			description TEXT,
			source TEXT,
			evidence_log_id INTEGER,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES target(id) ON DELETE SET NULL,
			FOREIGN KEY (service_id) REFERENCES service(id) ON DELETE SET NULL,
			FOREIGN KEY (evidence_log_id) REFERENCES command_log(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS command_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			command TEXT NOT NULL,
			expanded_command TEXT NOT NULL,
			status TEXT NOT NULL,
			exit_code INTEGER,
			stdout TEXT,
			stderr TEXT,
			started_at TEXT NOT NULL,
			ended_at TEXT NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS scan_run (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			target_id INTEGER NOT NULL,
			target_ip TEXT NOT NULL,
			ports TEXT NOT NULL,
			command_log_id INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES target(id) ON DELETE CASCADE,
			FOREIGN KEY (command_log_id) REFERENCES command_log(id) ON DELETE CASCADE,
			UNIQUE (workspace_id, target_id, target_ip, ports)
		)`,
		`CREATE TABLE IF NOT EXISTS note (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			command_log_id INTEGER,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
			FOREIGN KEY (command_log_id) REFERENCES command_log(id) ON DELETE SET NULL
		)`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("failed to create database schema: %w", err)
		}
	}

	return nil
}

type workspaceExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertWorkspace(db workspaceExecer, workspace *Workspace) error {
	_, err := db.Exec(`
		INSERT INTO workspace (id, root_path, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			root_path = excluded.root_path,
			updated_at = CURRENT_TIMESTAMP
	`, workspace.ID, workspace.RootPath)
	if err != nil {
		return fmt.Errorf("failed to save workspace: %w", err)
	}

	return nil
}

func ensureWorkspaceDatabase(db *sql.DB, workspace *Workspace) error {
	if err := createSchema(db); err != nil {
		return err
	}
	return migrateWorkspaceRoots(db, workspace)
}

func migrateWorkspaceRoots(db *sql.DB, preferred *Workspace) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start workspace migration: %w", err)
	}
	defer tx.Rollback()

	if err := dropWorkspaceNameColumn(tx); err != nil {
		return err
	}

	rows, err := tx.Query(`
		SELECT id, root_path
		FROM workspace
		ORDER BY updated_at DESC, created_at DESC, id DESC
	`)
	if err != nil {
		return fmt.Errorf("failed to inspect workspace roots: %w", err)
	}

	type workspaceRootRecord struct {
		id   string
		root string
	}
	var records []workspaceRootRecord
	for rows.Next() {
		var record workspaceRootRecord
		if err := rows.Scan(&record.id, &record.root); err != nil {
			rows.Close()
			return fmt.Errorf("failed to read workspace root: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close workspace roots: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to list workspace roots: %w", err)
	}

	groups := make(map[string][]string)
	var rootOrder []string
	for _, record := range records {
		if _, exists := groups[record.root]; !exists {
			rootOrder = append(rootOrder, record.root)
		}
		groups[record.root] = append(groups[record.root], record.id)
	}

	for _, root := range rootOrder {
		ids := groups[root]
		keepID := ids[0]
		if preferred != nil && preferred.RootPath == root {
			keepID = preferred.ID
		} else if markerID, err := readWorkspaceID(filepath.Join(root, MarkerFile)); err == nil && containsWorkspaceID(ids, markerID) {
			keepID = markerID
		}
		for _, id := range ids {
			if id == keepID {
				continue
			}
			if _, err := tx.Exec(`DELETE FROM workspace WHERE id = ?`, id); err != nil {
				return fmt.Errorf("failed to remove duplicate workspace %s: %w", id, err)
			}
		}
	}

	if preferred != nil {
		if _, err := tx.Exec(`DELETE FROM workspace WHERE root_path = ? AND id <> ?`, preferred.RootPath, preferred.ID); err != nil {
			return fmt.Errorf("failed to reconcile workspace root %s: %w", preferred.RootPath, err)
		}
		if err := upsertWorkspace(tx, preferred); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_root_path ON workspace(root_path)`); err != nil {
		return fmt.Errorf("failed to enforce unique workspace root paths: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit workspace migration: %w", err)
	}
	return nil
}

func containsWorkspaceID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func GetWorkspaceRecordByRoot(rootPath string) (*WorkspaceRecord, error) {
	dbPath := filepath.Join(dataRoot(), "db.sqlite")
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to inspect database %s: %w", dbPath, err)
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := createSchema(db); err != nil {
		return nil, err
	}
	if err := migrateWorkspaceRoots(db, nil); err != nil {
		return nil, err
	}

	var record WorkspaceRecord
	err = db.QueryRow(`
		SELECT id, root_path, created_at, updated_at
		FROM workspace
		WHERE root_path = ?
	`, rootPath).Scan(&record.ID, &record.RootPath, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace by root: %w", err)
	}
	return &record, nil
}

func WorkspaceDatabaseReady(workspace *Workspace) (bool, error) {
	if _, err := os.Stat(workspace.DatabasePath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to inspect database %s: %w", workspace.DatabasePath, err)
	}

	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var tableExists int
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'workspace'
		)
	`).Scan(&tableExists); err != nil {
		return false, fmt.Errorf("failed to inspect workspace table: %w", err)
	}
	if tableExists == 0 {
		return false, nil
	}

	columns, err := db.Query(`PRAGMA table_info(workspace)`)
	if err != nil {
		return false, fmt.Errorf("failed to inspect workspace columns: %w", err)
	}
	for columns.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := columns.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			columns.Close()
			return false, fmt.Errorf("failed to read workspace column: %w", err)
		}
		if name == "name" {
			columns.Close()
			return false, nil
		}
	}
	if err := columns.Close(); err != nil {
		return false, fmt.Errorf("failed to close workspace columns: %w", err)
	}

	var matchingRecords int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM workspace WHERE id = ? AND root_path = ?
	`, workspace.ID, workspace.RootPath).Scan(&matchingRecords); err != nil {
		return false, fmt.Errorf("failed to inspect workspace record: %w", err)
	}
	var rootRecords int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspace WHERE root_path = ?`, workspace.RootPath).Scan(&rootRecords); err != nil {
		return false, fmt.Errorf("failed to inspect workspace root records: %w", err)
	}
	var uniqueIndex int
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM sqlite_master
			WHERE type = 'index' AND name = 'idx_workspace_root_path'
		)
	`).Scan(&uniqueIndex); err != nil {
		return false, fmt.Errorf("failed to inspect workspace root index: %w", err)
	}
	return matchingRecords == 1 && rootRecords == 1 && uniqueIndex == 1, nil
}

func dropWorkspaceNameColumn(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA table_info(workspace)`)
	if err != nil {
		return fmt.Errorf("failed to inspect workspace columns: %w", err)
	}
	hasName := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("failed to read workspace column: %w", err)
		}
		if name == "name" {
			hasName = true
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close workspace columns: %w", err)
	}
	if !hasName {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE workspace DROP COLUMN name`); err != nil {
		return fmt.Errorf("failed to remove workspace name column: %w", err)
	}
	return nil
}
