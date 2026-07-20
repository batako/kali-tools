package ctx

import (
	"database/sql"
	"fmt"
	"strings"
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
	DiscoveryType      string
	TemplateURL        string
	ParameterName      string
	ParameterValue     string
	FuzzPart           string
	WordCount          int
	WordCountValid     bool
	LineCount          int
	LineCountValid     bool
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
			discovery_type, template_url, parameter_name, parameter_value, fuzz_part,
			word_count, line_count, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, target.ID, discovery.URL, discovery.Path, discovery.StatusCode,
		nullableWebInt64(discovery.ContentLengthValid, discovery.ContentLength),
		nullableWebString(discovery.RedirectURLValid, discovery.RedirectURL),
		discovery.SourceTool, discovery.Wordlist,
		nullableWebInt64(discovery.CommandLogIDValid, discovery.CommandLogID),
		webDiscoveryType(discovery.DiscoveryType), nullableWebText(discovery.TemplateURL),
		nullableWebText(discovery.ParameterName), nullableWebText(discovery.ParameterValue),
		nullableWebText(discovery.FuzzPart), nullableWebInt(discovery.WordCountValid, discovery.WordCount),
		nullableWebInt(discovery.LineCountValid, discovery.LineCount))
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
		       discovery_type, template_url, parameter_name, parameter_value, fuzz_part,
		       word_count, line_count, created_at, updated_at
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
		var contentLength, commandLogID, wordCount, lineCount sql.NullInt64
		var redirectURL, templateURL, parameterName, parameterValue, fuzzPart sql.NullString
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
			&discovery.DiscoveryType,
			&templateURL,
			&parameterName,
			&parameterValue,
			&fuzzPart,
			&wordCount,
			&lineCount,
			&discovery.CreatedAt,
			&discovery.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to read web discovery: %w", err)
		}
		discovery.ContentLength, discovery.ContentLengthValid = nullableWebInt64Value(contentLength)
		discovery.RedirectURL, discovery.RedirectURLValid = nullableStringValue(redirectURL)
		discovery.CommandLogID, discovery.CommandLogIDValid = nullableWebInt64Value(commandLogID)
		discovery.TemplateURL, _ = nullableStringValue(templateURL)
		discovery.ParameterName, _ = nullableStringValue(parameterName)
		discovery.ParameterValue, _ = nullableStringValue(parameterValue)
		discovery.FuzzPart, _ = nullableStringValue(fuzzPart)
		discovery.WordCount, discovery.WordCountValid = nullableWebIntValue(wordCount)
		discovery.LineCount, discovery.LineCountValid = nullableWebIntValue(lineCount)
		discoveries = append(discoveries, discovery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list web discoveries: %w", err)
	}
	return discoveries, nil
}

func GetWebDiscovery(workspace *Workspace, target *Target, id int64) (*WebDiscovery, error) {
	discoveries, err := ListWebDiscoveries(workspace, target)
	if err != nil {
		return nil, err
	}
	for i := range discoveries {
		if discoveries[i].ID == id {
			return &discoveries[i], nil
		}
	}
	return nil, fmt.Errorf("web discovery not found: %d", id)
}

func webDiscoveryType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "path"
	}
	return value
}

func nullableWebText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableWebInt(valid bool, value int) any {
	if !valid {
		return nil
	}
	return value
}

func nullableWebIntValue(value sql.NullInt64) (int, bool) {
	if !value.Valid {
		return 0, false
	}
	return int(value.Int64), true
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
