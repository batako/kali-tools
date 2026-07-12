package ctx

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCreateSchemaInitializesNewDatabaseWithCurrentVersion(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		t.Fatalf("createSchema() error = %v", err)
	}

	assertTableExists(t, db, "workspaces", true)
	assertSchemaMigrationVersion(t, db, currentSchemaVersion, false)
}

func TestCreateSchemaBaselinesCtxV100Database(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	createCtxV100Schema(t, db)
	if _, err := db.Exec(`INSERT INTO workspaces (id, root_path) VALUES ('workspace-1', '/tmp/workspace-1')`); err != nil {
		t.Fatalf("insert v1.0.0 workspace error = %v", err)
	}

	if err := createSchema(db); err != nil {
		t.Fatalf("createSchema() error = %v", err)
	}

	assertSchemaMigrationVersion(t, db, currentSchemaVersion, false)
	var name, path string
	if err := db.QueryRow(`SELECT name, path FROM workspaces WHERE name = 'workspace-1'`).Scan(&name, &path); err != nil {
		t.Fatalf("load v1.0.0 workspace after baseline error = %v", err)
	}
	if name != "workspace-1" || path != "/tmp/workspace-1" {
		t.Fatalf("workspace = %q %q, want workspace-1 /tmp/workspace-1", name, path)
	}
}

func TestWorkspaceMigrationFromCtxV100Schema(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	createCtxV100Schema(t, db)
	insertCtxV100WorkspaceData(t, db, root)

	if err := createSchema(db); err != nil {
		t.Fatalf("createSchema() migration error = %v", err)
	}

	assertSchemaMigrationVersion(t, db, currentSchemaVersion, false)
	assertColumnExists(t, db, "workspaces", "name", true)
	assertColumnExists(t, db, "workspaces", "path", true)
	assertColumnExists(t, db, "workspaces", "root_path", false)

	var workspaceID int64
	var workspaceName, workspacePath string
	if err := db.QueryRow(`SELECT id, name, path FROM workspaces`).Scan(&workspaceID, &workspaceName, &workspacePath); err != nil {
		t.Fatalf("load migrated workspace error = %v", err)
	}
	if workspaceID != 1 || workspaceName != "workspace-uuid-1" || workspacePath != root {
		t.Fatalf("migrated workspace = %d %q %q", workspaceID, workspaceName, workspacePath)
	}

	assertSingleInt64Value(t, db, `SELECT workspace_id FROM targets WHERE id = 1`, 1)
	assertSingleInt64Value(t, db, `SELECT workspace_id FROM command_logs WHERE id = 1`, 1)
	assertSingleInt64Value(t, db, `SELECT workspace_id FROM notes WHERE id = 1`, 1)
	assertSingleInt64Value(t, db, `SELECT command_log_id FROM scan_runs WHERE id = 1`, 1)
	assertSingleInt64Value(t, db, `SELECT target_id FROM hosts WHERE id = 1`, 1)
	assertSingleInt64Value(t, db, `SELECT target_id FROM services WHERE id = 1`, 1)
	assertSingleInt64Value(t, db, `SELECT target_id FROM credentials WHERE id = 1`, 1)
	assertForeignKeyCheckClean(t, db)

	record, err := GetWorkspaceRecordByRoot(root)
	if err != nil {
		t.Fatalf("GetWorkspaceRecordByRoot() error = %v", err)
	}
	if record == nil || record.ID != 1 || record.UUID != "workspace-uuid-1" || record.RootPath != root {
		t.Fatalf("workspace record = %+v", record)
	}
}

func TestCreateSchemaRejectsUnverifiedLegacyDatabase(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	createCtxV100Schema(t, db)
	if _, err := db.Exec(`DROP INDEX idx_targets_workspace_id`); err != nil {
		t.Fatalf("drop required index error = %v", err)
	}

	if err := createSchema(db); err == nil {
		t.Fatal("createSchema() error = nil, want unverified legacy database error")
	}
	assertTableExists(t, db, "schema_migrations", false)
}

func TestCreateSchemaNoopsWhenCurrent(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		t.Fatalf("first createSchema() error = %v", err)
	}
	if err := createSchema(db); err != nil {
		t.Fatalf("second createSchema() error = %v", err)
	}

	assertSchemaMigrationVersion(t, db, currentSchemaVersion, false)
}

func TestCreateSchemaBaselinesCurrentSchemaWithoutMigrationTable(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	schema, err := embeddedMigrations.ReadFile(latestSchemaSnapshotPath)
	if err != nil {
		t.Fatalf("read schema.sql error = %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("create current schema without migrations error = %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO workspaces (name, path) VALUES ('workspace-1', '/tmp/workspace-1')
	`); err != nil {
		t.Fatalf("insert current workspace error = %v", err)
	}

	if err := createSchema(db); err != nil {
		t.Fatalf("createSchema() error = %v", err)
	}

	assertSchemaMigrationVersion(t, db, currentSchemaVersion, false)
	var workspaceID int64
	if err := db.QueryRow(`SELECT id FROM workspaces WHERE name = 'workspace-1' AND path = '/tmp/workspace-1'`).Scan(&workspaceID); err != nil {
		t.Fatalf("load current workspace after baseline error = %v", err)
	}
	if workspaceID != 1 {
		t.Fatalf("workspace id = %d, want 1", workspaceID)
	}
}

func TestLatestSchemaSnapshotMatchesMigratedV100Schema(t *testing.T) {
	snapshotDB := openTestDatabase(t)
	defer snapshotDB.Close()
	if err := createLatestSchema(snapshotDB, embeddedMigrations, latestSchemaSnapshotPath, currentSchemaVersion); err != nil {
		t.Fatalf("createLatestSchema() error = %v", err)
	}

	migratedDB := openTestDatabase(t)
	defer migratedDB.Close()
	createCtxV100Schema(t, migratedDB)
	if err := baselineLegacyDatabase(migratedDB, embeddedMigrations, "migrations"); err != nil {
		t.Fatalf("baselineLegacyDatabase() error = %v", err)
	}
	if err := applyPendingMigrations(migratedDB, embeddedMigrations, "migrations"); err != nil {
		t.Fatalf("applyPendingMigrations() error = %v", err)
	}

	snapshotSchema := loadDatabaseSchemaSnapshot(t, snapshotDB)
	migratedSchema := loadDatabaseSchemaSnapshot(t, migratedDB)
	if !reflect.DeepEqual(snapshotSchema, migratedSchema) {
		t.Fatalf("schema.sql schema and migrated v1.0.0 schema differ\nschema.sql: %#v\nmigrated: %#v", snapshotSchema, migratedSchema)
	}
}

func TestMigrationV120FixtureMigratesToLatest(t *testing.T) {
	assertMigrationFixtureMigratesToLatest(t, migrationDatabaseFixture{
		Version:       "v1.2.0",
		WorkspacePath: "/tmp/ctx-released-v1.0.0-fixture/projects/fixture-project",
		NoteBody:      "fixture note from ctx v1.0.0",
	})
}

type migrationDatabaseFixture struct {
	Version       string
	WorkspacePath string
	NoteBody      string
}

func assertMigrationFixtureMigratesToLatest(t *testing.T, fixture migrationDatabaseFixture) {
	t.Helper()
	fixturePath := filepath.Join("testdata", "database", fixture.Version+".sqlite")
	fixtureBefore, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read migration %s fixture error = %v", fixture.Version, err)
	}

	dbPath := filepath.Join(t.TempDir(), fixture.Version+".sqlite")
	if err := os.WriteFile(dbPath, fixtureBefore, 0644); err != nil {
		t.Fatalf("copy migration %s fixture error = %v", fixture.Version, err)
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		t.Fatalf("open copied migration %s fixture error = %v", fixture.Version, err)
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		t.Fatalf("createSchema() for migration %s fixture error = %v", fixture.Version, err)
	}

	assertSchemaMigrationVersion(t, db, currentSchemaVersion, false)
	if err := validateIntegrity(db); err != nil {
		t.Fatalf("integrity_check after migration %s fixture error = %v", fixture.Version, err)
	}
	assertForeignKeyCheckClean(t, db)
	assertMigrationFixtureDataRetained(t, db, fixture)

	snapshotDB := openTestDatabase(t)
	defer snapshotDB.Close()
	if err := createLatestSchema(snapshotDB, embeddedMigrations, latestSchemaSnapshotPath, currentSchemaVersion); err != nil {
		t.Fatalf("createLatestSchema() error = %v", err)
	}
	snapshotSchema := loadDatabaseSchemaSnapshot(t, snapshotDB)
	migratedSchema := loadDatabaseSchemaSnapshot(t, db)
	if !reflect.DeepEqual(snapshotSchema, migratedSchema) {
		t.Fatalf("schema.sql schema and migration %s schema differ\nschema.sql: %#v\nmigrated: %#v", fixture.Version, snapshotSchema, migratedSchema)
	}

	fixtureAfter, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("reread migration %s fixture error = %v", fixture.Version, err)
	}
	if !bytes.Equal(fixtureBefore, fixtureAfter) {
		t.Fatalf("migration %s fixture was modified during migration test", fixture.Version)
	}
}

func TestApplyPendingMigrationsUpdatesExistingDatabase(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	fsys := fstest.MapFS{
		"migrations/000001_1.0.0.up.sql":   {Data: []byte(`CREATE TABLE base_table (id INTEGER PRIMARY KEY);`)},
		"migrations/000001_1.0.0.down.sql": {Data: []byte(`DROP TABLE IF EXISTS base_table;`)},
		"migrations/000002_1.2.0.up.sql":   {Data: []byte(`CREATE TABLE future_table (id INTEGER PRIMARY KEY);`)},
		"migrations/000002_1.2.0.down.sql": {Data: []byte(`DROP TABLE IF EXISTS future_table;`)},
	}

	if err := createLatestSchema(db, fsys, "migrations/000001_1.0.0.up.sql", 1); err != nil {
		t.Fatalf("createLatestSchema() error = %v", err)
	}
	if err := applyPendingMigrations(db, fsys, "migrations"); err != nil {
		t.Fatalf("applyPendingMigrations() error = %v", err)
	}

	assertTableExists(t, db, "future_table", true)
	assertSchemaMigrationVersion(t, db, 2, false)
}

func TestApplyPendingMigrationsRollsBackFailedMigrationBody(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()

	fsys := fstest.MapFS{
		"migrations/000001_1.0.0.up.sql":   {Data: []byte(`CREATE TABLE base_table (id INTEGER PRIMARY KEY);`)},
		"migrations/000001_1.0.0.down.sql": {Data: []byte(`DROP TABLE IF EXISTS base_table;`)},
		"migrations/000002_1.2.0.up.sql":   {Data: []byte(`CREATE TABLE should_rollback (id INTEGER PRIMARY KEY); CREATE TABLE broken (`)},
		"migrations/000002_1.2.0.down.sql": {Data: []byte(`DROP TABLE IF EXISTS should_rollback;`)},
	}

	if err := createLatestSchema(db, fsys, "migrations/000001_1.0.0.up.sql", 1); err != nil {
		t.Fatalf("createLatestSchema() error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO base_table (id) VALUES (42)`); err != nil {
		t.Fatalf("insert base data error = %v", err)
	}
	err = applyPendingMigrations(db, fsys, "migrations")
	if err == nil {
		t.Fatal("applyPendingMigrations() error = nil, want error")
	}

	assertSingleInt64Value(t, db, `SELECT id FROM base_table`, 42)
	assertTableExists(t, db, "should_rollback", false)
	assertSchemaMigrationVersion(t, db, 2, true)
	if err := applyPendingMigrations(db, fsys, "migrations"); err == nil {
		t.Fatal("applyPendingMigrations() after dirty state error = nil, want error")
	}
	assertSchemaMigrationVersion(t, db, 2, true)
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openDatabase(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	return db
}

func createCtxV100Schema(t *testing.T, db *sql.DB) {
	t.Helper()
	schema, err := fsReadEmbeddedMigration("migrations/000001_1.0.0.up.sql")
	if err != nil {
		t.Fatalf("read v1.0.0 schema error = %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create v1.0.0 schema error = %v", err)
	}
}

func insertCtxV100WorkspaceData(t *testing.T, db *sql.DB, root string) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces (id, root_path, created_at, updated_at) VALUES ('workspace-uuid-1', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, []any{root}},
		{`INSERT INTO targets (id, workspace_id, name, ip, is_primary, created_at, updated_at) VALUES (1, 'workspace-uuid-1', 'default', '10.10.10.10', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, nil},
		{`INSERT INTO command_logs (id, workspace_id, command, expanded_command, status, started_at, created_at, updated_at) VALUES (1, 'workspace-uuid-1', 'nmap', 'nmap', 'success', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, nil},
		{`INSERT INTO hosts (id, target_id, hostname, source, created_at, updated_at) VALUES (1, 1, 'target.example', 'manual', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, nil},
		{`INSERT INTO services (id, target_id, port, protocol, state, service_name, created_at, updated_at) VALUES (1, 1, 22, 'tcp', 'open', 'ssh', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, nil},
		{`INSERT INTO credentials (id, target_id, scope, username, password, verified, evidence_log_id, created_at, updated_at) VALUES (1, 1, 'ssh', 'root', 'toor', 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, nil},
		{`INSERT INTO scan_runs (id, target_id, target_ip, ports, command_log_id, created_at, updated_at) VALUES (1, 1, '10.10.10.10', '22,80', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, nil},
		{`INSERT INTO notes (id, workspace_id, body, created_at, updated_at) VALUES (1, 'workspace-uuid-1', 'note', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, nil},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("insert v1.0.0 data error = %v", err)
		}
	}
}

func fsReadEmbeddedMigration(path string) (string, error) {
	content, err := embeddedMigrations.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func assertSchemaMigrationVersion(t *testing.T, db *sql.DB, wantVersion uint, wantDirty bool) {
	t.Helper()
	var version uint
	var dirty bool
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatalf("load schema_migrations error = %v", err)
	}
	if version != wantVersion || dirty != wantDirty {
		t.Fatalf("schema_migrations = version %d dirty %t, want version %d dirty %t", version, dirty, wantVersion, wantDirty)
	}
}

func assertColumnExists(t *testing.T, db *sql.DB, table, column string, want bool) {
	t.Helper()
	if got := columnExists(t, db, table, column); got != want {
		t.Fatalf("%s.%s exists = %t, want %t", table, column, got, want)
	}
}

func assertSingleInt64Value(t *testing.T, db *sql.DB, query string, want int64) {
	t.Helper()
	var got int64
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q error = %v", query, err)
	}
	if got != want {
		t.Fatalf("query %q = %d, want %d", query, got, want)
	}
}

func assertForeignKeyCheckClean(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check error = %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check returned violations")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign_key_check error = %v", err)
	}
}

func assertMigrationFixtureDataRetained(t *testing.T, db *sql.DB, fixture migrationDatabaseFixture) {
	t.Helper()
	assertSingleStringValue(t, db, `SELECT path FROM workspaces`, fixture.WorkspacePath)
	assertSingleStringValue(t, db, `SELECT ip FROM targets WHERE name = 'default' AND is_primary = 1`, "10.10.10.10")
	assertSingleStringValue(t, db, `SELECT hostname FROM hosts`, "target.example")
	assertSingleStringValue(t, db, `SELECT password FROM credentials WHERE scope = 'ssh' AND username = 'root'`, "toor")
	assertSingleStringValue(t, db, `SELECT body FROM notes`, fixture.NoteBody)
	assertSingleInt64Value(t, db, `SELECT COUNT(*) FROM services`, 2)
	assertSingleInt64Value(t, db, `SELECT COUNT(*) FROM scan_runs`, 1)
	assertSingleInt64Value(t, db, `SELECT COUNT(*) FROM command_logs WHERE command IN ('ctx scan -p 22,80', 'echo fixture-command')`, 2)
	assertSingleInt64Value(t, db, `SELECT COUNT(*) FROM services WHERE port = 22 AND protocol = 'tcp' AND service_name = 'ssh' AND product = 'OpenSSH'`, 1)
	assertSingleInt64Value(t, db, `SELECT COUNT(*) FROM services WHERE port = 80 AND protocol = 'tcp' AND service_name = 'http' AND product = 'nginx'`, 1)
}

func assertSingleStringValue(t *testing.T, db *sql.DB, query string, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q error = %v", query, err)
	}
	if got != want {
		t.Fatalf("query %q = %q, want %q", query, got, want)
	}
}

type schemaSnapshot struct {
	Master      []sqliteMasterEntry
	TableInfo   map[string][]pragmaTableInfo
	IndexList   map[string][]pragmaIndexList
	IndexInfo   map[string][]pragmaIndexInfo
	ForeignKeys map[string][]pragmaForeignKey
}

type sqliteMasterEntry struct {
	Type    string
	Name    string
	Table   string
	SQLText string
}

type pragmaTableInfo struct {
	CID        int
	Name       string
	Type       string
	NotNull    int
	Default    string
	PrimaryKey int
}

type pragmaIndexList struct {
	Seq     int
	Name    string
	Unique  int
	Origin  string
	Partial int
}

type pragmaIndexInfo struct {
	IndexName string
	SeqNo     int
	CID       int
	Name      string
}

type pragmaForeignKey struct {
	ID       int
	Seq      int
	Table    string
	From     string
	To       string
	OnUpdate string
	OnDelete string
	Match    string
}

func loadDatabaseSchemaSnapshot(t *testing.T, db *sql.DB) schemaSnapshot {
	t.Helper()
	return schemaSnapshot{
		Master:      loadSQLiteMaster(t, db),
		TableInfo:   loadAllTableInfo(t, db),
		IndexList:   loadAllIndexLists(t, db),
		IndexInfo:   loadAllIndexInfo(t, db),
		ForeignKeys: loadAllForeignKeys(t, db),
	}
}

func loadSQLiteMaster(t *testing.T, db *sql.DB) []sqliteMasterEntry {
	t.Helper()
	rows, err := db.Query(`
		SELECT type, name, tbl_name, COALESCE(sql, '')
		FROM sqlite_master
		ORDER BY type, name, tbl_name
	`)
	if err != nil {
		t.Fatalf("load sqlite_master error = %v", err)
	}
	defer rows.Close()

	var entries []sqliteMasterEntry
	for rows.Next() {
		var entry sqliteMasterEntry
		if err := rows.Scan(&entry.Type, &entry.Name, &entry.Table, &entry.SQLText); err != nil {
			t.Fatalf("scan sqlite_master error = %v", err)
		}
		entry.SQLText = normalizeSchemaSQL(entry.SQLText)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite_master error = %v", err)
	}
	return entries
}

func loadAllTableInfo(t *testing.T, db *sql.DB) map[string][]pragmaTableInfo {
	t.Helper()
	result := make(map[string][]pragmaTableInfo)
	for _, table := range loadTableNames(t, db) {
		rows, err := db.Query("PRAGMA table_info(" + quoteSQLiteIdentifier(table) + ")")
		if err != nil {
			t.Fatalf("table_info(%s) error = %v", table, err)
		}
		var columns []pragmaTableInfo
		for rows.Next() {
			var column pragmaTableInfo
			var defaultValue sql.NullString
			if err := rows.Scan(&column.CID, &column.Name, &column.Type, &column.NotNull, &defaultValue, &column.PrimaryKey); err != nil {
				rows.Close()
				t.Fatalf("scan table_info(%s) error = %v", table, err)
			}
			if defaultValue.Valid {
				column.Default = normalizeSchemaSQL(defaultValue.String)
			}
			columns = append(columns, column)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close table_info(%s) error = %v", table, err)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate table_info(%s) error = %v", table, err)
		}
		result[table] = columns
	}
	return result
}

func loadAllIndexLists(t *testing.T, db *sql.DB) map[string][]pragmaIndexList {
	t.Helper()
	result := make(map[string][]pragmaIndexList)
	for _, table := range loadTableNames(t, db) {
		rows, err := db.Query("PRAGMA index_list(" + quoteSQLiteIdentifier(table) + ")")
		if err != nil {
			t.Fatalf("index_list(%s) error = %v", table, err)
		}
		var indexes []pragmaIndexList
		for rows.Next() {
			var index pragmaIndexList
			if err := rows.Scan(&index.Seq, &index.Name, &index.Unique, &index.Origin, &index.Partial); err != nil {
				rows.Close()
				t.Fatalf("scan index_list(%s) error = %v", table, err)
			}
			indexes = append(indexes, index)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close index_list(%s) error = %v", table, err)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate index_list(%s) error = %v", table, err)
		}
		result[table] = indexes
	}
	return result
}

func loadAllIndexInfo(t *testing.T, db *sql.DB) map[string][]pragmaIndexInfo {
	t.Helper()
	result := make(map[string][]pragmaIndexInfo)
	for _, index := range loadIndexNames(t, db) {
		rows, err := db.Query("PRAGMA index_info(" + quoteSQLiteIdentifier(index) + ")")
		if err != nil {
			t.Fatalf("index_info(%s) error = %v", index, err)
		}
		var columns []pragmaIndexInfo
		for rows.Next() {
			var column pragmaIndexInfo
			var name sql.NullString
			if err := rows.Scan(&column.SeqNo, &column.CID, &name); err != nil {
				rows.Close()
				t.Fatalf("scan index_info(%s) error = %v", index, err)
			}
			column.IndexName = index
			if name.Valid {
				column.Name = name.String
			}
			columns = append(columns, column)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close index_info(%s) error = %v", index, err)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate index_info(%s) error = %v", index, err)
		}
		result[index] = columns
	}
	return result
}

func loadAllForeignKeys(t *testing.T, db *sql.DB) map[string][]pragmaForeignKey {
	t.Helper()
	result := make(map[string][]pragmaForeignKey)
	for _, table := range loadTableNames(t, db) {
		rows, err := db.Query("PRAGMA foreign_key_list(" + quoteSQLiteIdentifier(table) + ")")
		if err != nil {
			t.Fatalf("foreign_key_list(%s) error = %v", table, err)
		}
		var keys []pragmaForeignKey
		for rows.Next() {
			var key pragmaForeignKey
			if err := rows.Scan(&key.ID, &key.Seq, &key.Table, &key.From, &key.To, &key.OnUpdate, &key.OnDelete, &key.Match); err != nil {
				rows.Close()
				t.Fatalf("scan foreign_key_list(%s) error = %v", table, err)
			}
			keys = append(keys, key)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close foreign_key_list(%s) error = %v", table, err)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate foreign_key_list(%s) error = %v", table, err)
		}
		result[table] = keys
	}
	return result
}

func loadTableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatalf("load table names error = %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name error = %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table names error = %v", err)
	}
	return names
}

func loadIndexNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'index' ORDER BY name`)
	if err != nil {
		t.Fatalf("load index names error = %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name error = %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate index names error = %v", err)
	}
	return names
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func normalizeSchemaSQL(value string) string {
	value = strings.ReplaceAll(value, `"`, "")
	return strings.Join(strings.Fields(value), " ")
}
