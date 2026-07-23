#compdef xdec

_xdec_commands() {
  local -a commands
  commands=(
    'decode:decode values and recover passwords'
    'help:show help for the root or a subcommand'
    'version:show version'
  )
  _describe -V 'subcommand' commands
}

_xdec_global_options() {
  local -a options
  options=(
    '--online-help:show the versioned online help URL'
  )
  _describe -V 'option' options
}

_xdec_decode() {
  case "${words[CURRENT-1]}" in
    -f|--file|-w|--wordlist)
      _files
      return
      ;;
    --string|--scope|--username)
      return
      ;;
  esac

  _arguments -s \
    '1:subcommand:(decode)' \
    '!-f[read FILE as input]:file:_files' \
    '!--file[read FILE as input]:file:_files' \
    '!--string[treat VALUE as a string]:value:' \
    '!-w[use a ctx wordlist or path]:wordlist:_files' \
    '!-h' \
    '--wordlist[use a ctx wordlist or path]:wordlist:_files' \
    '--scope[credential scope]:scope:' \
    '--username[credential username]:username:' \
    '--save-credential[save a verified user password to ctx]' \
    '--no-save-credential[never save credentials]' \
    '--yes[approve expensive recovery]' \
    '--refresh[discard saved state for this input and analyze again]' \
    '--dry-run[show execution plan]' \
    '--json[emit JSON]' \
    '2:input:_files'
}

_xdec_help() {
  if (( CURRENT == 3 )); then
    local -a targets
    targets=(
      'decode:show decode help'
      'help:show help command help'
      'version:show version command help'
    )
    _describe -V 'subcommand' targets
  fi
}

_xdec() {
  if (( CURRENT == 2 )); then
    _xdec_commands
    _xdec_global_options
    return
  fi

  case "${words[2]}" in
    decode)
      _xdec_decode
      ;;
    help)
      _xdec_help
      ;;
  esac
}

_xdec
