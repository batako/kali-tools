package ctx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionScriptIncludesXCommandFunctions(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		script, err := CompletionScript(shell)
		if err != nil {
			t.Fatalf("CompletionScript(%s) error = %v", shell, err)
		}
		for _, want := range []string{"xinit", "xstatus", "xtarget", "xhosts", "xdoctor", "xinit-shell"} {
			if !strings.Contains(script, want) {
				t.Fatalf("CompletionScript(%s) missing %q in %q", shell, want, script)
			}
		}
	}
}

func TestZshCompletionIncludesDescribedSubcommandsAndXCommandRouting(t *testing.T) {
	script, err := CompletionScript("zsh")
	if err != nil {
		t.Fatalf("CompletionScript(zsh) error = %v", err)
	}

	for _, want := range []string{
		"'set:create or update the primary target'",
		"'add:add a hostname'",
		"'sync:sync the managed block to /etc/hosts'",
		"'zsh:print zsh completion script'",
		"invocation=${words[1]:t}",
		"command=${invocation#x}",
		"-z ${command} || CURRENT == 2",
		"CURRENT == command_position + 1",
		"_describe 'target command' _ctx_target_commands",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("CompletionScript(zsh) missing %q", want)
		}
	}
}

func TestInitShellWritesAndRemovesMarkedBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	config, changed, err := InstallShellConfig()
	if err != nil {
		t.Fatalf("InstallShellConfig() error = %v", err)
	}
	if !changed {
		t.Fatal("InstallShellConfig() changed = false, want true")
	}
	if config.Path != filepath.Join(home, ".zshrc") {
		t.Fatalf("config path = %q", config.Path)
	}

	content, err := os.ReadFile(config.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, shellBlockStart) || !strings.Contains(text, "source <(ctx completion zsh)") || !strings.Contains(text, shellBlockEnd) {
		t.Fatalf("shell config = %q, want ctx block", text)
	}

	_, changed, err = InstallShellConfig()
	if err != nil {
		t.Fatalf("InstallShellConfig() second error = %v", err)
	}
	if changed {
		t.Fatal("InstallShellConfig() second changed = true, want false")
	}

	_, changed, err = RemoveShellConfig()
	if err != nil {
		t.Fatalf("RemoveShellConfig() error = %v", err)
	}
	if !changed {
		t.Fatal("RemoveShellConfig() changed = false, want true")
	}
	content, err = os.ReadFile(config.Path)
	if err != nil {
		t.Fatalf("ReadFile() after remove error = %v", err)
	}
	if strings.Contains(string(content), shellBlockStart) || strings.Contains(string(content), shellBlockEnd) {
		t.Fatalf("shell config after remove = %q, want ctx block removed", string(content))
	}
}

func TestDetectShellFallsBackToParentProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/sh")

	oldParentProcessNameFunc := parentProcessNameFunc
	parentProcessNameFunc = func() (string, error) {
		return "/usr/bin/zsh", nil
	}
	t.Cleanup(func() { parentProcessNameFunc = oldParentProcessNameFunc })

	config, err := DetectShell()
	if err != nil {
		t.Fatalf("DetectShell() error = %v", err)
	}
	if config.Shell != "zsh" {
		t.Fatalf("shell = %q, want zsh", config.Shell)
	}
	if config.Path != filepath.Join(home, ".zshrc") {
		t.Fatalf("path = %q, want .zshrc", config.Path)
	}
}

func TestNormalizeShellName(t *testing.T) {
	tests := map[string]string{
		"/usr/bin/zsh": "zsh",
		"-zsh":         "zsh",
		"bash -l":      "bash",
		"":             "",
	}

	for input, want := range tests {
		if got := normalizeShellName(input); got != want {
			t.Fatalf("normalizeShellName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRunCompletionAndInitShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "completion", "bash"}, &out); err != nil {
		t.Fatalf("Run(completion bash) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "complete -F _ctx_completion ctx") || !strings.Contains(got, "xinit()") {
		t.Fatalf("completion output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "init-shell"}, &out); err != nil {
		t.Fatalf("Run(init-shell) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "configured ctx shell integration") {
		t.Fatalf("init-shell output = %q", got)
	}

	content, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("ReadFile(.bashrc) error = %v", err)
	}
	if !strings.Contains(string(content), "source <(ctx completion bash)") {
		t.Fatalf(".bashrc = %q", string(content))
	}
}
