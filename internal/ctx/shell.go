package ctx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	shellBlockStart = "# >>> ctx >>>"
	shellBlockEnd   = "# <<< ctx <<<"
)

type ShellConfig struct {
	Shell string
	Path  string
}

var parentProcessNameFunc = parentProcessName

func CompletionScript(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshCompletionScript, nil
	case "bash":
		return bashCompletionScript, nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

func DetectShell() (ShellConfig, error) {
	rawShell := os.Getenv("SHELL")
	shell := normalizeShellName(rawShell)
	if shell == "" || !isSupportedShell(shell) {
		if parentName, err := parentProcessNameFunc(); err == nil {
			if parentShell := normalizeShellName(parentName); isSupportedShell(parentShell) {
				shell = parentShell
			}
		}
	}

	if shell == "" {
		return ShellConfig{}, errors.New("failed to detect shell: SHELL is not set")
	}
	if !isSupportedShell(shell) {
		return ShellConfig{}, fmt.Errorf("unsupported shell: %s", normalizeShellName(rawShell))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ShellConfig{}, fmt.Errorf("failed to locate home directory: %w", err)
	}
	name := "." + shell + "rc"
	return ShellConfig{Shell: shell, Path: filepath.Join(home, name)}, nil
}

func normalizeShellName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if fields := strings.Fields(value); len(fields) > 0 {
		value = fields[0]
	}
	return strings.TrimPrefix(filepath.Base(value), "-")
}

func isSupportedShell(shell string) bool {
	return shell == "zsh" || shell == "bash"
}

func parentProcessName() (string, error) {
	output, err := exec.Command("ps", "-p", strconv.Itoa(os.Getppid()), "-o", "comm=").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func InstallShellConfig() (ShellConfig, bool, error) {
	config, err := DetectShell()
	if err != nil {
		return ShellConfig{}, false, err
	}

	content, err := os.ReadFile(config.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ShellConfig{}, false, fmt.Errorf("failed to read %s: %w", config.Path, err)
	}

	text := string(content)
	block := shellBlock(config.Shell)
	updated, changed := upsertMarkedBlock(text, block)
	if !changed {
		return config, false, nil
	}

	if err := os.MkdirAll(filepath.Dir(config.Path), 0755); err != nil {
		return ShellConfig{}, false, fmt.Errorf("failed to create %s: %w", filepath.Dir(config.Path), err)
	}
	if err := os.WriteFile(config.Path, []byte(updated), 0644); err != nil {
		return ShellConfig{}, false, fmt.Errorf("failed to write %s: %w", config.Path, err)
	}

	return config, true, nil
}

func RemoveShellConfig() (ShellConfig, bool, error) {
	config, err := DetectShell()
	if err != nil {
		return ShellConfig{}, false, err
	}

	content, err := os.ReadFile(config.Path)
	if errors.Is(err, os.ErrNotExist) {
		return config, false, nil
	}
	if err != nil {
		return ShellConfig{}, false, fmt.Errorf("failed to read %s: %w", config.Path, err)
	}

	updated, changed := removeMarkedBlock(string(content))
	if !changed {
		return config, false, nil
	}
	if err := os.WriteFile(config.Path, []byte(updated), 0644); err != nil {
		return ShellConfig{}, false, fmt.Errorf("failed to write %s: %w", config.Path, err)
	}
	return config, true, nil
}

func CompletionConfigured(config ShellConfig) (bool, error) {
	content, err := os.ReadFile(config.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(content), shellBlockStart) && strings.Contains(string(content), shellBlockEnd), nil
}

func shellBlock(shell string) string {
	return shellBlockStart + "\nsource <(ctx completion " + shell + ")\n" + shellBlockEnd + "\n"
}

func upsertMarkedBlock(text, block string) (string, bool) {
	start := strings.Index(text, shellBlockStart)
	if start != -1 {
		end := strings.Index(text[start:], shellBlockEnd)
		if end == -1 {
			return text, false
		}
		end = start + end + len(shellBlockEnd)
		if end < len(text) && text[end] == '\n' {
			end++
		}
		if text[start:end] == block {
			return text, false
		}
		return text[:start] + block + text[end:], true
	}

	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" && !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	return text + block, true
}

func removeMarkedBlock(text string) (string, bool) {
	start := strings.Index(text, shellBlockStart)
	if start == -1 {
		return text, false
	}
	end := strings.Index(text[start:], shellBlockEnd)
	if end == -1 {
		return text, false
	}
	end = start + end + len(shellBlockEnd)
	if end < len(text) && text[end] == '\n' {
		end++
	}
	if start > 0 && text[start-1] == '\n' && end < len(text) && text[end] == '\n' {
		start--
	}
	return text[:start] + text[end:], true
}

const zshCompletionScript = `#compdef ctx xinit xstatus xworkspace xtarget xip xhost xhosts xlog x xcompletion xdoctor xinit-shell

_ctx_commands=(
  'status:show the current workspace'
  'workspace:initialize, list, or remove workspaces'
  'target:manage targets'
  'ip:show or update the primary target IP'
  'host:manage hostnames'
  'hosts:show, sync, or clean /etc/hosts entries'
  'log:show command execution logs'
  'x:run a command and save execution logs'
  'completion:print shell completion script'
  'init-shell:configure shell integration'
  'doctor:check ctx environment'
)

_ctx_workspace_commands=(
  'init:create a workspace in the current directory'
  'ls:list workspaces'
  'rm:remove a workspace and all of its ctx data'
)

_ctx_target_commands=(
  'set:create or update the primary target'
  'add:add a target'
  'update:update the primary target IP'
  'use:make a target primary'
  'rm:remove a target'
  'ls:list targets'
)

_ctx_host_commands=(
  'add:add a hostname'
  'rm:remove a hostname'
  'ls:list hostnames'
)

_ctx_hosts_commands=(
  'show:show the managed hosts block'
  'sync:sync the managed block to /etc/hosts'
  'clean:remove the managed block from /etc/hosts'
)

_ctx_completion_shells=(
  'zsh:print zsh completion script'
  'bash:print bash completion script'
)

_ctx_options=(
  '-h:show help'
  '--help:show help'
)

_ctx() {
  local invocation command command_position
  invocation=${words[1]:t}

  if [[ ${invocation} == ctx ]]; then
    command=${words[2]}
    command_position=2
  elif [[ ${invocation} == x ]]; then
    command=x
    command_position=1
  elif [[ ${invocation} == xinit ]]; then
    command=workspace-init
    command_position=1
  else
    command=${invocation#x}
    command_position=1
  fi

  if [[ ${invocation} == ctx && ( -z ${command} || CURRENT == 2 ) ]]; then
    _describe 'ctx command' _ctx_commands
    return
  fi

  if (( CURRENT == command_position + 1 )); then
    case ${command} in
      workspace) _describe 'workspace command' _ctx_workspace_commands ;;
      target) _describe 'target command' _ctx_target_commands ;;
      host) _describe 'host command' _ctx_host_commands ;;
      hosts) _describe 'hosts command' _ctx_hosts_commands ;;
      completion) _describe 'shell' _ctx_completion_shells ;;
      x) _command_names -e ;;
      *) _describe 'option' _ctx_options ;;
    esac
    return
  fi

  case ${command} in
    workspace|target|host|hosts) _message 'argument' ;;
    *) _describe 'option' _ctx_options ;;
  esac
}

_xctx_call() { ctx "${0#x}" "$@" }
xinit() { ctx workspace init "$@" }
xstatus() { ctx status "$@" }
xworkspace() { ctx workspace "$@" }
xtarget() { ctx target "$@" }
xip() { ctx ip "$@" }
xhost() { ctx host "$@" }
xhosts() { ctx hosts "$@" }
xlog() { ctx log "$@" }
x() { ctx x "$@" }
xcompletion() { ctx completion "$@" }
xdoctor() { ctx doctor "$@" }
xinit-shell() { ctx init-shell "$@" }

compdef _ctx ctx
compdef _ctx xinit xstatus xworkspace xtarget xip xhost xhosts xlog x xcompletion xdoctor xinit-shell
`

const bashCompletionScript = `_ctx_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "${prev}" in
    ctx)
      COMPREPLY=($(compgen -W "status workspace target ip host hosts log x completion init-shell doctor -h --help -V --version" -- "${cur}"))
      return
      ;;
    workspace|xworkspace)
      COMPREPLY=($(compgen -W "init ls rm" -- "${cur}"))
      return
      ;;
    target|xtarget)
      COMPREPLY=($(compgen -W "set add update use rm ls" -- "${cur}"))
      return
      ;;
    host|xhost)
      COMPREPLY=($(compgen -W "add rm ls" -- "${cur}"))
      return
      ;;
    hosts|xhosts)
      COMPREPLY=($(compgen -W "show sync clean" -- "${cur}"))
      return
      ;;
    x)
      COMPREPLY=($(compgen -c -- "${cur}"))
      return
      ;;
    completion|xcompletion)
      COMPREPLY=($(compgen -W "zsh bash" -- "${cur}"))
      return
      ;;
  esac
}

xinit() { ctx workspace init "$@"; }
xstatus() { ctx status "$@"; }
xworkspace() { ctx workspace "$@"; }
xtarget() { ctx target "$@"; }
xip() { ctx ip "$@"; }
xhost() { ctx host "$@"; }
xhosts() { ctx hosts "$@"; }
xlog() { ctx log "$@"; }
x() { ctx x "$@"; }
xcompletion() { ctx completion "$@"; }
xdoctor() { ctx doctor "$@"; }
xinit-shell() { ctx init-shell "$@"; }

complete -F _ctx_completion ctx
complete -F _ctx_completion xinit xstatus xworkspace xtarget xip xhost xhosts xlog x xcompletion xdoctor xinit-shell
`
