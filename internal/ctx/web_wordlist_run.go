package ctx

import (
	"database/sql"
	"fmt"
)

type WebWordlistRun struct {
	ID                int64
	TargetID          int64
	URL               string
	Provider          string
	Profile           string
	SearchSignature   string
	Wordlist          string
	Status            string
	CommandLogID      int64
	CommandLogIDValid bool
	StartedAt         string
	EndedAt           string
}

func ListWebWordlistRuns(workspace *Workspace, target *Target, url string) ([]WebWordlistRun, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, target_id, url, provider, profile, search_signature, wordlist, status, command_log_id,
		       started_at, COALESCE(ended_at, '')
		FROM web_wordlist_runs WHERE target_id = ? AND url = ? ORDER BY id ASC
	`, target.ID, url)
	if err != nil {
		return nil, fmt.Errorf("failed to list web wordlist runs: %w", err)
	}
	defer rows.Close()
	var runs []WebWordlistRun
	for rows.Next() {
		var run WebWordlistRun
		var logID sql.NullInt64
		if err := rows.Scan(&run.ID, &run.TargetID, &run.URL, &run.Provider, &run.Profile, &run.SearchSignature, &run.Wordlist, &run.Status, &logID, &run.StartedAt, &run.EndedAt); err != nil {
			return nil, fmt.Errorf("failed to read web wordlist run: %w", err)
		}
		if logID.Valid {
			run.CommandLogID, run.CommandLogIDValid = logID.Int64, true
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func StartWebWordlistRun(workspace *Workspace, target *Target, url, provider, profile, searchSignature, wordlist, startedAt string, commandLogID int64) (int64, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var id int64
	err = db.QueryRow(`
		INSERT INTO web_wordlist_runs (target_id, url, provider, profile, search_signature, wordlist, status, command_log_id, started_at)
		VALUES (?, ?, ?, ?, ?, ?, 'running', ?, ?)
		ON CONFLICT(target_id, url, profile, search_signature, wordlist) DO UPDATE SET
			provider = excluded.provider, status = 'running', command_log_id = excluded.command_log_id,
			started_at = excluded.started_at, ended_at = NULL, updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, target.ID, url, provider, profile, searchSignature, wordlist, nullableWebInt64(commandLogID > 0, commandLogID), startedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to start web wordlist run: %w", err)
	}
	return id, nil
}

func FinishWebWordlistRun(workspace *Workspace, id int64, status string, endedAt string) error {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.Exec(`UPDATE web_wordlist_runs SET status = ?, ended_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, endedAt, id)
	if err != nil {
		return fmt.Errorf("failed to finish web wordlist run: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("web wordlist run not found: %d", id)
	}
	return nil
}
