#compdef xdec

_xdec_commands() {
  local -a commands
  commands=(
    'decode:decode values and detect recoverable inputs'
    'recover:recover passwords and key passphrases'
    'rot:apply Caesar/ROT shifts'
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

  if [[ "${words[2]}" == "recover" ]]; then
    _arguments -s \
      '1:subcommand:(recover)' \
      '!-f[read FILE as input]:file:_files' \
      '!--file[read FILE as input]:file:_files' \
      '!--string[treat VALUE as a string]:value:' \
      '!-w[use a ctx wordlist or path]:wordlist:_files' \
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
    return
  fi

  _arguments -s \
    '1:subcommand:(decode)' \
    '!-f[read FILE as input]:file:_files' \
    '!--file[read FILE as input]:file:_files' \
    '!--string[treat VALUE as a string]:value:' \
    '2:input:_files'
}

_xdec_help() {
  if (( CURRENT == 3 )); then
    local -a targets
    targets=(
      'decode:show decode help'
      'recover:show recover help'
      'rot:show rot help'
      'help:show help command help'
      'version:show version command help'
    )
    _describe -V 'subcommand' targets
  fi
}

_xdec_rot() {
  case "${words[CURRENT-1]}" in
    -f|--file)
      _files
      return
      ;;
    --string|--shift|-n)
      return
      ;;
  esac

  _arguments -s \
    '!-f[read FILE as input]:file:_files' \
    '!--file[read FILE as input]:file:_files' \
    '!--string[treat VALUE as a string]:value:' \
    '--shift[apply one shift or a range]:shift:' \
    '2:input:_files'
}

_xdec() {
  if (( CURRENT == 2 )); then
    if [[ "${words[CURRENT]}" == */* ]]; then
      _files
      return
    fi
    _xdec_commands
    _xdec_global_options
    return
  fi

  case "${words[2]}" in
    decode)
      _xdec_decode
      ;;
    recover)
      _xdec_decode
      ;;
    rot)
      _xdec_rot
      ;;
    help)
      _xdec_help
      ;;
  esac
}

_xdec
