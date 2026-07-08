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
		if !strings.Contains(got, "Run ctx <command> -h for command-specific help.") {
			t.Fatalf("help output = %q, want command-specific help hint", got)
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

func TestRunSubcommandHelpDoesNotRequireWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ctx", "init", "-h"}, "usage: ctx init [options]"},
		{[]string{"ctx", "status", "--help"}, "usage: ctx status [options]"},
		{[]string{"ctx", "target", "-h"}, "usage: ctx target <command> [options]"},
		{[]string{"ctx", "target", "add", "--help"}, "usage: ctx target <command> [options]"},
		{[]string{"ctx", "ip", "-h"}, "usage: ctx ip [ip] [options]"},
		{[]string{"ctx", "host", "--help"}, "usage: ctx host <command> [options]"},
		{[]string{"ctx", "host", "add", "-h"}, "usage: ctx host <command> [options]"},
		{[]string{"ctx", "hosts", "-h"}, "usage: ctx hosts <command> [options]"},
		{[]string{"ctx", "hosts", "sync", "--help"}, "usage: ctx hosts <command> [options]"},
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
