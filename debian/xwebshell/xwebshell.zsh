#compdef xwebshell

_xwebshell_ids() {
  local -a ids
  local id label description
  while IFS=$'\t' read -r id label description; do
    [[ -n "${id}" ]] && ids+=("${id}:${label} - ${description}")
  done < <(xwebshell __complete 2>/dev/null)
  _describe -V 'ID' ids
}

_xwebshell_commands() {
  local -a commands
  commands=(
    'ls:list templates and their status'
    'show:show paths and contents for a template'
    'export:copy a template to the current directory'
  )
  _describe -V 'command' commands
}

_xwebshell() {
  _arguments -s \
    '(-h --help)'{-h,--help}'[show this help]' \
    '(-V --version)'{-V,--version}'[show version]' \
    '1:command:_xwebshell_commands' \
    '2:ID:_xwebshell_ids'
}

_xwebshell
