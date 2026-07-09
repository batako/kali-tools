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

const zshCompletionScript = `#compdef ctx xinit xstatus xworkspace xtarget xip xhost xhosts xnote xlog xprompt x xcompletion xdoctor xinit-shell xreset

_ctx_commands=(
  'status:show the current workspace'
  'workspace:initialize, list, or remove workspaces'
  'target:manage targets'
  'ip:show or update the primary target IP'
  'host:manage hostnames'
  'hosts:show, sync, or clean /etc/hosts entries'
  'note:add a note to the workspace timeline'
  'log:show the workspace timeline'
  'prompt:print data for shell prompts'
  'x:run a command and save execution logs'
  'completion:print shell completion script'
  'init-shell:configure shell integration'
  'doctor:check ctx environment'
  'reset:remove all ctx data and configuration'
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

_ctx_log_options=(
  '-p:print a compact timeline'
  '--plain:print a compact timeline'
  '-v:print IDs, status, and exit codes'
  '--verbose:print IDs, status, and exit codes'
  '-i:open the interactive timeline'
  '--interactive:open the interactive timeline'
  '-h:show help'
  '--help:show help'
)

_ctx_prompt_options=(
  '--format:select shell or json output'
  '--field:print one prompt field'
  '-h:show help'
  '--help:show help'
)

_ctx_prompt_formats=(shell json)
_ctx_prompt_fields=(active workspace-id workspace-name workspace-root local-ip local-interface target-name target-ip)

_ctx_reset_options=(
  '--yes:skip confirmation'
  '-h:show help'
  '--help:show help'
)

_ctx_options=(
  '-h:show help'
  '--help:show help'
)

_ctx_dynamic_descriptions() {
  local kind=$1
  local -a values
  values=("${(@f)$(ctx completion descriptions "${kind}" 2>/dev/null)}")
  (( ${#values} )) && _describe "${kind}" values
}

_ctx() {
  local invocation command command_position subcommand previous
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
  subcommand=${words[command_position+1]}
  previous=${words[CURRENT-1]}

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
      log)
        _ctx_dynamic_descriptions log
        _describe 'log option' _ctx_log_options
        ;;
      prompt) _describe 'prompt option' _ctx_prompt_options ;;
      reset) _describe 'reset option' _ctx_reset_options ;;
      x) _command_names -e ;;
      *) _describe 'option' _ctx_options ;;
    esac
    return
  fi

  case ${command} in
    workspace)
      if [[ ${subcommand} == rm ]] && (( CURRENT == command_position + 2 )); then
        _ctx_dynamic_descriptions workspace
      else
        _message 'argument'
      fi
      ;;
    target)
      if [[ ${subcommand} == use || ${subcommand} == rm ]] && (( CURRENT == command_position + 2 )); then
        _ctx_dynamic_descriptions target
      else
        _message 'argument'
      fi
      ;;
    host)
      if [[ ${subcommand} == rm ]] && (( CURRENT == command_position + 2 )); then
        _ctx_dynamic_descriptions host
      elif [[ ${subcommand} == add && ${previous} == --target ]]; then
        _ctx_dynamic_descriptions target
      else
        _message 'argument'
      fi
      ;;
    hosts) _message 'argument' ;;
    prompt)
      case ${words[CURRENT-1]} in
        --format) _describe 'format' _ctx_prompt_formats ;;
        --field) _describe 'field' _ctx_prompt_fields ;;
        *) _describe 'prompt option' _ctx_prompt_options ;;
      esac
      ;;
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
xnote() { ctx note "$@" }
xlog() { ctx log "$@" }
xprompt() { ctx prompt "$@" }
x() { ctx x "$@" }
xcompletion() { ctx completion "$@" }
xdoctor() { ctx doctor "$@" }
xinit-shell() { ctx init-shell "$@" }
xreset() { ctx reset "$@" }

compdef _ctx ctx
compdef _ctx xinit xstatus xworkspace xtarget xip xhost xhosts xnote xlog xprompt x xcompletion xdoctor xinit-shell xreset
`

const bashCompletionScript = `_ctx_complete_values() {
  local kind=$1
  local prefix=$2
  local value
  while IFS= read -r value; do
    [[ ${value} == "${prefix}"* ]] && COMPREPLY+=("${value}")
  done < <(ctx completion values "${kind}" 2>/dev/null)
}

_ctx_completion() {
  local cur prev invocation command subcommand
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  invocation="${COMP_WORDS[0]##*/}"

  if [[ ${invocation} == ctx ]]; then
    command="${COMP_WORDS[1]}"
    subcommand="${COMP_WORDS[2]}"
  elif [[ ${invocation} == x ]]; then
    command=x
  else
    command="${invocation#x}"
    subcommand="${COMP_WORDS[1]}"
  fi

  if [[ ${command} == target && (${subcommand} == use || ${subcommand} == rm) && ${prev} == "${subcommand}" ]]; then
    _ctx_complete_values target "${cur}"
    return
  fi
  if [[ ${command} == host && ${subcommand} == rm && ${prev} == rm ]]; then
    _ctx_complete_values host "${cur}"
    return
  fi
  if [[ ${command} == host && ${subcommand} == add && ${prev} == --target ]]; then
    _ctx_complete_values target "${cur}"
    return
  fi
  if [[ ${command} == workspace && ${subcommand} == rm && ${prev} == rm ]]; then
    _ctx_complete_values workspace "${cur}"
    return
  fi

  case "${prev}" in
    ctx)
      COMPREPLY=($(compgen -W "status workspace target ip host hosts note log prompt x completion init-shell doctor reset -h --help -V --version" -- "${cur}"))
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
    log|xlog)
      _ctx_complete_values log "${cur}"
      local -a log_options
      log_options=($(compgen -W "-p --plain -v --verbose -i --interactive -h --help" -- "${cur}"))
      COMPREPLY+=("${log_options[@]}")
      return
      ;;
    prompt|xprompt)
      COMPREPLY=($(compgen -W "--format --field -h --help" -- "${cur}"))
      return
      ;;
    reset|xreset)
      COMPREPLY=($(compgen -W "--yes -h --help" -- "${cur}"))
      return
      ;;
    --format)
      COMPREPLY=($(compgen -W "shell json" -- "${cur}"))
      return
      ;;
    --field)
      COMPREPLY=($(compgen -W "active workspace-id workspace-name workspace-root local-ip local-interface target-name target-ip" -- "${cur}"))
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
xnote() { ctx note "$@"; }
xlog() { ctx log "$@"; }
xprompt() { ctx prompt "$@"; }
x() { ctx x "$@"; }
xcompletion() { ctx completion "$@"; }
xdoctor() { ctx doctor "$@"; }
xinit-shell() { ctx init-shell "$@"; }
xreset() { ctx reset "$@"; }

complete -F _ctx_completion ctx
complete -F _ctx_completion xinit xstatus xworkspace xtarget xip xhost xhosts xnote xlog xprompt x xcompletion xdoctor xinit-shell xreset
`
