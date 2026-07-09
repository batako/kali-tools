package ctx

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunTopLevelHelp(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"-h", "--help"} {
		var out bytes.Buffer
		if err := Run([]string{"ctx", arg}, &out); err != nil {
			t.Fatalf("Run(%s) error = %v", arg, err)
		}
		got := out.String()
		if !strings.Contains(got, "usage: ctx <command> [options]") {
			t.Fatalf("help output = %q, want top-level usage", got)
		}
		if !strings.Contains(got, "-h, --help") {
			t.Fatalf("help output = %q, want -h/--help option", got)
		}
		if !strings.Contains(got, "-V, --version") {
			t.Fatalf("help output = %q, want -V/--version option", got)
		}
		for _, command := range []string{"completion", "init-shell", "doctor"} {
			if !strings.Contains(got, command) {
				t.Fatalf("help output = %q, want command %q", got, command)
			}
		}
		if !strings.Contains(got, "Run ctx <command> -h for command-specific help.") {
			t.Fatalf("help output = %q, want command-specific help hint", got)
		}
		if !strings.Contains(got, "shortcuts (requires ctx init-shell):") {
			t.Fatalf("help output = %q, want shell shortcut requirement", got)
		}
		shortcuts := map[string]string{
			"xinit":       "ctx workspace init",
			"xstatus":     "ctx status",
			"xworkspace":  "ctx workspace",
			"xtarget":     "ctx target",
			"xip":         "ctx ip",
			"xhost":       "ctx host",
			"xhosts":      "ctx hosts",
			"xnote":       "ctx note",
			"xlog":        "ctx log",
			"x":           "ctx x",
			"xcompletion": "ctx completion",
			"xdoctor":     "ctx doctor",
			"xinit-shell": "ctx init-shell",
		}
		for shortcut, command := range shortcuts {
			if !strings.Contains(got, shortcut) || !strings.Contains(got, command) {
				t.Fatalf("help output = %q, want shortcut %s for %s", got, shortcut, command)
			}
		}
		for _, detail := range []string{"target set <ip>", "host add <hostname>", "hosts sync"} {
			if strings.Contains(got, detail) {
				t.Fatalf("help output = %q, should not include detailed command %q", got, detail)
			}
		}
		if strings.Contains(got, "  help") {
			t.Fatalf("help output = %q, should not list ctx help command", got)
		}
	}
}

func TestRunVersion(t *testing.T) {
	oldVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = oldVersion })

	for _, arg := range []string{"-V", "--version"} {
		var out bytes.Buffer
		if err := Run([]string{"ctx", arg}, &out); err != nil {
			t.Fatalf("Run(ctx %s) error = %v", arg, err)
		}
		if got, want := out.String(), "ctx 1.2.3\n"; got != want {
			t.Fatalf("version output = %q, want %q", got, want)
		}
	}
}

func TestRunDoesNotSupportHelpCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := Run([]string{"ctx", "help"}, &out)
	if err == nil {
		t.Fatal("Run(ctx help) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown ctx command: help") {
		t.Fatalf("error = %q, want unknown ctx command", err.Error())
	}
}

func TestRunDoesNotSupportTopLevelInit(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := Run([]string{"ctx", "init"}, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown ctx command: init") {
		t.Fatalf("Run(ctx init) error = %v, want unknown command", err)
	}
}

func TestRunSubcommandHelpDoesNotRequireWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ctx", "status", "--help"}, "usage: ctx status [options]"},
		{[]string{"ctx", "workspace", "-h"}, "usage: ctx workspace <command> [options]"},
		{[]string{"ctx", "workspace", "init", "--help"}, "usage: ctx workspace <command> [options]"},
		{[]string{"ctx", "workspace", "rm", "--help"}, "usage: ctx workspace <command> [options]"},
		{[]string{"ctx", "target", "-h"}, "usage: ctx target <command> [options]"},
		{[]string{"ctx", "target", "add", "--help"}, "usage: ctx target <command> [options]"},
		{[]string{"ctx", "ip", "-h"}, "usage: ctx ip [ip] [options]"},
		{[]string{"ctx", "host", "--help"}, "usage: ctx host <command> [options]"},
		{[]string{"ctx", "host", "add", "-h"}, "usage: ctx host <command> [options]"},
		{[]string{"ctx", "hosts", "-h"}, "usage: ctx hosts <command> [options]"},
		{[]string{"ctx", "hosts", "sync", "--help"}, "usage: ctx hosts <command> [options]"},
		{[]string{"ctx", "note", "--help"}, "usage: ctx note <text> [options]"},
		{[]string{"ctx", "log", "--help"}, "usage: ctx log [id] [options]"},
		{[]string{"ctx", "prompt", "--help"}, "usage: ctx prompt [options]"},
		{[]string{"ctx", "x", "--help"}, "usage: ctx x <command> [args...]"},
		{[]string{"ctx", "completion", "-h"}, "usage: ctx completion <zsh|bash> [options]"},
		{[]string{"ctx", "init-shell", "--help"}, "usage: ctx init-shell [--remove] [options]"},
		{[]string{"ctx", "doctor", "-h"}, "usage: ctx doctor [options]"},
	}

	for _, tt := range tests {
		var out bytes.Buffer
		if err := Run(tt.args, &out); err != nil {
			t.Fatalf("Run(%v) error = %v", tt.args, err)
		}
		if got := out.String(); !strings.Contains(got, tt.want) {
			t.Fatalf("Run(%v) output = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestRunLogHelpListsDisplayModes(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := Run([]string{"ctx", "log", "--help"}, &out); err != nil {
		t.Fatalf("Run(ctx log --help) error = %v", err)
	}
	for _, option := range []string{"-p, --plain", "-v, --verbose", "-i, --interactive"} {
		if !strings.Contains(out.String(), option) {
			t.Fatalf("log help = %q, want %s", out.String(), option)
		}
	}
}
