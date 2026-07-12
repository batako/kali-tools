package ctx

import (
	"fmt"
	"sort"
	"time"
)

type Note struct {
	ID          int64
	WorkspaceID int64
	Body        string
	CreatedAt   string
}

type TimelineEntry struct {
	ID        int64
	Ref       string
	Time      string
	Status    string
	ExitCode  int
	Text      string
	IsCommand bool
}

func SaveNote(workspace *Workspace, body string) (*Note, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := db.Exec(`
		INSERT INTO notes (workspace_id, body, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, workspace.ID, body, createdAt, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save note: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect note id: %w", err)
	}
	return &Note{ID: id, WorkspaceID: workspace.ID, Body: body, CreatedAt: createdAt}, nil
}

func ListNotes(workspace *Workspace) ([]Note, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, workspace_id, body, created_at
		FROM notes
		WHERE workspace_id = ?
		ORDER BY created_at ASC, id ASC
	`, workspace.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var note Note
		if err := rows.Scan(&note.ID, &note.WorkspaceID, &note.Body, &note.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to read note: %w", err)
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list notes: %w", err)
	}
	return notes, nil
}

func ListTimeline(workspace *Workspace) ([]TimelineEntry, error) {
	logs, err := ListCommandLogs(workspace)
	if err != nil {
		return nil, err
	}
	notes, err := ListNotes(workspace)
	if err != nil {
		return nil, err
	}

	entries := make([]TimelineEntry, 0, len(logs)+len(notes))
	for _, log := range logs {
		entries = append(entries, TimelineEntry{
			ID:        log.ID,
			Ref:       fmt.Sprintf("%d", log.ID),
			Time:      log.StartedAt,
			Status:    log.Status,
			ExitCode:  log.ExitCode,
			Text:      log.Command,
			IsCommand: true,
		})
	}
	for _, note := range notes {
		entries = append(entries, TimelineEntry{
			ID:     note.ID,
			Ref:    fmt.Sprintf("note:%d", note.ID),
			Time:   note.CreatedAt,
			Status: "note",
			Text:   note.Body,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		left, leftOK := timelineTime(entries[i].Time)
		right, rightOK := timelineTime(entries[j].Time)
		if leftOK && rightOK && !left.Equal(right) {
			return left.Before(right)
		}
		if entries[i].Time != entries[j].Time {
			return entries[i].Time < entries[j].Time
		}
		if entries[i].IsCommand != entries[j].IsCommand {
			return entries[i].IsCommand
		}
		return entries[i].ID < entries[j].ID
	})
	return entries, nil
}

func timelineTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
