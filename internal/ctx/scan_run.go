package ctx

import (
	"database/sql"
	"fmt"
)

func findScanRun(workspace *Workspace, target *Target, targetIP, ports string) (int64, bool, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return 0, false, err
	}
	defer db.Close()

	var logID int64
	err = db.QueryRow(`
		SELECT command_log_id
		FROM scan_runs
		WHERE target_id = ? AND ports = ?
		ORDER BY updated_at DESC, id DESC
		LIMIT 1
	`, target.ID, ports).Scan(&logID)
	switch err {
	case nil:
		return logID, true, nil
	case sql.ErrNoRows:
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("failed to inspect previous scan: %w", err)
	}
}

func saveScanRun(workspace *Workspace, target *Target, targetIP, ports string, logID int64) error {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
		UPDATE scan_runs
		SET target_ip = ?, command_log_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = (
			SELECT id FROM scan_runs
			WHERE target_id = ? AND ports = ?
			ORDER BY updated_at DESC, id DESC
			LIMIT 1
		)
	`, targetIP, logID, target.ID, ports)
	if err != nil {
		return fmt.Errorf("failed to save scan history: %w", err)
	}
	var updated int64
	if err := db.QueryRow(`SELECT changes()`).Scan(&updated); err != nil {
		return fmt.Errorf("failed to inspect scan history update: %w", err)
	}
	if updated == 0 {
		_, err = db.Exec(`
			INSERT INTO scan_runs (
				target_id, target_ip, ports, command_log_id,
				created_at, updated_at
			)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, target.ID, targetIP, ports, logID)
		if err != nil {
			return fmt.Errorf("failed to create scan history: %w", err)
		}
	}
	return nil
}
