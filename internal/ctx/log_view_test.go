package ctx

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWritePlainTimelineShowsOnlyTimeAndAction(t *testing.T) {
	entries := []TimelineEntry{
		{Time: "2026-07-09T10:00:00Z", Status: "success", Text: "nmap -sV 10.0.0.1", IsCommand: true},
		{Time: "2026-07-09T10:01:00Z", Status: "note", Text: "SMB anonymous login", IsCommand: false},
		{Time: "2026-07-09T10:02:00Z", Status: "failed", Text: "curl http://target", IsCommand: true},
	}

	var out bytes.Buffer
	if err := writePlainTimeline(&out, entries); err != nil {
		t.Fatalf("writePlainTimeline() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"2026-07-09 10:00    nmap -sV 10.0.0.1",
		"2026-07-09 10:01  # SMB anonymous login",
		"2026-07-09 10:02  ! curl http://target",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain timeline = %q, want %q", got, want)
		}
	}
	for _, unwanted := range []string{"success", "failed", "note:", "exit"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("plain timeline = %q, should not contain %q", got, unwanted)
		}
	}
}

func TestCommandOutputSectionsKeepStreamsSeparate(t *testing.T) {
	got := commandOutputSections("stdout without newline", "stderr output")
	want := "---------------- stdout ----------------\n" +
		"stdout without newline\n" +
		"\n" +
		"---------------- stderr ----------------\n" +
		"stderr output"
	if got != want {
		t.Fatalf("commandOutputSections() = %q, want %q", got, want)
	}
}

func TestCommandOutputSectionsHideEmptyStreams(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		stderr string
		want   string
	}{
		{
			name:   "stdout only",
			stdout: "output\n",
			want:   "---------------- stdout ----------------\noutput\n",
		},
		{
			name:   "stderr only",
			stderr: "error\n",
			want:   "---------------- stderr ----------------\nerror\n",
		},
		{
			name: "both empty",
			want: "",
		},
	}
	for _, tt := range tests {
		if got := commandOutputSections(tt.stdout, tt.stderr); got != tt.want {
			t.Errorf("%s: commandOutputSections() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestCommandOutputSectionsSanitizesTerminalControlSequences(t *testing.T) {
	stdout := "\x1b[?2004h\x1b]0;testuser@target: ~\a\x1b[01;32mtestuser\x1b[00m:~$ j\b \bcd ../\n\x1b[?2004l\n/home/testuser\n"

	got := commandOutputSections(stdout, "")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("output = %q, must not contain ANSI escape sequences", got)
	}
	for _, want := range []string{"testuser:~$ cd ../", "/home/testuser"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "cd ../\n\n") {
		t.Fatalf("output = %q, must not contain PTY duplicate blank line", got)
	}
}

func TestSanitizeTerminalOutputReconstructsEditedInput(t *testing.T) {
	stdout := "testuser$ hoge\b \b\b \b\b \b\b \becho hge\b \b\b \b\b \basdf > aaa\n"

	got := sanitizeTerminalOutput(stdout)
	if got != "testuser$ echo asdf > aaa\n" {
		t.Fatalf("sanitizeTerminalOutput() = %q, want edited command", got)
	}
}

func TestSanitizeTerminalOutputRemovesBracketedPasteNewline(t *testing.T) {
	stdout := "testuser$ pwd\n\x1b[?2004l\n/home/testuser\n"

	got := sanitizeTerminalOutput(stdout)
	if got != "testuser$ pwd\n/home/testuser\n" {
		t.Fatalf("sanitizeTerminalOutput() = %q, want no control-sequence blank line", got)
	}
}

func TestLogModelOpensDetailAndQReturnsThenQuits(t *testing.T) {
	entry := TimelineEntry{
		ID:        7,
		Ref:       "7",
		Time:      "2026-07-09T10:00:00Z",
		Status:    "success",
		Text:      "echo hello",
		IsCommand: true,
	}
	model := newLogModel([]TimelineEntry{entry}, func(id int64) (*CommandLog, error) {
		if id != 7 {
			t.Fatalf("loader id = %d, want 7", id)
		}
		return &CommandLog{
			ID:              7,
			Command:         "echo hello",
			ExpandedCommand: "echo hello",
			Status:          "success",
			Stdout:          "hello\n",
			StartedAt:       entry.Time,
			EndedAt:         "2026-07-09T10:00:01Z",
		}, nil
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("enter command should be nil")
	}
	detail := updated.(logModel)
	if !strings.Contains(detail.View(), "---------------- stdout ----------------") || !strings.Contains(detail.View(), "hello") {
		t.Fatalf("detail view = %q, want command output", detail.View())
	}

	updated, cmd = detail.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("q from detail should not quit")
	}
	list := updated.(logModel)
	if list.detail != "" || !strings.Contains(list.View(), "echo hello") {
		t.Fatalf("list view after q = %q", list.View())
	}

	_, cmd = list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q from list should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q command message = %T, want tea.QuitMsg", cmd())
	}
}

func TestLogModelStartsAtNewestAndMovesThroughTimeline(t *testing.T) {
	entries := []TimelineEntry{
		{ID: 1, Text: "first", IsCommand: true},
		{ID: 2, Text: "second", IsCommand: true},
		{ID: 3, Text: "third", IsCommand: true},
	}
	model := newLogModel(entries, nil)
	if model.cursor != 2 {
		t.Fatalf("initial cursor = %d, want newest entry 2", model.cursor)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(logModel)
	if model.cursor != 1 {
		t.Fatalf("cursor after up = %d, want 1", model.cursor)
	}
}

func TestParseLogArgs(t *testing.T) {
	tests := []struct {
		args     []string
		wantID   string
		wantMode logDisplayMode
		wantErr  bool
	}{
		{nil, "", logDisplayAuto, false},
		{[]string{"-p"}, "", logDisplayPlain, false},
		{[]string{"--plain"}, "", logDisplayPlain, false},
		{[]string{"-v"}, "", logDisplayVerbose, false},
		{[]string{"--verbose"}, "", logDisplayVerbose, false},
		{[]string{"-i"}, "", logDisplayInteractive, false},
		{[]string{"--interactive"}, "", logDisplayInteractive, false},
		{[]string{"12"}, "12", logDisplayAuto, false},
		{[]string{"12", "--plain"}, "", logDisplayAuto, true},
		{[]string{"--plain", "--verbose"}, "", logDisplayAuto, true},
	}
	for _, tt := range tests {
		id, mode, err := parseLogArgs(tt.args)
		if (err != nil) != tt.wantErr {
			t.Fatalf("parseLogArgs(%v) error = %v, wantErr %t", tt.args, err, tt.wantErr)
		}
		if !tt.wantErr && (id != tt.wantID || mode != tt.wantMode) {
			t.Fatalf("parseLogArgs(%v) = %q, %v; want %q, %v", tt.args, id, mode, tt.wantID, tt.wantMode)
		}
	}
}
