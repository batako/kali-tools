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
		for _, command := range []string{"project", "scan", "completion", "init-shell", "doctor"} {
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
			"xproject":    "ctx project",
			"xnew":        "ctx project new",
			"xtarget":     "ctx target",
			"xip":         "ctx ip",
			"xhost":       "ctx host",
			"xhosts":      "ctx hosts",
			"xscan":       "ctx scan",
			"xnote":       "ctx note",
			"xlog":        "ctx log",
			"x":           "ctx x",
			"xcompletion": "ctx completion",
			"xdoctor":     "ctx doctor",
			"xinit-shell": "ctx init-shell",
			"xreset":      "ctx reset",
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

func TestResourceWithoutDefaultActionReportsUnknownCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := Run([]string{"ctx", "workspace", "hoge"}, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown ctx workspace command: hoge") {
		t.Fatalf("Run(ctx workspace hoge) error = %v, want unknown workspace command", err)
	}
}

func TestResolveResourceActionRejectsDestructiveDefaultAction(t *testing.T) {
	t.Parallel()

	_, err := resolveResourceAction("example", []string{"value"}, []string{"rm"}, "rm")
	if err == nil || !strings.Contains(err.Error(), "invalid default action") {
		t.Fatalf("resolveResourceAction() error = %v, want invalid default action", err)
	}
}

func TestResolveResourceCommandUsesDefaultView(t *testing.T) {
	t.Parallel()

	args, showHelp, err := resolveResourceCommand("example", nil, []string{"ls"}, "", "ls")
	if err != nil {
		t.Fatalf("resolveResourceCommand() error = %v", err)
	}
	if showHelp || len(args) != 1 || args[0] != "ls" {
		t.Fatalf("resolveResourceCommand() = %v, %v, want ls without help", args, showHelp)
	}
}

func TestResolveResourceCommandDefaultsToHelpView(t *testing.T) {
	t.Parallel()

	args, showHelp, err := resolveResourceCommand("example", nil, []string{"ls"}, "", "help")
	if err != nil {
		t.Fatalf("resolveResourceCommand() error = %v", err)
	}
	if !showHelp || args != nil {
		t.Fatalf("resolveResourceCommand() = %v, %v, want help", args, showHelp)
	}
}

func TestResolveResourceCommandRejectsDestructiveDefaultView(t *testing.T) {
	t.Parallel()

	_, _, err := resolveResourceCommand("example", nil, []string{"rm"}, "", "rm")
	if err == nil || !strings.Contains(err.Error(), "invalid default view") {
		t.Fatalf("resolveResourceCommand() error = %v, want invalid default view", err)
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
		{[]string{"ctx", "project", "-h"}, "usage: ctx project [<name> | <command>] [options]"},
		{[]string{"ctx", "project", "new", "--help"}, "usage: ctx project [<name> | <command>] [options]"},
		{[]string{"ctx", "target", "-h"}, "usage: ctx target [<ip> | <command>] [options]"},
		{[]string{"ctx", "target", "add", "--help"}, "usage: ctx target [<ip> | <command>] [options]"},
		{[]string{"ctx", "ip", "-h"}, "usage: ctx ip [ip] [options]"},
		{[]string{"ctx", "host", "--help"}, "usage: ctx host [<hostname> | <command>] [options]"},
		{[]string{"ctx", "host", "add", "-h"}, "usage: ctx host [<hostname> | <command>] [options]"},
		{[]string{"ctx", "hosts", "-h"}, "usage: ctx hosts <command> [options]"},
		{[]string{"ctx", "hosts", "sync", "--help"}, "usage: ctx hosts <command> [options]"},
		{[]string{"ctx", "scan", "--help"}, "usage: ctx scan [ip] [options]"},
		{[]string{"ctx", "note", "--help"}, "usage: ctx note <text> [options]"},
		{[]string{"ctx", "log", "--help"}, "usage: ctx log [id] [options]"},
		{[]string{"ctx", "prompt", "--help"}, "usage: ctx prompt [options]"},
		{[]string{"ctx", "x", "--help"}, "usage: ctx x <command> [args...]"},
		{[]string{"ctx", "completion", "-h"}, "usage: ctx completion <zsh|bash> [options]"},
		{[]string{"ctx", "init-shell", "--help"}, "usage: ctx init-shell [--remove] [options]"},
		{[]string{"ctx", "doctor", "-h"}, "usage: ctx doctor [options]"},
		{[]string{"ctx", "reset", "-h"}, "usage: ctx reset [-y|--yes] [options]"},
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

func TestRunResourceHelpListsDefaultActionShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ctx", "target", "--help"}, "ctx target <ip>          same as 'ctx target set <ip>'"},
		{[]string{"ctx", "host", "--help"}, "ctx host <hostname>              same as 'ctx host add <hostname>'"},
		{[]string{"ctx", "project", "--help"}, "ctx project <name>           same as 'ctx project new <name>'"},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if err := Run(tt.args, &out); err != nil {
			t.Fatalf("Run(%v) error = %v", tt.args, err)
		}
		if got := out.String(); !strings.Contains(got, "shorthand:") || !strings.Contains(got, tt.want) {
			t.Fatalf("Run(%v) output = %q, want shorthand %q", tt.args, got, tt.want)
		}
	}
}
