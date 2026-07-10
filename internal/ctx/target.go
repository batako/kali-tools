package ctx

import (
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
)

type Target struct {
	ID        int64
	Name      string
	IP        string
	IsPrimary bool
}

func SetPrimaryTargetIP(workspace *Workspace, ip string) (*Target, error) {
	if err := validateIP(ip); err != nil {
		return nil, err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	name, err := primaryTargetName(db, workspace.ID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = "default"
	}

	if err := setTargetPrimaryIP(db, workspace.ID, name, ip); err != nil {
		return nil, err
	}

	return GetPrimaryTarget(workspace)
}

func AddTarget(workspace *Workspace, ip, name string) (*Target, error) {
	if err := validateIP(ip); err != nil {
		return nil, err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if name == "" {
		name, err = nextTargetName(db, workspace.ID)
		if err != nil {
			return nil, err
		}
	}

	count, err := targetCount(db, workspace.ID)
	if err != nil {
		return nil, err
	}
	isPrimary := count == 0

	if isPrimary {
		if err := setTargetPrimaryIP(db, workspace.ID, name, ip); err != nil {
			return nil, err
		}
	} else {
		_, err = db.Exec(`
			INSERT INTO targets (workspace_id, name, ip, is_primary, created_at, updated_at)
			VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, workspace.ID, name, ip)
		if err != nil {
			return nil, fmt.Errorf("failed to add target: %w", err)
		}
	}

	return GetTargetByName(workspace, name)
}

func UseTarget(workspace *Workspace, name string) (*Target, error) {
	if name == "" {
		return nil, errors.New("target name is required")
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start target update: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE targets SET is_primary = 0, updated_at = CURRENT_TIMESTAMP WHERE workspace_id = ?`, workspace.ID); err != nil {
		return nil, fmt.Errorf("failed to update targets: %w", err)
	}
	result, err := tx.Exec(`UPDATE targets SET is_primary = 1, updated_at = CURRENT_TIMESTAMP WHERE workspace_id = ? AND name = ?`, workspace.ID, name)
	if err != nil {
		return nil, fmt.Errorf("failed to use target: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect target update: %w", err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("target not found: %s", name)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit target update: %w", err)
	}

	return GetTargetByName(workspace, name)
}

func RemoveTarget(workspace *Workspace, name string) error {
	if name == "" {
		return errors.New("target name is required")
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return err
	}
	defer db.Close()

	var exists int
	err = db.QueryRow(`SELECT 1 FROM targets WHERE workspace_id = ? AND name = ?`, workspace.ID, name).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("target not found: %s", name)
	}
	if err != nil {
		return fmt.Errorf("failed to load target: %w", err)
	}

	result, err := db.Exec(`DELETE FROM targets WHERE workspace_id = ? AND name = ?`, workspace.ID, name)
	if err != nil {
		return fmt.Errorf("failed to remove target: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect target removal: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("target not found: %s", name)
	}

	return nil
}

func ListTargets(workspace *Workspace) ([]Target, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, name, ip, is_primary
		FROM targets
		WHERE workspace_id = ?
		ORDER BY id
	`, workspace.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}
	defer rows.Close()

	var targets []Target
	for rows.Next() {
		var target Target
		var isPrimary int
		if err := rows.Scan(&target.ID, &target.Name, &target.IP, &isPrimary); err != nil {
			return nil, fmt.Errorf("failed to read target: %w", err)
		}
		target.IsPrimary = isPrimary == 1
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	return targets, nil
}

func GetPrimaryTarget(workspace *Workspace) (*Target, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var target Target
	var isPrimary int
	err = db.QueryRow(`
		SELECT id, name, ip, is_primary
		FROM targets
		WHERE workspace_id = ? AND is_primary = 1
		ORDER BY id
		LIMIT 1
	`, workspace.ID).Scan(&target.ID, &target.Name, &target.IP, &isPrimary)
	if err == sql.ErrNoRows {
		return nil, errors.New("primary target not set")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load primary target: %w", err)
	}
	target.IsPrimary = isPrimary == 1

	return &target, nil
}

func GetTargetByName(workspace *Workspace, name string) (*Target, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var target Target
	var isPrimary int
	err = db.QueryRow(`
		SELECT id, name, ip, is_primary
		FROM targets
		WHERE workspace_id = ? AND name = ?
	`, workspace.ID, name).Scan(&target.ID, &target.Name, &target.IP, &isPrimary)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("target not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load target: %w", err)
	}
	target.IsPrimary = isPrimary == 1

	return &target, nil
}

func GetTargetByIP(workspace *Workspace, ip string) (*Target, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var target Target
	var isPrimary int
	err = db.QueryRow(`
		SELECT id, name, ip, is_primary
		FROM targets
		WHERE workspace_id = ? AND ip = ?
		ORDER BY is_primary DESC, id ASC
		LIMIT 1
	`, workspace.ID, ip).Scan(&target.ID, &target.Name, &target.IP, &isPrimary)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("target not found for IP: %s", ip)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load target by IP: %w", err)
	}
	target.IsPrimary = isPrimary == 1
	return &target, nil
}

func openWorkspaceDatabase(workspace *Workspace) (*sql.DB, error) {
	db, err := openDatabase(workspace.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkspaceDatabase(db, workspace); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func setTargetPrimaryIP(db *sql.DB, workspaceID, name, ip string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start target update: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE targets SET is_primary = 0, updated_at = CURRENT_TIMESTAMP WHERE workspace_id = ?`, workspaceID); err != nil {
		return fmt.Errorf("failed to update targets: %w", err)
	}
	_, err = tx.Exec(`
		INSERT INTO targets (workspace_id, name, ip, is_primary, created_at, updated_at)
		VALUES (?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(workspace_id, name) DO UPDATE SET
			ip = excluded.ip,
			is_primary = 1,
			updated_at = CURRENT_TIMESTAMP
	`, workspaceID, name, ip)
	if err != nil {
		return fmt.Errorf("failed to set target: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit target update: %w", err)
	}

	return nil
}

func primaryTargetName(db *sql.DB, workspaceID string) (string, error) {
	var name string
	err := db.QueryRow(`SELECT name FROM targets WHERE workspace_id = ? AND is_primary = 1 ORDER BY id LIMIT 1`, workspaceID).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to load primary target: %w", err)
	}
	return name, nil
}

func nextTargetName(db *sql.DB, workspaceID string) (string, error) {
	existing := map[string]struct{}{}
	rows, err := db.Query(`SELECT name FROM targets WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return "", fmt.Errorf("failed to load target names: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", fmt.Errorf("failed to read target name: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("failed to load target names: %w", err)
	}

	if _, ok := existing["default"]; !ok {
		return "default", nil
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("target%d", i)
		if _, ok := existing[name]; !ok {
			return name, nil
		}
	}
}

func targetCount(db *sql.DB, workspaceID string) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM targets WHERE workspace_id = ?`, workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count targets: %w", err)
	}
	return count, nil
}

func validateIP(ip string) error {
	if _, err := netip.ParseAddr(ip); err != nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	return nil
}
