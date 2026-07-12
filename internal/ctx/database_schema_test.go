package ctx

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestDatabaseSchemaConstraints(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	defer db.Close()
	if err := createSchema(db); err != nil {
		t.Fatalf("createSchema() error = %v", err)
	}

	assertTableExists(t, db, "finding", false)
	for _, table := range []string{"workspaces", "targets", "hosts", "services", "credentials", "command_logs", "scan_runs", "notes"} {
		assertTableExists(t, db, table, true)
	}

	removedColumns := map[string][]string{
		"hosts":       {"workspace_id"},
		"services":    {"workspace_id", "hostname", "scripts_json"},
		"credentials": {"workspace_id", "service_id", "hash", "source"},
		"scan_runs":   {"workspace_id"},
		"notes":       {"command_log_id"},
	}
	for table, columns := range removedColumns {
		for _, column := range columns {
			if columnExists(t, db, table, column) {
				t.Fatalf("%s.%s exists, want removed", table, column)
			}
		}
	}

	if _, err := db.Exec(`INSERT INTO workspaces (id, name, path) VALUES (1, 'workspace-1', '/tmp/workspace-1')`); err != nil {
		t.Fatalf("insert workspace error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, path) VALUES (2, 'workspace-2', '/tmp/workspace-1')`); err == nil {
		t.Fatal("duplicate workspace root insert succeeded, want unique constraint error")
	}
	if _, err := db.Exec(`INSERT INTO targets (id, workspace_id, name, ip, is_primary) VALUES (1, 1, 'default', '10.10.10.10', 1)`); err != nil {
		t.Fatalf("insert primary target error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO targets (workspace_id, name, ip, is_primary) VALUES (1, 'target2', '10.10.10.20', 1)`); err == nil {
		t.Fatal("second primary target insert succeeded, want unique constraint error")
	}
	if _, err := db.Exec(`INSERT INTO services (target_id, port, protocol) VALUES (1, 0, 'tcp')`); err == nil {
		t.Fatal("invalid service port insert succeeded, want check constraint error")
	}
	if _, err := db.Exec(`INSERT INTO services (target_id, port, protocol) VALUES (1, 80, 'tcp')`); err != nil {
		t.Fatalf("insert service error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO services (target_id, port, protocol) VALUES (1, 80, 'tcp')`); err == nil {
		t.Fatal("duplicate service insert succeeded, want unique constraint error")
	}
	if _, err := db.Exec(`INSERT INTO command_logs (id, workspace_id, command, expanded_command, status, started_at) VALUES (1, 1, 'hydra', 'hydra', 'success', '2026-07-09T00:00:00Z')`); err != nil {
		t.Fatalf("insert command log error = %v", err)
	}
	if !columnExists(t, db, "credentials", "scope") {
		t.Fatal("credentials.scope missing")
	}
	if _, err := db.Exec(`INSERT INTO credentials (target_id, scope, username, password, evidence_log_id) VALUES (1, 'ssh', 'admin', NULL, 1)`); err != nil {
		t.Fatalf("insert username-only credential error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO credentials (target_id, scope, username, password) VALUES (1, 'ssh', 'admin', 'secret')`); err == nil {
		t.Fatal("duplicate credential insert succeeded, want unique constraint error")
	}
	if _, err := db.Exec(`INSERT INTO credentials (target_id, scope, username, password) VALUES (1, 'wordpress', 'admin', 'secret')`); err != nil {
		t.Fatalf("insert same username in different scope error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO scan_runs (target_id, target_ip, ports, command_log_id) VALUES (1, '10.10.10.10', '80', 1)`); err != nil {
		t.Fatalf("insert scan run error = %v", err)
	}
	if _, err := db.Exec(`DELETE FROM command_logs WHERE id = 1`); err != nil {
		t.Fatalf("delete command log error = %v", err)
	}
	var evidenceLogID sql.NullInt64
	if err := db.QueryRow(`SELECT evidence_log_id FROM credentials WHERE target_id = 1 AND scope = 'ssh' AND username = 'admin'`).Scan(&evidenceLogID); err != nil {
		t.Fatalf("load credential evidence error = %v", err)
	}
	if evidenceLogID.Valid {
		t.Fatalf("credential evidence_log_id = %d, want NULL after command log deletion", evidenceLogID.Int64)
	}
	assertTableCount(t, db, "scan_runs", 0)

	if _, err := db.Exec(`INSERT INTO hosts (target_id, hostname) VALUES (1, 'example.thm')`); err != nil {
		t.Fatalf("insert host error = %v", err)
	}
	if _, err := db.Exec(`DELETE FROM targets WHERE id = 1`); err != nil {
		t.Fatalf("delete target error = %v", err)
	}
	for _, table := range []string{"hosts", "services", "credentials"} {
		assertTableCount(t, db, table, 0)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string, want bool) {
	t.Helper()
	var exists int
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?
		)
	`, table).Scan(&exists); err != nil {
		t.Fatalf("inspect table %s error = %v", table, err)
	}
	if (exists == 1) != want {
		t.Fatalf("table %s exists = %t, want %t", table, exists == 1, want)
	}
}

func assertTableCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s error = %v", table, err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s) error = %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan column for %s error = %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read columns for %s error = %v", table, err)
	}
	return false
}
