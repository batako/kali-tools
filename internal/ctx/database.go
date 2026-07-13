package ctx

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

const (
	currentSchemaVersion     = 3
	v100SchemaVersion        = 1
	latestSchemaSnapshotPath = "schema.sql"
)

//go:embed schema.sql migrations/*.sql
var embeddedMigrations embed.FS

type WorkspaceRecord struct {
	ID        int64
	UUID      string
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
	var row *sql.Row
	switch {
	case workspace.ID > 0:
		row = db.QueryRow(`
			SELECT id, name, path, created_at, updated_at
			FROM workspaces
			WHERE id = ?
		`, workspace.ID)
	case workspace.UUID != "":
		row = db.QueryRow(`
			SELECT id, name, path, created_at, updated_at
			FROM workspaces
			WHERE name = ?
		`, workspace.UUID)
	default:
		row = db.QueryRow(`
			SELECT id, name, path, created_at, updated_at
			FROM workspaces
			WHERE path = ?
		`, workspace.RootPath)
	}
	err = row.Scan(&record.ID, &record.UUID, &record.RootPath, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace: %w", err)
	}
	record.Name = workspaceName(record.RootPath)
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
	switch {
	case workspace.ID > 0:
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)`, workspace.ID).Scan(&recordExists); err != nil {
			return false, fmt.Errorf("failed to inspect workspace record: %w", err)
		}
	case workspace.UUID != "":
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workspaces WHERE name = ?)`, workspace.UUID).Scan(&recordExists); err != nil {
			return false, fmt.Errorf("failed to inspect workspace record: %w", err)
		}
	default:
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workspaces WHERE path = ?)`, workspace.RootPath).Scan(&recordExists); err != nil {
			return false, fmt.Errorf("failed to inspect workspace record: %w", err)
		}
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
		SELECT id, name, path, created_at, updated_at
		FROM workspaces
		ORDER BY path ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	defer rows.Close()

	var records []WorkspaceRecord
	for rows.Next() {
		var record WorkspaceRecord
		if err := rows.Scan(&record.ID, &record.UUID, &record.RootPath, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to read workspace: %w", err)
		}
		record.Name = workspaceName(record.RootPath)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	return records, nil
}

func RemoveWorkspace(record WorkspaceRecord) error {
	markerPath := filepath.Join(record.RootPath, MarkerFile)
	markerID, markerUUID, err := readWorkspaceMarker(markerPath)
	switch {
	case err == nil && ((markerID > 0 && markerID != record.ID) || (markerUUID != "" && markerUUID != record.UUID)):
		return fmt.Errorf("refusing to remove workspace %d: %s belongs to another workspace", record.ID, markerPath)
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
		return fmt.Errorf("workspace not found: %d", record.ID)
	}

	if err := os.RemoveAll(filepath.Join(dataRoot(), "workspaces", record.UUID)); err != nil {
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
	empty, err := databaseHasNoUserTables(db)
	if err != nil {
		return err
	}
	if empty {
		return createLatestSchema(db, embeddedMigrations, latestSchemaSnapshotPath, currentSchemaVersion)
	}

	if err := baselineLegacyDatabase(db, embeddedMigrations, "migrations"); err != nil {
		return err
	}
	return applyPendingMigrations(db, embeddedMigrations, "migrations")
}

func createLatestSchema(db *sql.DB, fsys fs.FS, schemaPath string, version uint) error {
	schema, err := fs.ReadFile(fsys, schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read embedded schema %s: %w", schemaPath, err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start schema creation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(string(schema)); err != nil {
		return fmt.Errorf("failed to create database schema: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema creation: %w", err)
	}
	return forceSchemaVersion(db, fsys, "migrations", version)
}

func baselineLegacyDatabase(db *sql.DB, fsys fs.FS, migrationsPath string) error {
	hasMigrations, err := tableExists(db, "schema_migrations")
	if err != nil {
		return err
	}
	if hasMigrations {
		return nil
	}

	if err := validateCurrentSchema(db); err == nil {
		return forceSchemaVersion(db, fsys, migrationsPath, currentSchemaVersion)
	}

	if err := validateV100Schema(db); err != nil {
		return fmt.Errorf("database schema is not managed by migrations and is not a recognized ctx schema: %w", err)
	}
	return forceSchemaVersion(db, fsys, migrationsPath, v100SchemaVersion)
}

func applyPendingMigrations(db *sql.DB, fsys fs.FS, migrationsPath string) error {
	migrator, err := newMigrator(db, fsys, migrationsPath)
	if err != nil {
		return err
	}
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to apply database migrations: %w", err)
	}
	return nil
}

func forceSchemaVersion(db *sql.DB, fsys fs.FS, migrationsPath string, version uint) error {
	migrator, err := newMigrator(db, fsys, migrationsPath)
	if err != nil {
		return err
	}
	if err := migrator.Force(int(version)); err != nil {
		return fmt.Errorf("failed to mark database schema version %d: %w", version, err)
	}
	return nil
}

func newMigrator(db *sql.DB, fsys fs.FS, migrationsPath string) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(fsys, migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded migrations: %w", err)
	}
	databaseDriver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{
		DatabaseName: "ctx",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize migration database driver: %w", err)
	}
	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", databaseDriver)
	if err != nil {
		_ = sourceDriver.Close()
		return nil, fmt.Errorf("failed to initialize database migrations: %w", err)
	}
	return migrator, nil
}

func tableExists(db *sql.DB, table string) (bool, error) {
	var exists int
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?
		)
	`, table).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to inspect table %s: %w", table, err)
	}
	return exists == 1, nil
}

func databaseHasNoUserTables(db *sql.DB) (bool, error) {
	rows, err := db.Query(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'schema_migrations'
	`)
	if err != nil {
		return false, fmt.Errorf("failed to inspect database tables: %w", err)
	}
	defer rows.Close()
	hasTables := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("failed to inspect database tables: %w", err)
	}
	return !hasTables, nil
}

type schemaColumn struct {
	Name       string
	Type       string
	NotNull    bool
	PrimaryKey bool
	Default    string
}

type schemaTable struct {
	Name         string
	Columns      []schemaColumn
	SQLFragments []string
}

var v100SchemaTables = []schemaTable{
	{
		Name: "workspaces",
		Columns: []schemaColumn{
			{Name: "id", Type: "TEXT", PrimaryKey: true},
			{Name: "root_path", Type: "TEXT", NotNull: true},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"UNIQUE (root_path)"},
	},
	{
		Name: "targets",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "workspace_id", Type: "TEXT", NotNull: true},
			{Name: "name", Type: "TEXT", NotNull: true},
			{Name: "ip", Type: "TEXT", NotNull: true},
			{Name: "os_name", Type: "TEXT"},
			{Name: "os_accuracy", Type: "INTEGER"},
			{Name: "os_source", Type: "TEXT"},
			{Name: "is_primary", Type: "INTEGER", NotNull: true, Default: "0"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (is_primary IN (0, 1))", "FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE", "UNIQUE (workspace_id, name)"},
	},
	{
		Name: "command_logs",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "workspace_id", Type: "TEXT", NotNull: true},
			{Name: "command", Type: "TEXT", NotNull: true},
			{Name: "expanded_command", Type: "TEXT", NotNull: true},
			{Name: "status", Type: "TEXT", NotNull: true},
			{Name: "exit_code", Type: "INTEGER"},
			{Name: "stdout", Type: "TEXT"},
			{Name: "stderr", Type: "TEXT"},
			{Name: "started_at", Type: "TEXT", NotNull: true},
			{Name: "ended_at", Type: "TEXT"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (status IN ('running', 'success', 'failed', 'interrupted'))", "FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE"},
	},
	{
		Name: "hosts",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "hostname", Type: "TEXT", NotNull: true},
			{Name: "source", Type: "TEXT"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "UNIQUE (target_id, hostname)"},
	},
	{
		Name: "services",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "port", Type: "INTEGER", NotNull: true},
			{Name: "protocol", Type: "TEXT", NotNull: true},
			{Name: "state", Type: "TEXT"},
			{Name: "reason", Type: "TEXT"},
			{Name: "service_name", Type: "TEXT"},
			{Name: "product", Type: "TEXT"},
			{Name: "version", Type: "TEXT"},
			{Name: "extrainfo", Type: "TEXT"},
			{Name: "tunnel", Type: "TEXT"},
			{Name: "cpe", Type: "TEXT"},
			{Name: "last_seen", Type: "TEXT"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (port BETWEEN 1 AND 65535)", "FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "UNIQUE (target_id, port, protocol)"},
	},
	{
		Name: "credentials",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "scope", Type: "TEXT", NotNull: true},
			{Name: "username", Type: "TEXT", NotNull: true},
			{Name: "password", Type: "TEXT"},
			{Name: "verified", Type: "INTEGER", NotNull: true, Default: "0"},
			{Name: "evidence_log_id", Type: "INTEGER"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (verified IN (0, 1))", "FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "FOREIGN KEY (evidence_log_id) REFERENCES command_logs(id) ON DELETE SET NULL", "UNIQUE (target_id, scope, username)"},
	},
	{
		Name: "scan_runs",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "target_ip", Type: "TEXT", NotNull: true},
			{Name: "ports", Type: "TEXT", NotNull: true},
			{Name: "command_log_id", Type: "INTEGER", NotNull: true},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE CASCADE", "UNIQUE (target_id, target_ip, ports)"},
	},
	{
		Name: "notes",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "workspace_id", Type: "TEXT", NotNull: true},
			{Name: "body", Type: "TEXT", NotNull: true},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE"},
	},
}

var v100SchemaIndexes = map[string]string{
	"idx_targets_one_primary":               "targets",
	"idx_targets_workspace_id":              "targets",
	"idx_hosts_target_id":                   "hosts",
	"idx_services_target_id":                "services",
	"idx_credentials_target_id":             "credentials",
	"idx_command_logs_workspace_started_at": "command_logs",
	"idx_scan_runs_target_id":               "scan_runs",
	"idx_notes_workspace_created_at":        "notes",
}

var currentSchemaIndexes = map[string]string{
	"idx_targets_one_primary":               "targets",
	"idx_targets_workspace_id":              "targets",
	"idx_hosts_target_id":                   "hosts",
	"idx_services_target_id":                "services",
	"idx_credentials_target_id":             "credentials",
	"idx_command_logs_workspace_started_at": "command_logs",
	"idx_scan_runs_target_id":               "scan_runs",
	"idx_notes_workspace_created_at":        "notes",
	"idx_web_discoveries_target_id":         "web_discoveries",
	"idx_web_discoveries_command_log_id":    "web_discoveries",
}

var currentSchemaTables = []schemaTable{
	{
		Name: "workspaces",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "name", Type: "TEXT", NotNull: true},
			{Name: "path", Type: "TEXT", NotNull: true},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"UNIQUE (path)"},
	},
	{
		Name: "targets",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "workspace_id", Type: "INTEGER", NotNull: true},
			{Name: "name", Type: "TEXT", NotNull: true},
			{Name: "ip", Type: "TEXT", NotNull: true},
			{Name: "os_name", Type: "TEXT"},
			{Name: "os_accuracy", Type: "INTEGER"},
			{Name: "os_source", Type: "TEXT"},
			{Name: "is_primary", Type: "INTEGER", NotNull: true, Default: "0"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (is_primary IN (0, 1))", "FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE", "UNIQUE (workspace_id, name)"},
	},
	{
		Name: "command_logs",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "workspace_id", Type: "INTEGER", NotNull: true},
			{Name: "command", Type: "TEXT", NotNull: true},
			{Name: "expanded_command", Type: "TEXT", NotNull: true},
			{Name: "status", Type: "TEXT", NotNull: true},
			{Name: "exit_code", Type: "INTEGER"},
			{Name: "stdout", Type: "TEXT"},
			{Name: "stderr", Type: "TEXT"},
			{Name: "started_at", Type: "TEXT", NotNull: true},
			{Name: "ended_at", Type: "TEXT"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (status IN ('running', 'success', 'failed', 'interrupted'))", "FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE"},
	},
	{
		Name: "hosts",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "hostname", Type: "TEXT", NotNull: true},
			{Name: "source", Type: "TEXT"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "UNIQUE (target_id, hostname)"},
	},
	{
		Name: "services",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "port", Type: "INTEGER", NotNull: true},
			{Name: "protocol", Type: "TEXT", NotNull: true},
			{Name: "state", Type: "TEXT"},
			{Name: "reason", Type: "TEXT"},
			{Name: "service_name", Type: "TEXT"},
			{Name: "product", Type: "TEXT"},
			{Name: "version", Type: "TEXT"},
			{Name: "extrainfo", Type: "TEXT"},
			{Name: "tunnel", Type: "TEXT"},
			{Name: "cpe", Type: "TEXT"},
			{Name: "last_seen", Type: "TEXT"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (port BETWEEN 1 AND 65535)", "FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "UNIQUE (target_id, port, protocol)"},
	},
	{
		Name: "credentials",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "scope", Type: "TEXT", NotNull: true},
			{Name: "username", Type: "TEXT", NotNull: true},
			{Name: "password", Type: "TEXT"},
			{Name: "verified", Type: "INTEGER", NotNull: true, Default: "0"},
			{Name: "evidence_log_id", Type: "INTEGER"},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"CHECK (verified IN (0, 1))", "FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "FOREIGN KEY (evidence_log_id) REFERENCES command_logs(id) ON DELETE SET NULL", "UNIQUE (target_id, scope, username)"},
	},
	{
		Name: "scan_runs",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "target_ip", Type: "TEXT", NotNull: true},
			{Name: "ports", Type: "TEXT", NotNull: true},
			{Name: "command_log_id", Type: "INTEGER", NotNull: true},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE", "FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE CASCADE", "UNIQUE (target_id, target_ip, ports)"},
	},
	{
		Name: "notes",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "workspace_id", Type: "INTEGER", NotNull: true},
			{Name: "body", Type: "TEXT", NotNull: true},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{"FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE"},
	},
	{
		Name: "web_discoveries",
		Columns: []schemaColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "target_id", Type: "INTEGER", NotNull: true},
			{Name: "url", Type: "TEXT", NotNull: true},
			{Name: "path", Type: "TEXT", NotNull: true},
			{Name: "status_code", Type: "INTEGER", NotNull: true},
			{Name: "content_length", Type: "INTEGER"},
			{Name: "redirect_url", Type: "TEXT"},
			{Name: "source_tool", Type: "TEXT", NotNull: true},
			{Name: "wordlist", Type: "TEXT", NotNull: true},
			{Name: "command_log_id", Type: "INTEGER"},
			{Name: "discovered_at", Type: "TEXT", NotNull: true},
			{Name: "created_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
			{Name: "updated_at", Type: "TEXT", NotNull: true, Default: "CURRENT_TIMESTAMP"},
		},
		SQLFragments: []string{
			"FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE",
			"FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE SET NULL",
		},
	},
}

func validateCurrentSchema(db *sql.DB) error {
	if err := validateIntegrity(db); err != nil {
		return err
	}
	for _, table := range currentSchemaTables {
		if err := validateSchemaTable(db, table); err != nil {
			return err
		}
	}
	for index, table := range currentSchemaIndexes {
		if err := validateSchemaIndex(db, index, table); err != nil {
			return err
		}
	}
	return nil
}

func validateV100Schema(db *sql.DB) error {
	if err := validateIntegrity(db); err != nil {
		return err
	}
	for _, table := range v100SchemaTables {
		if err := validateSchemaTable(db, table); err != nil {
			return err
		}
	}
	for index, table := range v100SchemaIndexes {
		if err := validateSchemaIndex(db, index, table); err != nil {
			return err
		}
	}
	return nil
}

func validateIntegrity(db *sql.DB) error {
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("integrity_check failed to run: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check returned %q", result)
	}
	return nil
}

func validateSchemaTable(db *sql.DB, expected schemaTable) error {
	exists, err := tableExists(db, expected.Name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("missing required table %s", expected.Name)
	}

	columns, err := loadTableColumns(db, expected.Name)
	if err != nil {
		return err
	}
	for _, expectedColumn := range expected.Columns {
		actual, ok := columns[expectedColumn.Name]
		if !ok {
			return fmt.Errorf("%s.%s is missing", expected.Name, expectedColumn.Name)
		}
		if actual.Type != expectedColumn.Type {
			return fmt.Errorf("%s.%s type = %q, want %q", expected.Name, expectedColumn.Name, actual.Type, expectedColumn.Type)
		}
		if actual.NotNull != expectedColumn.NotNull {
			return fmt.Errorf("%s.%s notnull = %t, want %t", expected.Name, expectedColumn.Name, actual.NotNull, expectedColumn.NotNull)
		}
		if actual.PrimaryKey != expectedColumn.PrimaryKey {
			return fmt.Errorf("%s.%s primary key = %t, want %t", expected.Name, expectedColumn.Name, actual.PrimaryKey, expectedColumn.PrimaryKey)
		}
		if normalizeDefault(actual.Default) != normalizeDefault(expectedColumn.Default) {
			return fmt.Errorf("%s.%s default = %q, want %q", expected.Name, expectedColumn.Name, actual.Default, expectedColumn.Default)
		}
	}

	sqlText, err := tableSQL(db, expected.Name)
	if err != nil {
		return err
	}
	normalizedSQL := normalizeSQL(sqlText)
	for _, fragment := range expected.SQLFragments {
		if !strings.Contains(normalizedSQL, normalizeSQL(fragment)) {
			return fmt.Errorf("%s DDL is missing %q", expected.Name, fragment)
		}
	}
	return nil
}

func loadTableColumns(db *sql.DB, table string) (map[string]schemaColumn, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, fmt.Errorf("failed to inspect %s columns: %w", table, err)
	}
	defer rows.Close()

	columns := make(map[string]schemaColumn)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("failed to read %s column: %w", table, err)
		}
		column := schemaColumn{
			Name:       name,
			Type:       strings.ToUpper(columnType),
			NotNull:    notNull == 1,
			PrimaryKey: primaryKey > 0,
		}
		if defaultValue.Valid {
			column.Default = defaultValue.String
		}
		columns[name] = column
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to inspect %s columns: %w", table, err)
	}
	return columns, nil
}

func tableSQL(db *sql.DB, table string) (string, error) {
	var sqlText string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&sqlText); err != nil {
		return "", fmt.Errorf("failed to load %s DDL: %w", table, err)
	}
	return sqlText, nil
}

func validateSchemaIndex(db *sql.DB, index, table string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ? AND tbl_name = ?`, index, table).Scan(&count); err != nil {
		return fmt.Errorf("failed to inspect index %s: %w", index, err)
	}
	if count != 1 {
		return fmt.Errorf("missing required index %s on %s", index, table)
	}
	return nil
}

func normalizeDefault(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "()")
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func tableColumnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("failed to inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("failed to read %s column: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("failed to inspect %s columns: %w", table, err)
	}
	return false, nil
}

type workspaceExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertWorkspace(db workspaceExecer, workspace *Workspace) error {
	_, err := db.Exec(`
		INSERT INTO workspaces (id, name, path, created_at, updated_at)
		VALUES (
			CASE WHEN ? > 0 THEN ? ELSE NULL END,
			?,
			?,
			CURRENT_TIMESTAMP,
			CURRENT_TIMESTAMP
		)
		ON CONFLICT(name) DO UPDATE SET
			path = excluded.path,
			updated_at = CURRENT_TIMESTAMP
	`, workspace.ID, workspace.ID, workspace.UUID, workspace.RootPath)
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
		if _, err := tx.Exec(`DELETE FROM workspaces WHERE path = ? AND name <> ?`, preferred.RootPath, preferred.UUID); err != nil {
			return fmt.Errorf("failed to reconcile workspace root %s: %w", preferred.RootPath, err)
		}
		if preferred.UUID == "" {
			return fmt.Errorf("failed to reconcile workspace root %s: missing name", preferred.RootPath)
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
		SELECT id, name, path, created_at, updated_at
		FROM workspaces
		WHERE path = ?
	`, rootPath).Scan(&record.ID, &record.UUID, &record.RootPath, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace by root: %w", err)
	}
	record.Name = workspaceName(record.RootPath)
	return &record, nil
}

func GetWorkspaceRecordByUUID(name string) (*WorkspaceRecord, error) {
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := createSchema(db); err != nil {
		return nil, err
	}

	var record WorkspaceRecord
	err = db.QueryRow(`
		SELECT id, name, path, created_at, updated_at
		FROM workspaces
		WHERE name = ?
	`, name).Scan(&record.ID, &record.UUID, &record.RootPath, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace by name: %w", err)
	}
	record.Name = workspaceName(record.RootPath)
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
	hasNameColumn := false
	hasPathColumn := false
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
			hasNameColumn = true
		}
		if name == "path" {
			hasPathColumn = true
		}
	}
	if err := columns.Close(); err != nil {
		return false, fmt.Errorf("failed to close workspace columns: %w", err)
	}
	if !hasNameColumn || !hasPathColumn {
		return false, nil
	}

	var matchingRecords int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM workspaces WHERE name = ? AND path = ?
	`, workspace.UUID, workspace.RootPath).Scan(&matchingRecords); err != nil {
		return false, fmt.Errorf("failed to inspect workspace record: %w", err)
	}
	var rootRecords int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE path = ?`, workspace.RootPath).Scan(&rootRecords); err != nil {
		return false, fmt.Errorf("failed to inspect workspace root records: %w", err)
	}
	return matchingRecords == 1 && rootRecords == 1, nil
}

func migrateWorkspaceSchema(db *sql.DB) error {
	needsMigration, err := workspaceSchemaNeedsMigration(db)
	if err != nil || !needsMigration {
		return err
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("failed to disable foreign keys during migration: %w", err)
	}
	defer func() {
		_, _ = db.Exec(`PRAGMA foreign_keys = ON`)
	}()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start workspace migration: %w", err)
	}
	defer tx.Rollback()

	for _, table := range []string{"credentials", "services", "hosts", "scan_runs", "notes", "command_logs", "targets", "workspaces"} {
		if _, err := tx.Exec(`ALTER TABLE ` + table + ` RENAME TO ` + table + `_old`); err != nil {
			if !strings.Contains(err.Error(), "no such table") {
				return fmt.Errorf("failed to rename %s table: %w", table, err)
			}
		}
	}

	for _, statement := range []string{
		`CREATE TABLE workspaces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			path TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (path)
		)`,
		`CREATE TABLE targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL,
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
		`CREATE TABLE command_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL,
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
		`CREATE TABLE hosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			hostname TEXT NOT NULL,
			source TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
			UNIQUE (target_id, hostname)
		)`,
		`CREATE TABLE services (
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
		`CREATE TABLE credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			scope TEXT NOT NULL,
			username TEXT NOT NULL,
			password TEXT,
			verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
			evidence_log_id INTEGER,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
			FOREIGN KEY (evidence_log_id) REFERENCES command_logs(id) ON DELETE SET NULL,
			UNIQUE (target_id, scope, username)
		)`,
		`CREATE TABLE scan_runs (
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
		`CREATE TABLE notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("failed to create migrated workspace schema: %w", err)
		}
	}

	oldWorkspaces, err := tx.Query(`SELECT id, root_path, created_at, updated_at FROM workspaces_old ORDER BY created_at, id`)
	if err != nil {
		return fmt.Errorf("failed to read old workspaces: %w", err)
	}
	defer oldWorkspaces.Close()

	idMap := make(map[string]int64)
	for oldWorkspaces.Next() {
		var oldID, rootPath, createdAt, updatedAt string
		if err := oldWorkspaces.Scan(&oldID, &rootPath, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("failed to scan old workspace: %w", err)
		}
		result, err := tx.Exec(`
			INSERT INTO workspaces (name, path, created_at, updated_at)
			VALUES (?, ?, ?, ?)
		`, oldID, rootPath, createdAt, updatedAt)
		if err != nil {
			return fmt.Errorf("failed to migrate workspace %s: %w", oldID, err)
		}
		newID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to read migrated workspace id: %w", err)
		}
		idMap[oldID] = newID
	}
	if err := oldWorkspaces.Err(); err != nil {
		return fmt.Errorf("failed to iterate old workspaces: %w", err)
	}

	if err := copyWorkspaceRows(tx, "targets", []string{
		"id", "workspace_id", "name", "ip", "os_name", "os_accuracy", "os_source", "is_primary", "created_at", "updated_at",
	}, func(rows *sql.Rows) ([]any, error) {
		var (
			id                                          int64
			workspaceID, name, ip, createdAt, updatedAt string
			osName, osSource                            sql.NullString
			osAccuracy, isPrimary                       sql.NullInt64
		)
		if err := rows.Scan(&id, &workspaceID, &name, &ip, &osName, &osAccuracy, &osSource, &isPrimary, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		newWorkspaceID, ok := idMap[workspaceID]
		if !ok {
			return nil, fmt.Errorf("missing workspace mapping for %s", workspaceID)
		}
		return []any{id, newWorkspaceID, name, ip, nullableStringValueAny(osName), nullableInt64Value(osAccuracy), nullableStringValueAny(osSource), nullableInt64Value(isPrimary), createdAt, updatedAt}, nil
	}); err != nil {
		return err
	}

	if err := copyWorkspaceRows(tx, "command_logs", []string{
		"id", "workspace_id", "command", "expanded_command", "status", "exit_code", "stdout", "stderr", "started_at", "ended_at", "created_at", "updated_at",
	}, func(rows *sql.Rows) ([]any, error) {
		var (
			id                                                       int64
			workspaceID, command, expandedCommand, status, startedAt string
			exitCode                                                 sql.NullInt64
			stdout, stderr, endedAt, createdAt, updatedAt            sql.NullString
		)
		if err := rows.Scan(&id, &workspaceID, &command, &expandedCommand, &status, &exitCode, &stdout, &stderr, &startedAt, &endedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		newWorkspaceID, ok := idMap[workspaceID]
		if !ok {
			return nil, fmt.Errorf("missing workspace mapping for %s", workspaceID)
		}
		return []any{id, newWorkspaceID, command, expandedCommand, status, nullableInt64Value(exitCode), nullableStringValueAny(stdout), nullableStringValueAny(stderr), startedAt, nullableStringValueAny(endedAt), nullableStringValueAny(createdAt), nullableStringValueAny(updatedAt)}, nil
	}); err != nil {
		return err
	}

	if err := copyWorkspaceRows(tx, "hosts", []string{
		"id", "target_id", "hostname", "source", "created_at", "updated_at",
	}, func(rows *sql.Rows) ([]any, error) {
		var (
			id                             int64
			targetID                       int64
			hostname, createdAt, updatedAt string
			source                         sql.NullString
		)
		if err := rows.Scan(&id, &targetID, &hostname, &source, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		return []any{id, targetID, hostname, nullableStringValueAny(source), createdAt, updatedAt}, nil
	}); err != nil {
		return err
	}

	if err := copyWorkspaceRows(tx, "services", []string{
		"id", "target_id", "port", "protocol", "state", "reason", "service_name", "product", "version", "extrainfo", "tunnel", "cpe", "last_seen", "created_at", "updated_at",
	}, func(rows *sql.Rows) ([]any, error) {
		var (
			id, targetID, port                                      int64
			protocol, createdAt, updatedAt                          string
			state, reason, serviceName, product, version, extrainfo sql.NullString
			tunnel, cpe, lastSeen                                   sql.NullString
		)
		if err := rows.Scan(&id, &targetID, &port, &protocol, &state, &reason, &serviceName, &product, &version, &extrainfo, &tunnel, &cpe, &lastSeen, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		return []any{
			id, targetID, port, protocol,
			nullableStringValueAny(state),
			nullableStringValueAny(reason),
			nullableStringValueAny(serviceName),
			nullableStringValueAny(product),
			nullableStringValueAny(version),
			nullableStringValueAny(extrainfo),
			nullableStringValueAny(tunnel),
			nullableStringValueAny(cpe),
			nullableStringValueAny(lastSeen),
			createdAt,
			updatedAt,
		}, nil
	}); err != nil {
		return err
	}

	if err := copyWorkspaceRows(tx, "credentials", []string{
		"id", "target_id", "scope", "username", "password", "verified", "evidence_log_id", "created_at", "updated_at",
	}, func(rows *sql.Rows) ([]any, error) {
		var (
			id, targetID                          int64
			scope, username, createdAt, updatedAt string
			password                              sql.NullString
			verified, evidenceLogID               sql.NullInt64
		)
		if err := rows.Scan(&id, &targetID, &scope, &username, &password, &verified, &evidenceLogID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		return []any{id, targetID, scope, username, nullableStringValueAny(password), nullableInt64Value(verified), nullableInt64Value(evidenceLogID), createdAt, updatedAt}, nil
	}); err != nil {
		return err
	}

	oldScanRunsHasWorkspaceID, err := txTableColumnExists(tx, "scan_runs_old", "workspace_id")
	if err != nil {
		return err
	}
	if oldScanRunsHasWorkspaceID {
		if err := copyWorkspaceRows(tx, "scan_runs", []string{
			"id", "workspace_id", "target_id", "target_ip", "ports", "command_log_id", "created_at", "updated_at",
		}, func(rows *sql.Rows) ([]any, error) {
			var (
				id, targetID, commandLogID                         int64
				workspaceID, targetIP, ports, createdAt, updatedAt string
			)
			if err := rows.Scan(&id, &workspaceID, &targetID, &targetIP, &ports, &commandLogID, &createdAt, &updatedAt); err != nil {
				return nil, err
			}
			if _, ok := idMap[workspaceID]; !ok {
				return nil, fmt.Errorf("missing workspace mapping for %s", workspaceID)
			}
			return []any{id, targetID, targetIP, ports, commandLogID, createdAt, updatedAt}, nil
		}); err != nil {
			return err
		}
	} else {
		if err := copyWorkspaceRows(tx, "scan_runs", []string{
			"id", "target_id", "target_ip", "ports", "command_log_id", "created_at", "updated_at",
		}, func(rows *sql.Rows) ([]any, error) {
			var (
				id, targetID, commandLogID            int64
				targetIP, ports, createdAt, updatedAt string
			)
			if err := rows.Scan(&id, &targetID, &targetIP, &ports, &commandLogID, &createdAt, &updatedAt); err != nil {
				return nil, err
			}
			return []any{id, targetID, targetIP, ports, commandLogID, createdAt, updatedAt}, nil
		}); err != nil {
			return err
		}
	}

	if err := copyWorkspaceRows(tx, "notes", []string{
		"id", "workspace_id", "body", "created_at", "updated_at",
	}, func(rows *sql.Rows) ([]any, error) {
		var id int64
		var workspaceID, body, createdAt, updatedAt string
		if err := rows.Scan(&id, &workspaceID, &body, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		newWorkspaceID, ok := idMap[workspaceID]
		if !ok {
			return nil, fmt.Errorf("missing workspace mapping for %s", workspaceID)
		}
		return []any{id, newWorkspaceID, body, createdAt, updatedAt}, nil
	}); err != nil {
		return err
	}

	for _, table := range []string{"workspaces_old", "targets_old", "command_logs_old", "notes_old", "scan_runs_old", "hosts_old", "services_old", "credentials_old"} {
		if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
			return fmt.Errorf("failed to drop %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit workspace migration: %w", err)
	}
	return nil
}

func workspaceSchemaNeedsMigration(db *sql.DB) (bool, error) {
	exists, err := tableExists(db, "workspaces")
	if err != nil || !exists {
		return err != nil, err
	}
	hasName, err := tableColumnExists(db, "workspaces", "name")
	if err != nil {
		return false, err
	}
	if !hasName {
		return true, nil
	}
	return false, nil
}

func txTableColumnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("failed to inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("failed to read %s column: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("failed to inspect %s columns: %w", table, err)
	}
	return false, nil
}

func copyWorkspaceRows(tx *sql.Tx, table string, columns []string, build func(*sql.Rows) ([]any, error)) error {
	oldTable := table + "_old"
	rows, err := tx.Query(`SELECT ` + strings.Join(columns, ", ") + ` FROM ` + oldTable)
	if err != nil {
		return fmt.Errorf("failed to read old %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		values, err := build(rows)
		if err != nil {
			return fmt.Errorf("failed to migrate %s row: %w", table, err)
		}
		if len(values) == 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO `+table+` VALUES (`+placeholders(len(values))+`)`, values...); err != nil {
			return fmt.Errorf("failed to insert migrated %s row: %w", table, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate old %s rows: %w", table, err)
	}
	return nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func nullableInt64Value(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableStringValueAny(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
