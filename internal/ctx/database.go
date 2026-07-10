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
		FROM workspaces
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
			SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'
		)
	`).Scan(&tableExists); err != nil {
		return false, fmt.Errorf("failed to inspect workspace table: %w", err)
	}
	if tableExists == 0 {
		return false, nil
	}

	var recordExists int
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)`, workspace.ID).Scan(&recordExists); err != nil {
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
	if err := reconcileWorkspaceRecord(db, nil); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT id, root_path, created_at, updated_at
		FROM workspaces
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
	if err := reconcileWorkspaceRecord(db, nil); err != nil {
		return err
	}

	result, err := db.Exec(`DELETE FROM workspaces WHERE id = ?`, record.ID)
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
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return db, nil
}

func createSchema(db *sql.DB) error {
	for _, statement := range schemaStatements() {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("failed to create database schema: %w", err)
		}
	}
	return nil
}

func schemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			root_path TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (root_path)
		)`,
		`CREATE TABLE IF NOT EXISTS targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			ip TEXT NOT NULL,
			os_name TEXT,
			os_accuracy INTEGER,
			os_source TEXT,
			is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			UNIQUE (workspace_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS command_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			command TEXT NOT NULL,
			expanded_command TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'interrupted')),
			exit_code INTEGER,
			stdout TEXT,
			stderr TEXT,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS hosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			hostname TEXT NOT NULL,
			source TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
			UNIQUE (target_id, hostname)
		)`,
		`CREATE TABLE IF NOT EXISTS services (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
			protocol TEXT NOT NULL,
			state TEXT,
			reason TEXT,
			service_name TEXT,
			product TEXT,
			version TEXT,
			extrainfo TEXT,
			tunnel TEXT,
			cpe TEXT,
			last_seen TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
			UNIQUE (target_id, port, protocol)
		)`,
		`CREATE TABLE IF NOT EXISTS credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			password TEXT,
			verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
			evidence_log_id INTEGER,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
			FOREIGN KEY (evidence_log_id) REFERENCES command_logs(id) ON DELETE SET NULL,
			UNIQUE (target_id, username)
		)`,
		`CREATE TABLE IF NOT EXISTS scan_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			target_ip TEXT NOT NULL,
			ports TEXT NOT NULL,
			command_log_id INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
			FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE CASCADE,
			UNIQUE (target_id, target_ip, ports)
		)`,
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_targets_one_primary ON targets(workspace_id) WHERE is_primary = 1`,
		`CREATE INDEX IF NOT EXISTS idx_targets_workspace_id ON targets(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hosts_target_id ON hosts(target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_services_target_id ON services(target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_credentials_target_id ON credentials(target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_command_logs_workspace_started_at ON command_logs(workspace_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_scan_runs_target_id ON scan_runs(target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_workspace_created_at ON notes(workspace_id, created_at DESC)`,
	}
}

type workspaceExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertWorkspace(db workspaceExecer, workspace *Workspace) error {
	_, err := db.Exec(`
		INSERT INTO workspaces (id, root_path, created_at, updated_at)
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
	return reconcileWorkspaceRecord(db, workspace)
}

func reconcileWorkspaceRecord(db *sql.DB, preferred *Workspace) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start workspace record update: %w", err)
	}
	defer tx.Rollback()

	if preferred != nil {
		if _, err := tx.Exec(`DELETE FROM workspaces WHERE root_path = ? AND id <> ?`, preferred.RootPath, preferred.ID); err != nil {
			return fmt.Errorf("failed to reconcile workspace root %s: %w", preferred.RootPath, err)
		}
		if err := upsertWorkspace(tx, preferred); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit workspace record update: %w", err)
	}
	return nil
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
	if err := reconcileWorkspaceRecord(db, nil); err != nil {
		return nil, err
	}

	var record WorkspaceRecord
	err = db.QueryRow(`
		SELECT id, root_path, created_at, updated_at
		FROM workspaces
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
			SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'
		)
	`).Scan(&tableExists); err != nil {
		return false, fmt.Errorf("failed to inspect workspace table: %w", err)
	}
	if tableExists == 0 {
		return false, nil
	}

	columns, err := db.Query(`PRAGMA table_info(workspaces)`)
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
		SELECT COUNT(*) FROM workspaces WHERE id = ? AND root_path = ?
	`, workspace.ID, workspace.RootPath).Scan(&matchingRecords); err != nil {
		return false, fmt.Errorf("failed to inspect workspace record: %w", err)
	}
	var rootRecords int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE root_path = ?`, workspace.RootPath).Scan(&rootRecords); err != nil {
		return false, fmt.Errorf("failed to inspect workspace root records: %w", err)
	}
	return matchingRecords == 1 && rootRecords == 1, nil
}
