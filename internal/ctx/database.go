package ctx

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type WorkspaceRecord struct {
	ID        string
	Name      string
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

	if err := createSchema(db); err != nil {
		return err
	}
	if err := upsertWorkspace(db, workspace); err != nil {
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

	if err := createSchema(db); err != nil {
		return nil, err
	}

	var record WorkspaceRecord
	err = db.QueryRow(`
		SELECT id, name, root_path, created_at, updated_at
		FROM workspace
		WHERE id = ?
	`, workspace.ID).Scan(&record.ID, &record.Name, &record.RootPath, &record.CreatedAt, &record.UpdatedAt)
	if err == nil {
		return &record, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to load workspace: %w", err)
	}

	if err := upsertWorkspace(db, workspace); err != nil {
		return nil, err
	}

	err = db.QueryRow(`
		SELECT id, name, root_path, created_at, updated_at
		FROM workspace
		WHERE id = ?
	`, workspace.ID).Scan(&record.ID, &record.Name, &record.RootPath, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace: %w", err)
	}

	return &record, nil
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
			name TEXT NOT NULL,
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

func upsertWorkspace(db *sql.DB, workspace *Workspace) error {
	name := filepath.Base(workspace.RootPath)
	if name == "." || name == string(filepath.Separator) {
		name = workspace.ID
	}

	_, err := db.Exec(`
		INSERT INTO workspace (id, name, root_path, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			root_path = excluded.root_path,
			updated_at = CURRENT_TIMESTAMP
	`, workspace.ID, name, workspace.RootPath)
	if err != nil {
		return fmt.Errorf("failed to save workspace: %w", err)
	}

	return nil
}
