package ctx

import (
	"database/sql"
	"fmt"
	"strconv"
)

type CommandLog struct {
	ID              int64
	WorkspaceID     string
	Command         string
	ExpandedCommand string
	Status          string
	ExitCode        int
	Stdout          string
	Stderr          string
	StartedAt       string
	EndedAt         string
}

func SaveCommandLog(workspace *Workspace, log CommandLog) (int64, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	result, err := db.Exec(`
		INSERT INTO command_log (
			workspace_id, command, expanded_command, status, exit_code,
			stdout, stderr, started_at, ended_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, workspace.ID, log.Command, log.ExpandedCommand, log.Status, log.ExitCode, log.Stdout, log.Stderr, log.StartedAt, log.EndedAt)
	if err != nil {
		return 0, fmt.Errorf("failed to save command log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to inspect command log id: %w", err)
	}
	return id, nil
}

func ListCommandLogs(workspace *Workspace) ([]CommandLog, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, workspace_id, command, expanded_command, status, COALESCE(exit_code, 0),
			COALESCE(stdout, ''), COALESCE(stderr, ''), started_at, ended_at
		FROM command_log
		WHERE workspace_id = ?
		ORDER BY started_at ASC, id ASC
	`, workspace.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list command logs: %w", err)
	}
	defer rows.Close()

	var logs []CommandLog
	for rows.Next() {
		var log CommandLog
		if err := rows.Scan(&log.ID, &log.WorkspaceID, &log.Command, &log.ExpandedCommand, &log.Status, &log.ExitCode, &log.Stdout, &log.Stderr, &log.StartedAt, &log.EndedAt); err != nil {
			return nil, fmt.Errorf("failed to read command log: %w", err)
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list command logs: %w", err)
	}

	return logs, nil
}

func GetCommandLog(workspace *Workspace, rawID string) (*CommandLog, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id < 1 {
		return nil, fmt.Errorf("invalid log id: %s", rawID)
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var log CommandLog
	err = db.QueryRow(`
		SELECT id, workspace_id, command, expanded_command, status, COALESCE(exit_code, 0),
			COALESCE(stdout, ''), COALESCE(stderr, ''), started_at, ended_at
		FROM command_log
		WHERE workspace_id = ? AND id = ?
	`, workspace.ID, id).Scan(&log.ID, &log.WorkspaceID, &log.Command, &log.ExpandedCommand, &log.Status, &log.ExitCode, &log.Stdout, &log.Stderr, &log.StartedAt, &log.EndedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("log not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load command log: %w", err)
	}

	return &log, nil
}
