package ctx

import (
	"database/sql"
	"fmt"
)

type WebDiscovery struct {
	ID                 int64
	TargetID           int64
	URL                string
	Path               string
	StatusCode         int
	ContentLength      int64
	ContentLengthValid bool
	RedirectURL        string
	RedirectURLValid   bool
	SourceTool         string
	Wordlist           string
	CommandLogID       int64
	CommandLogIDValid  bool
	CreatedAt          string
	UpdatedAt          string
}

func SaveWebDiscovery(workspace *Workspace, target *Target, discovery WebDiscovery) (int64, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	result, err := db.Exec(`
		INSERT INTO web_discoveries (
			target_id, url, path, status_code, content_length, redirect_url,
			source_tool, wordlist, command_log_id,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, target.ID, discovery.URL, discovery.Path, discovery.StatusCode,
		nullableWebInt64(discovery.ContentLengthValid, discovery.ContentLength),
		nullableWebString(discovery.RedirectURLValid, discovery.RedirectURL),
		discovery.SourceTool, discovery.Wordlist,
		nullableWebInt64(discovery.CommandLogIDValid, discovery.CommandLogID))
	if err != nil {
		return 0, fmt.Errorf("failed to save web discovery: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to inspect web discovery id: %w", err)
	}
	return id, nil
}

func ListWebDiscoveries(workspace *Workspace, target *Target) ([]WebDiscovery, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, target_id, url, path, status_code, content_length,
		       redirect_url, source_tool, wordlist, command_log_id,
		       created_at, updated_at
		FROM web_discoveries
		WHERE target_id = ?
		ORDER BY created_at ASC, id ASC
	`, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list web discoveries: %w", err)
	}
	defer rows.Close()

	var discoveries []WebDiscovery
	for rows.Next() {
		var discovery WebDiscovery
		var contentLength, commandLogID sql.NullInt64
		var redirectURL sql.NullString
		if err := rows.Scan(
			&discovery.ID,
			&discovery.TargetID,
			&discovery.URL,
			&discovery.Path,
			&discovery.StatusCode,
			&contentLength,
			&redirectURL,
			&discovery.SourceTool,
			&discovery.Wordlist,
			&commandLogID,
			&discovery.CreatedAt,
			&discovery.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to read web discovery: %w", err)
		}
		discovery.ContentLength, discovery.ContentLengthValid = nullableWebInt64Value(contentLength)
		discovery.RedirectURL, discovery.RedirectURLValid = nullableStringValue(redirectURL)
		discovery.CommandLogID, discovery.CommandLogIDValid = nullableWebInt64Value(commandLogID)
		discoveries = append(discoveries, discovery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list web discoveries: %w", err)
	}
	return discoveries, nil
}

func nullableWebInt64(valid bool, value int64) any {
	if !valid {
		return nil
	}
	return value
}

func nullableWebString(valid bool, value string) any {
	if !valid {
		return nil
	}
	return value
}

func nullableWebInt64Value(value sql.NullInt64) (int64, bool) {
	if !value.Valid {
		return 0, false
	}
	return value.Int64, true
}
