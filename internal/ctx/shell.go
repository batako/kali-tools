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

type CompletionOptions struct {
	ExtraShortcuts bool
}

type ShellIntegrationOptions struct {
	ExtraShortcuts bool
}

var parentProcessNameFunc = parentProcessName

func CompletionScript(shell string, options ...CompletionOptions) (string, error) {
	completionOptions := CompletionOptions{}
	if len(options) > 0 {
		completionOptions = options[0]
	}

	switch shell {
	case "zsh":
		script := zshCompletionScript
		if completionOptions.ExtraShortcuts {
			var err error
			script, err = enableExtraShortcutsInZshCompletionScript(script)
			if err != nil {
				return "", err
			}
		}
		return script, nil
	case "bash":
		script := bashCompletionScript
		if completionOptions.ExtraShortcuts {
			var err error
			script, err = enableExtraShortcutsInBashCompletionScript(script)
			if err != nil {
				return "", err
			}
		}
		return script, nil
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

func InstallShellConfig(options ...ShellIntegrationOptions) (ShellConfig, bool, error) {
	shellOptions := ShellIntegrationOptions{}
	if len(options) > 0 {
		shellOptions = options[0]
	}

	config, err := DetectShell()
	if err != nil {
		return ShellConfig{}, false, err
	}

	content, err := os.ReadFile(config.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ShellConfig{}, false, fmt.Errorf("failed to read %s: %w", config.Path, err)
	}

	text := string(content)
	block := shellBlockWithOptions(config.Shell, shellOptions.ExtraShortcuts)
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
	return shellBlockWithOptions(shell, false)
}

func shellBlockWithOptions(shell string, includeExtraShortcuts bool) string {
	command := "ctx completion " + shell
	if includeExtraShortcuts {
		command += " --extra-shortcuts"
	}
	return shellBlockStart + "\nsource <(" + command + ")\n" + shellBlockEnd + "\n"
}

type scriptReplacement struct {
	old string
	new string
}

func applyScriptReplacements(script string, replacements []scriptReplacement) (string, error) {
	for _, replacement := range replacements {
		if !strings.Contains(script, replacement.old) {
			return "", errors.New("failed to enable extra shortcuts: completion script anchor not found")
		}
		script = strings.Replace(script, replacement.old, replacement.new, 1)
	}
	return script, nil
}

func enableExtraShortcutsInZshCompletionScript(script string) (string, error) {
	return applyScriptReplacements(script, []scriptReplacement{
		{
			old: `  elif [[ ${invocation} == xinit ]]; then
    command=workspace-init
    command_position=1
  else
    command=${invocation#x}
    command_position=1
  fi`,
			new: `  elif [[ ${invocation} == xinit ]]; then
    command=workspace-init
    command_position=1
  elif [[ ${invocation} == pj ]]; then
    command=project
    command_position=1
  elif [[ ${invocation} == ta ]]; then
    command=target
    command_position=1
  elif [[ ${invocation} == cr ]]; then
    command=credential
    command_position=1
  elif [[ ${invocation} == sv ]]; then
    command=service
    command_position=1
  else
    command=${invocation#x}
    command_position=1
  fi`,
		},
		{
			old: `xreset() { ctx reset "$@" }

compdef _ctx ctx
compdef _ctx xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset
`,
			new: `xreset() { ctx reset "$@" }
pj() { xproject "$@" }
ta() { xtarget "$@" }
unalias cr 2>/dev/null
cr() { xcredential "$@" }
sv() { xservice "$@" }

compdef _ctx ctx
compdef _ctx xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset pj ta cr sv
`,
		},
	})
}

func enableExtraShortcutsInBashCompletionScript(script string) (string, error) {
	return applyScriptReplacements(script, []scriptReplacement{
		{
			old: `  elif [[ ${invocation} == x ]]; then
    command=x
  else
    command="${invocation#x}"
    subcommand="${COMP_WORDS[1]}"
    root_command="${COMP_WORDS[2]}"
    root_move_source_position=3
  fi`,
			new: `  elif [[ ${invocation} == x ]]; then
    command=x
  elif [[ ${invocation} == pj ]]; then
    command=project
    subcommand="${COMP_WORDS[1]}"
    root_command="${COMP_WORDS[2]}"
    root_move_source_position=3
  elif [[ ${invocation} == ta ]]; then
    command=target
    subcommand="${COMP_WORDS[1]}"
  elif [[ ${invocation} == cr ]]; then
    command=credential
    subcommand="${COMP_WORDS[1]}"
  elif [[ ${invocation} == sv ]]; then
    command=service
    subcommand="${COMP_WORDS[1]}"
  else
    command="${invocation#x}"
    subcommand="${COMP_WORDS[1]}"
    root_command="${COMP_WORDS[2]}"
    root_move_source_position=3
  fi`,
		},
		{
			old: `    project|xproject)
      COMPREPLY=($(compgen -W "root new ls rm" -- "${cur}"))
      return
      ;;
    root)
      if [[ ${command} == project ]]; then
        COMPREPLY=($(compgen -W "add use ls rm move" -- "${cur}"))
        return
      fi
      ;;
    target|xtarget)
      COMPREPLY=($(compgen -W "set add update use rm ls" -- "${cur}"))
      return
      ;;
`,
			new: `    project|xproject|pj)
      COMPREPLY=($(compgen -W "root new ls rm" -- "${cur}"))
      return
      ;;
    root)
      if [[ ${command} == project ]]; then
        COMPREPLY=($(compgen -W "add use ls rm move" -- "${cur}"))
        return
      fi
      ;;
    target|xtarget|ta)
      COMPREPLY=($(compgen -W "set add update use rm ls" -- "${cur}"))
      return
      ;;
`,
		},
		{
			old: `    credential|xcredential)
      COMPREPLY=($(compgen -W "ls set add update rm -h --help" -- "${cur}"))
      return
      ;;
`,
			new: `    credential|xcredential|cr)
      COMPREPLY=($(compgen -W "ls set add update rm -h --help" -- "${cur}"))
				return
				;;
`,
		},
		{
			old: `    service|xservice)
      COMPREPLY=($(compgen -W "ls -h --help" -- "${cur}"))
      return
      ;;
`,
			new: `    service|xservice|sv)
      COMPREPLY=($(compgen -W "ls -h --help" -- "${cur}"))
      return
      ;;
`,
		},
		{
			old: `xreset() { ctx reset "$@"; }

complete -F _ctx_completion ctx
complete -F _ctx_completion xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset
`,
			new: `xreset() { ctx reset "$@"; }
pj() { xproject "$@"; }
ta() { xtarget "$@"; }
unalias cr 2>/dev/null
cr() { xcredential "$@"; }
sv() { xservice "$@"; }

complete -F _ctx_completion ctx
complete -F _ctx_completion xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset pj ta cr sv
`,
		},
	})
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

const zshCompletionScript = `#compdef ctx xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset

_ctx_commands=(
  'status:show the current workspace'
  'config:get or set ctx configuration'
  'workspace:initialize, list, or remove workspaces'
  'project:create and manage projects under the configured root'
  'target:manage targets'
  'ip:show or update the primary target IP'
  'host:manage hostnames'
  'hosts:show, sync, or clean /etc/hosts entries'
  'scan:run nmap and save service information'
  'service:show saved service information'
  'credential:manage stored credentials'
  'note:add a note to the workspace timeline'
  'log:show the workspace timeline'
  'prompt:print shell prompt data'
  'formats:list supported JSON outputs and format versions'
  'x:run a command and save execution logs'
  'completion:print shell completion script'
  'init-shell:configure shell integration'
  'doctor:check ctx environment'
  'reset:remove all ctx data and configuration'
)

_ctx_config_commands=(
  'ls:list configuration values'
  'get:print a configuration value'
  'set:set a configuration value'
)

_ctx_config_keys=(
  'project.root:project root directory'
  'web.directory.max-requests:maximum directory requests per automatic run'
  'web.file.max-requests:maximum file requests per automatic run'
  'web.vhost.max-requests:maximum vhost requests per automatic run'
  'web.vhost.calibration-samples:number of vhost calibration requests'
  'web.vhost.calibration-confidence:minimum vhost calibration confidence percentage'
  'password.max-requests:maximum password requests per automatic run'
  'dns.max-queries:maximum DNS queries per automatic run'
  'web.tls.verify:verify TLS certificates for web requests'
)

_ctx_workspace_commands=(
  'init:create a workspace in the current directory'
  'ls:list workspaces'
  'rm:remove a workspace and all of its ctx data'
)

_ctx_project_commands=(
  'root:manage project roots'
  'new:create a project and initialize a workspace'
  'ls:list projects'
  'rm:remove a project directory'
)

_ctx_project_root_commands=(
  'add:register a project root, deriving its name from the path by default'
  'use:switch the active project root'
  'ls:list registered project roots'
  'rm:unregister an inactive project root'
  'move:move all ctx projects between registered roots'
)

_ctx_project_root_options=(
  '--name:override the name derived from the path'
)

_ctx_project_root_move_options=(
  '--dry-run:show the move plan without changing files or data'
  '-y:skip move confirmation'
  '--yes:skip move confirmation'
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

_ctx_scan_options=(
  '-p:pass an explicit port list/range to nmap'
  '--ports:pass an explicit port list/range to nmap'
  '-n:print the nmap command without running it'
  '--dry-run:print the nmap command without running it'
  '-f:run even if the same scan already succeeded'
  '--force:run even if the same scan already succeeded'
  '-h:show help'
  '--help:show help'
)

_ctx_service_commands=(
  'ls:list saved ports and services'
)

_ctx_service_options=(
  '--target:select a target by name'
  '--format:select shell or json output'
  '--format-version:select JSON format version'
  '-h:show help'
  '--help:show help'
)

_ctx_credential_commands=(
  'ls:list credentials'
  'set:create or update a credential'
  'add:add a credential'
  'update:update an existing credential'
  'rm:remove a credential'
)

_ctx_credential_options=(
  '--format:select shell or json output'
  '--format-version:select JSON format version'
  '-y:skip confirmation'
  '--yes:skip confirmation'
  '-h:show help'
  '--help:show help'
)

_ctx_completion_shells=(
  'zsh:print zsh completion script'
  'bash:print bash completion script'
)

_ctx_completion_options=(
  '--extra-shortcuts:include pj, ta, and cr shortcuts'
  '-h:show help'
  '--help:show help'
)

_ctx_init_shell_options=(
  '--extra-shortcuts:include pj, ta, and cr shortcuts'
  '--remove:remove ctx shell integration'
  '-h:show help'
  '--help:show help'
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
  '--format-version:select JSON format version'
  '--field:print one prompt field'
  '-h:show help'
  '--help:show help'
)

_ctx_prompt_formats=(shell json)
_ctx_prompt_fields=(active workspace-id workspace-name workspace-path local-ip local-interface target-name target-ip)
_ctx_format_versions=(1 1.0)

_ctx_reset_options=(
  '-y:skip confirmation'
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
  local invocation command command_position subcommand previous root_command
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
  root_command=${words[command_position+2]}
  previous=${words[CURRENT-1]}

  if [[ ${invocation} == ctx && ( -z ${command} || CURRENT == 2 ) ]]; then
    _describe 'ctx command' _ctx_commands
    return
  fi

  if (( CURRENT == command_position + 1 )); then
    case ${command} in
      config) _describe 'config command' _ctx_config_commands ;;
      workspace) _describe 'workspace command' _ctx_workspace_commands ;;
      project) _describe 'project command' _ctx_project_commands ;;
      target) _describe 'target command' _ctx_target_commands ;;
      host) _describe 'host command' _ctx_host_commands ;;
      hosts) _describe 'hosts command' _ctx_hosts_commands ;;
      completion) _describe 'shell' _ctx_completion_shells ;;
      scan) _describe 'scan option' _ctx_scan_options ;;
      service) _describe 'service command' _ctx_service_commands ;;
      credential) _describe 'credential command' _ctx_credential_commands ;;
      log)
        _ctx_dynamic_descriptions log
        _describe 'log option' _ctx_log_options
        ;;
      prompt) _describe 'prompt option' _ctx_prompt_options ;;
      formats) _describe 'formats option' _ctx_prompt_options ;;
      reset) _describe 'reset option' _ctx_reset_options ;;
      init-shell) _describe 'init-shell option' _ctx_init_shell_options ;;
      x) _command_names -e ;;
      *) _describe 'option' _ctx_options ;;
    esac
    return
  fi

  case ${command} in
    config)
      if [[ ${subcommand} == get || ${subcommand} == set ]] && (( CURRENT == command_position + 2 )); then
        _describe 'config key' _ctx_config_keys
      else
        _message 'argument'
      fi
      ;;
    workspace)
      if [[ ${subcommand} == rm ]] && (( CURRENT == command_position + 2 )); then
        _ctx_dynamic_descriptions workspace
      else
        _message 'argument'
      fi
      ;;
    completion)
      if [[ ${previous} == zsh || ${previous} == bash ]]; then
        _describe 'completion option' _ctx_completion_options
      else
        _describe 'shell' _ctx_completion_shells
      fi
      ;;
    project)
      if [[ ${subcommand} == root ]] && (( CURRENT == command_position + 2 )); then
        _describe 'project root command' _ctx_project_root_commands
      elif [[ ${subcommand} == root && (${root_command} == use || ${root_command} == rm) ]] && (( CURRENT == command_position + 3 )); then
        _ctx_dynamic_descriptions project-root
      elif [[ ${subcommand} == root && ${root_command} == move ]] && (( CURRENT == command_position + 3 || CURRENT == command_position + 4 )); then
        _ctx_dynamic_descriptions project-root
      elif [[ ${subcommand} == root && ${root_command} == move ]]; then
        _describe 'project root move option' _ctx_project_root_move_options
      elif [[ ${subcommand} == root && ${root_command} == add ]] && (( CURRENT == command_position + 3 )); then
        _directories
      elif [[ ${subcommand} == root && ${root_command} == add && ${previous} == --name ]]; then
        _message 'project root name'
      elif [[ ${subcommand} == root && ${root_command} == add ]]; then
        _describe 'project root option' _ctx_project_root_options
      elif [[ ${subcommand} == rm ]] && (( CURRENT == command_position + 2 )); then
        _ctx_dynamic_descriptions project
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
      elif [[ ${previous} == --target ]]; then
        _ctx_dynamic_descriptions target
      else
        _message 'argument'
      fi
      ;;
    hosts) _message 'argument' ;;
    scan)
      case ${words[CURRENT-1]} in
        -p|--ports) _message 'ports' ;;
        *) _describe 'scan option' _ctx_scan_options ;;
      esac
      ;;
    service)
      if [[ ${subcommand} == ls && ${previous} == --target ]]; then
        _ctx_dynamic_descriptions target
      elif [[ ${previous} == --format ]]; then
        _describe 'format' _ctx_prompt_formats
      elif [[ ${previous} == --format-version ]]; then
        _describe 'format version' _ctx_format_versions
      else
        _describe 'service option' _ctx_service_options
      fi
      ;;
    credential)
      if [[ ${previous} == --format ]]; then
        _describe 'format' _ctx_prompt_formats
      elif [[ ${previous} == --format-version ]]; then
        _describe 'format version' _ctx_format_versions
      elif [[ ${subcommand} == rm || ${subcommand} == ls ]]; then
        _describe 'credential option' _ctx_credential_options
      else
        _message 'argument'
      fi
      ;;
    prompt)
      case ${words[CURRENT-1]} in
        --format) _describe 'format' _ctx_prompt_formats ;;
        --format-version) _describe 'format version' _ctx_format_versions ;;
        --field) _describe 'field' _ctx_prompt_fields ;;
        *) _describe 'prompt option' _ctx_prompt_options ;;
      esac
      ;;
    formats)
      case ${words[CURRENT-1]} in
        --format) _describe 'format' _ctx_prompt_formats ;;
        --format-version) _describe 'format version' _ctx_format_versions ;;
        *) _describe 'formats option' _ctx_prompt_options ;;
      esac
      ;;
    *) _describe 'option' _ctx_options ;;
  esac
}

_xctx_call() { ctx "${0#x}" "$@" }
xinit() { ctx workspace init "$@" }
xstatus() { ctx status "$@" }
xconfig() { ctx config "$@" }
xworkspace() { ctx workspace "$@" }
xproject() {
  if [[ $# == 0 || $1 == root || $1 == ls || $1 == rm || $1 == -h || $1 == --help ]]; then
    ctx project "$@"
    return
  fi
  local project_path
  project_path=$(ctx project "$@") || return
  [[ -n ${project_path} ]] && cd "${project_path}"
}
xnew() {
  local project_path
  project_path=$(ctx project new "$@") || return
  [[ -n ${project_path} ]] && cd "${project_path}"
}
xtarget() { ctx target "$@" }
xip() { ctx ip "$@" }
xhost() { ctx host "$@" }
xhosts() { ctx hosts "$@" }
xscan() { CTX_INVOKED_AS=xscan ctx scan "$@" }
xservice() { ctx service "$@" }
xcredential() { ctx credential "$@" }
xnote() { ctx note "$@" }
xlog() { ctx log "$@" }
xprompt() { ctx prompt "$@" }
xformats() { ctx formats "$@" }
x() { ctx x "$@" }
xcompletion() { ctx completion "$@" }
xdoctor() { ctx doctor "$@" }
xinit-shell() { ctx init-shell "$@" }
xreset() { ctx reset "$@" }

compdef _ctx ctx
compdef _ctx xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset
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
  local cur prev invocation command subcommand root_command root_move_source_position
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  invocation="${COMP_WORDS[0]##*/}"

  if [[ ${invocation} == ctx ]]; then
    command="${COMP_WORDS[1]}"
    subcommand="${COMP_WORDS[2]}"
    root_command="${COMP_WORDS[3]}"
    root_move_source_position=4
  elif [[ ${invocation} == x ]]; then
    command=x
  else
    command="${invocation#x}"
    subcommand="${COMP_WORDS[1]}"
    root_command="${COMP_WORDS[2]}"
    root_move_source_position=3
  fi

  if [[ ${command} == project && ${subcommand} == root && (${root_command} == use || ${root_command} == rm) && ${prev} == "${root_command}" ]]; then
    _ctx_complete_values project-root "${cur}"
    return
  fi
  if [[ ${command} == project && ${subcommand} == root && ${root_command} == move ]]; then
    if (( COMP_CWORD == root_move_source_position || COMP_CWORD == root_move_source_position + 1 )); then
      _ctx_complete_values project-root "${cur}"
    else
      COMPREPLY=($(compgen -W "--dry-run -y --yes" -- "${cur}"))
    fi
    return
  fi
  if [[ ${command} == project && ${subcommand} == root && ${root_command} == add ]]; then
    if [[ ${prev} == add ]]; then
      COMPREPLY=($(compgen -d -- "${cur}"))
    elif [[ ${prev} != --name ]]; then
      COMPREPLY=($(compgen -W "--name" -- "${cur}"))
    fi
    return
  fi

  if [[ ${command} == target && (${subcommand} == use || ${subcommand} == rm) && ${prev} == "${subcommand}" ]]; then
    _ctx_complete_values target "${cur}"
    return
  fi
  if [[ ${command} == host && ${subcommand} == rm && ${prev} == rm ]]; then
    _ctx_complete_values host "${cur}"
    return
  fi
  if [[ ${command} == host && ${prev} == --target ]]; then
    _ctx_complete_values target "${cur}"
    return
  fi
  if [[ ${command} == workspace && ${subcommand} == rm && ${prev} == rm ]]; then
    _ctx_complete_values workspace "${cur}"
    return
  fi
  if [[ ${command} == project && ${subcommand} == rm && ${prev} == rm ]]; then
    _ctx_complete_values project "${cur}"
    return
  fi
  if [[ ${command} == service && ${subcommand} == ls && ${prev} == --target ]]; then
    _ctx_complete_values target "${cur}"
    return
  fi
  case "${prev}" in
    ctx)
      COMPREPLY=($(compgen -W "status config workspace project target ip host hosts scan service credential note log prompt formats x completion init-shell doctor reset -h --help -V --version" -- "${cur}"))
      return
      ;;
    workspace|xworkspace)
      COMPREPLY=($(compgen -W "init ls rm" -- "${cur}"))
      return
      ;;
    config|xconfig)
      COMPREPLY=($(compgen -W "ls get set -h --help" -- "${cur}"))
      return
      ;;
    get|set)
      if [[ ${command} == config ]]; then
        COMPREPLY=($(compgen -W "project.root web.directory.max-requests web.file.max-requests web.vhost.max-requests web.vhost.calibration-samples web.vhost.calibration-confidence password.max-requests dns.max-queries web.tls.verify" -- "${cur}"))
        return
      fi
      ;;
    project|xproject)
      COMPREPLY=($(compgen -W "root new ls rm" -- "${cur}"))
      return
      ;;
    root)
      if [[ ${command} == project ]]; then
        COMPREPLY=($(compgen -W "add use ls rm move" -- "${cur}"))
        return
      fi
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
    scan|xscan)
      COMPREPLY=($(compgen -W "-p --ports -n --dry-run -f --force -h --help" -- "${cur}"))
      return
      ;;
    service|xservice)
      COMPREPLY=($(compgen -W "ls -h --help" -- "${cur}"))
      return
      ;;
    credential|xcredential)
      COMPREPLY=($(compgen -W "ls set add update rm -h --help" -- "${cur}"))
      return
      ;;
    ls)
      if [[ ${command} == service ]]; then
        COMPREPLY=($(compgen -W "--target --format --format-version -h --help" -- "${cur}"))
        return
      fi
      if [[ ${command} == credential ]]; then
        COMPREPLY=($(compgen -W "--format --format-version -h --help" -- "${cur}"))
        return
      fi
      ;;
    log|xlog)
      _ctx_complete_values log "${cur}"
      local -a log_options
      log_options=($(compgen -W "-p --plain -v --verbose -i --interactive -h --help" -- "${cur}"))
      COMPREPLY+=("${log_options[@]}")
      return
      ;;
    prompt|xprompt)
      COMPREPLY=($(compgen -W "--format --format-version --field -h --help" -- "${cur}"))
      return
      ;;
    formats|xformats)
      COMPREPLY=($(compgen -W "--format --format-version -h --help" -- "${cur}"))
      return
      ;;
    reset|xreset)
      COMPREPLY=($(compgen -W "-y --yes -h --help" -- "${cur}"))
      return
      ;;
    init-shell|xinit-shell)
      COMPREPLY=($(compgen -W "--extra-shortcuts --remove -h --help" -- "${cur}"))
      return
      ;;
    --format)
      COMPREPLY=($(compgen -W "shell json" -- "${cur}"))
      return
      ;;
    --format-version)
      COMPREPLY=($(compgen -W "1 1.0" -- "${cur}"))
      return
      ;;
    --field)
      COMPREPLY=($(compgen -W "active workspace-id workspace-name workspace-path local-ip local-interface target-name target-ip" -- "${cur}"))
      return
      ;;
    x)
      COMPREPLY=($(compgen -c -- "${cur}"))
      return
      ;;
    completion|xcompletion)
      if [[ ${cur} == -* ]]; then
        COMPREPLY=($(compgen -W "--extra-shortcuts -h --help" -- "${cur}"))
      else
        COMPREPLY=($(compgen -W "zsh bash" -- "${cur}"))
      fi
      return
      ;;
  esac
}

xinit() { ctx workspace init "$@"; }
xstatus() { ctx status "$@"; }
xconfig() { ctx config "$@"; }
xworkspace() { ctx workspace "$@"; }
xproject() {
  if [[ $# == 0 || $1 == root || $1 == ls || $1 == rm || $1 == -h || $1 == --help ]]; then
    ctx project "$@"
    return
  fi
  local project_path
  project_path=$(ctx project "$@") || return
  [[ -n ${project_path} ]] && cd "${project_path}"
}
xnew() {
  local project_path
  project_path=$(ctx project new "$@") || return
  [[ -n ${project_path} ]] && cd "${project_path}"
}
xtarget() { ctx target "$@"; }
xip() { ctx ip "$@"; }
xhost() { ctx host "$@"; }
xhosts() { ctx hosts "$@"; }
xscan() { CTX_INVOKED_AS=xscan ctx scan "$@"; }
xservice() { ctx service "$@"; }
xcredential() { ctx credential "$@"; }
xnote() { ctx note "$@"; }
xlog() { ctx log "$@"; }
xprompt() { ctx prompt "$@"; }
xformats() { ctx formats "$@"; }
x() { ctx x "$@"; }
xcompletion() { ctx completion "$@"; }
xdoctor() { ctx doctor "$@"; }
xinit-shell() { ctx init-shell "$@"; }
xreset() { ctx reset "$@"; }

complete -F _ctx_completion ctx
complete -F _ctx_completion xinit xstatus xconfig xworkspace xproject xnew xtarget xip xhost xhosts xscan xservice xcredential xnote xlog xprompt xformats x xcompletion xdoctor xinit-shell xreset
`
