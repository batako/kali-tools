package ctx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionScriptsIncludeXFunctionForEveryCommand(t *testing.T) {
	commands := topLevelCommandsFromUsage(t)

	for _, shell := range []string{"zsh", "bash"} {
		script, err := CompletionScript(shell)
		if err != nil {
			t.Fatalf("CompletionScript(%s) error = %v", shell, err)
		}

		for _, command := range commands {
			var want string
			if command == "scan" {
				want = `xscan() { CTX_INVOKED_AS=xscan ctx scan "$@"`
			} else if command == "project" {
				want = `xproject() {`
			} else if command == "x" {
				want = `x() { ctx x "$@"`
			} else {
				want = "x" + command + `() { ctx ` + command + ` "$@"`
			}
			if !strings.Contains(script, want) {
				t.Errorf("CompletionScript(%s) missing function for %q", shell, command)
			}
		}
	}
}

func TestCompletionScriptsMapXInitToWorkspaceInit(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"zsh", "bash"} {
		script, err := CompletionScript(shell)
		if err != nil {
			t.Fatalf("CompletionScript(%s) error = %v", shell, err)
		}
		if !strings.Contains(script, `xinit() { ctx workspace init "$@"`) {
			t.Errorf("CompletionScript(%s) does not map xinit to ctx workspace init", shell)
		}
	}
}

func TestCompletionScriptsIncludeProjectHelpers(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		script, err := CompletionScript(shell)
		if err != nil {
			t.Fatalf("CompletionScript(%s) error = %v", shell, err)
		}
		for _, want := range []string{
			`xproject() {`,
			`if [[ $# == 0 || $1 == root || $1 == ls || $1 == rm || $1 == -h || $1 == --help ]]; then`,
			`project_path=$(ctx project "$@") || return`,
			`xnew() {`,
			`project_path=$(ctx project new "$@") || return`,
			`cd "${project_path}"`,
		} {
			if !strings.Contains(script, want) {
				t.Errorf("CompletionScript(%s) missing %q", shell, want)
			}
		}
	}
}

func TestCompletionScriptsIncludeAllConfigKeys(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		script, err := CompletionScript(shell)
		if err != nil {
			t.Fatalf("CompletionScript(%s) error = %v", shell, err)
		}
		for _, want := range []string{"project.root", "web.directory.max-requests", "web.file.max-requests", "password.max-requests", "web.tls.verify"} {
			if !strings.Contains(script, want) {
				t.Errorf("CompletionScript(%s) missing config key %q", shell, want)
			}
		}
	}
}

func TestCompletionScriptsIncludeExtraShortcutsWhenRequested(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		script, err := CompletionScript(shell, CompletionOptions{ExtraShortcuts: true})
		if err != nil {
			t.Fatalf("CompletionScript(%s, true) error = %v", shell, err)
		}
		wants := []string{
			"pj() { xproject \"$@\"",
			"ta() { xtarget \"$@\"",
			"unalias cr 2>/dev/null",
			"cr() { xcredential \"$@\"",
			"sv() { xservice \"$@\"",
		}
		if shell == "zsh" {
			wants = append(wants,
				"elif [[ ${invocation} == pj ]]",
				"elif [[ ${invocation} == ta ]]",
				"elif [[ ${invocation} == cr ]]",
				"elif [[ ${invocation} == sv ]]",
				"compdef _ctx xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset pj ta cr sv",
			)
		} else {
			wants = append(wants,
				"project|xproject|pj",
				"target|xtarget|ta",
				"credential|xcredential|cr",
				"service|xservice|sv",
			)
		}
		for _, want := range wants {
			if !strings.Contains(script, want) {
				t.Fatalf("CompletionScript(%s, true) missing %q", shell, want)
			}
		}
	}
}

func TestCompletionScriptsDoNotIncludeExtraShortcutsByDefault(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		script, err := CompletionScript(shell)
		if err != nil {
			t.Fatalf("CompletionScript(%s) error = %v", shell, err)
		}
		for _, unwanted := range []string{
			"pj() { xproject",
			"ta() { xtarget",
			"cr() { xcredential",
			"sv() { xservice",
			"elif [[ ${invocation} == pj ]]",
			"elif [[ ${invocation} == cr ]]",
			"elif [[ ${invocation} == sv ]]",
			"project|xproject|pj",
			"credential|xcredential|cr",
			"pj ta cr sv",
		} {
			if strings.Contains(script, unwanted) {
				t.Fatalf("CompletionScript(%s) unexpectedly contains %q", shell, unwanted)
			}
		}
	}
}

func topLevelCommandsFromUsage(t *testing.T) []string {
	t.Helper()

	var commands []string
	inCommands := false
	for _, line := range strings.Split(usageText, "\n") {
		switch strings.TrimSpace(line) {
		case "commands:":
			inCommands = true
			continue
		case "options:":
			inCommands = false
		}
		if !inCommands {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			commands = append(commands, fields[0])
		}
	}

	if len(commands) == 0 {
		t.Fatal("usageText contains no top-level commands")
	}
	return commands
}

func TestZshCompletionIncludesDescribedSubcommandsAndXCommandRouting(t *testing.T) {
	script, err := CompletionScript("zsh")
	if err != nil {
		t.Fatalf("CompletionScript(zsh) error = %v", err)
	}

	for _, want := range []string{
		"'set:create or update the primary target'",
		"'rm:remove a workspace and all of its ctx data'",
		"'new:create a project and initialize a workspace'",
		"'add:add a hostname'",
		"'sync:sync the managed block to /etc/hosts'",
		"'zsh:print zsh completion script'",
		"'--interactive:open the interactive timeline'",
		"'--field:print one prompt field'",
		"invocation=${words[1]:t}",
		"elif [[ ${invocation} == x ]]",
		"command=${invocation#x}",
		"-z ${command} || CURRENT == 2",
		"CURRENT == command_position + 1",
		"_describe 'target command' _ctx_target_commands",
		"_describe 'workspace command' _ctx_workspace_commands",
		"_describe 'project command' _ctx_project_commands",
		"_describe 'log option' _ctx_log_options",
		"_describe 'prompt option' _ctx_prompt_options",
		"_ctx_dynamic_descriptions target",
		"elif [[ ${previous} == --target ]]",
		"ctx completion descriptions \"${kind}\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("CompletionScript(zsh) missing %q", want)
		}
	}
}

func TestBashCompletionIncludesDynamicWorkspaceValues(t *testing.T) {
	script, err := CompletionScript("bash")
	if err != nil {
		t.Fatalf("CompletionScript(bash) error = %v", err)
	}
	for _, want := range []string{
		"_ctx_complete_values target",
		"_ctx_complete_values host",
		"_ctx_complete_values workspace",
		"_ctx_complete_values project",
		"_ctx_complete_values log",
		`ctx completion values "${kind}"`,
		"${subcommand} == use",
		"${command} == host && ${prev} == --target",
		"${prev} == --target",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("CompletionScript(bash) missing %q", want)
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
	if strings.Contains(text, "--extra-shortcuts") {
		t.Fatalf("shell config = %q, should not include extra shortcuts by default", text)
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
	if got := out.String(); !strings.Contains(got, "complete -F _ctx_completion ctx") || !strings.Contains(got, "xinit()") || !strings.Contains(got, "x() { ctx x") {
		t.Fatalf("completion output = %q", got)
	}
	if strings.Contains(out.String(), "pj()") || strings.Contains(out.String(), "ta()") || strings.Contains(out.String(), "cr()") || strings.Contains(out.String(), "sv()") {
		t.Fatalf("completion output unexpectedly contains extra shortcuts: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"ctx", "completion", "bash", "--extra-shortcuts"}, &out); err != nil {
		t.Fatalf("Run(completion bash --extra-shortcuts) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "pj() { xproject \"$@\"") || !strings.Contains(got, "ta() { xtarget \"$@\"") || !strings.Contains(got, "cr() { xcredential \"$@\"") || !strings.Contains(got, "sv() { xservice \"$@\"") {
		t.Fatalf("completion output with extra shortcuts = %q", got)
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
	if !strings.Contains(string(content), "source <(ctx completion bash)") || strings.Contains(string(content), "--extra-shortcuts") {
		t.Fatalf(".bashrc = %q", string(content))
	}

	out.Reset()
	if err := Run([]string{"ctx", "init-shell", "--extra-shortcuts"}, &out); err != nil {
		t.Fatalf("Run(init-shell --extra-shortcuts) error = %v", err)
	}
	content, err = os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("ReadFile(.bashrc extra) error = %v", err)
	}
	if !strings.Contains(string(content), "source <(ctx completion bash --extra-shortcuts)") {
		t.Fatalf(".bashrc with extra shortcuts = %q", string(content))
	}
}
