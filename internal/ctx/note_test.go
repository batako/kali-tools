package ctx

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRunNoteSavesNoteAndShowsItInLog(t *testing.T) {
	workspace := initXTestWorkspace(t)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "note", "SMB", "anonymous", "login", "possible"}, &out); err != nil {
		t.Fatalf("Run(ctx note) error = %v", err)
	}
	if got := out.String(); got != "saved note: note:1\n" {
		t.Fatalf("ctx note output = %q", got)
	}

	notes, err := ListNotes(workspace)
	if err != nil {
		t.Fatalf("ListNotes() error = %v", err)
	}
	if len(notes) != 1 || notes[0].Body != "SMB anonymous login possible" {
		t.Fatalf("notes = %+v, want saved note", notes)
	}

	out.Reset()
	if err := Run([]string{"ctx", "log"}, &out); err != nil {
		t.Fatalf("Run(ctx log) error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "note:1") || !strings.Contains(got, "note SMB anonymous login possible") {
		t.Fatalf("ctx log output = %q, want note timeline entry", got)
	}
}

func TestTimelineCombinesCommandsAndNotesChronologically(t *testing.T) {
	workspace := initXTestWorkspace(t)
	now := time.Now().UTC()

	if _, err := SaveCommandLog(workspace, CommandLog{
		Command:         "first command",
		ExpandedCommand: "first command",
		Status:          "success",
		StartedAt:       now.Add(-time.Hour).Format(time.RFC3339Nano),
		EndedAt:         now.Add(-time.Hour + time.Second).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveCommandLog(first) error = %v", err)
	}
	if _, err := SaveNote(workspace, "middle note"); err != nil {
		t.Fatalf("SaveNote() error = %v", err)
	}
	if _, err := SaveCommandLog(workspace, CommandLog{
		Command:         "last command",
		ExpandedCommand: "last command",
		Status:          "success",
		StartedAt:       now.Add(time.Hour).Format(time.RFC3339Nano),
		EndedAt:         now.Add(time.Hour + time.Second).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveCommandLog(last) error = %v", err)
	}

	entries, err := ListTimeline(workspace)
	if err != nil {
		t.Fatalf("ListTimeline() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("timeline length = %d, want 3", len(entries))
	}
	got := []string{entries[0].Text, entries[1].Text, entries[2].Text}
	want := []string{"first command", "middle note", "last command"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("timeline = %v, want %v", got, want)
		}
	}
}

func TestRunNoteRejectsEmptyBody(t *testing.T) {
	initXTestWorkspace(t)

	var out bytes.Buffer
	for _, args := range [][]string{
		{"ctx", "note"},
		{"ctx", "note", "   "},
	} {
		if err := Run(args, &out); err == nil {
			t.Fatalf("Run(%v) error = nil, want error", args)
		}
	}
}
