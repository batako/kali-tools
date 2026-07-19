package ctx

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

var ErrCommandLogNotFound = errors.New("command log not found")

type CommandLog struct {
	ID              int64
	WorkspaceID     int64
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
		INSERT INTO command_logs (
			workspace_id, command, expanded_command, status, exit_code,
			stdout, stderr, started_at, ended_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, workspace.ID, log.Command, log.ExpandedCommand, log.Status, nullableExitCode(log.Status, log.ExitCode), log.Stdout, log.Stderr, log.StartedAt, nullableEndedAt(log.EndedAt))
	if err != nil {
		return 0, fmt.Errorf("failed to save command log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to inspect command log id: %w", err)
	}
	return id, nil
}

func StartCommandLog(workspace *Workspace, log CommandLog) (int64, error) {
	log.Status = "running"
	return SaveCommandLog(workspace, log)
}

func FinishCommandLog(workspace *Workspace, id int64, log CommandLog) error {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return err
	}
	defer db.Close()

	result, err := db.Exec(`
		UPDATE command_logs
		SET status = ?,
		    exit_code = ?,
		    stdout = ?,
		    stderr = ?,
		    ended_at = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE workspace_id = ? AND id = ?
	`, log.Status, nullableExitCode(log.Status, log.ExitCode), log.Stdout, log.Stderr, nullableEndedAt(log.EndedAt), workspace.ID, id)
	if err != nil {
		return fmt.Errorf("failed to update command log: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect command log update: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: %d", ErrCommandLogNotFound, id)
	}
	return nil
}

func ListCommandLogs(workspace *Workspace) ([]CommandLog, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, workspace_id, command, expanded_command, status, COALESCE(exit_code, 0),
			COALESCE(stdout, ''), COALESCE(stderr, ''), started_at, COALESCE(ended_at, '')
		FROM command_logs
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
			COALESCE(stdout, ''), COALESCE(stderr, ''), started_at, COALESCE(ended_at, '')
		FROM command_logs
		WHERE workspace_id = ? AND id = ?
	`, workspace.ID, id).Scan(&log.ID, &log.WorkspaceID, &log.Command, &log.ExpandedCommand, &log.Status, &log.ExitCode, &log.Stdout, &log.Stderr, &log.StartedAt, &log.EndedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %d", ErrCommandLogNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load command log: %w", err)
	}

	return &log, nil
}

func commandLogStatus(exitCode int) (string, int) {
	switch {
	case exitCode == 0:
		return "success", 0
	case exitCode < 0:
		return "interrupted", exitCode
	default:
		return "failed", exitCode
	}
}

func nullableExitCode(status string, exitCode int) any {
	if status == "running" || status == "interrupted" && exitCode < 0 {
		return nil
	}
	return exitCode
}

func nullableEndedAt(value string) any {
	if value == "" {
		return nil
	}
	return value
}
