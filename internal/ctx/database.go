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
	currentSchemaVersion     = 1
	v100SchemaVersion        = 1
	latestSchemaSnapshotPath = "schema.sql"
)

//go:embed schema.sql migrations/*.sql
var embeddedMigrations embed.FS

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

	if err := validateV100Schema(db); err != nil {
		return fmt.Errorf("database schema is not managed by migrations and is not a recognized ctx v1.0.0 schema: %w", err)
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
