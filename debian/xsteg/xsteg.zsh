#compdef xsteg

_xsteg_commands() {
  local -a commands
  commands=(
    'ls:list saved analysis reports'
    'scan:detect hidden-data candidates without extracting'
    'extract:extract from a saved scan or assume a payload exists'
    'show:show a saved report'
    'doctor:show backend and wordlist availability'
  )
  _describe -V 'command' commands
}

_xsteg_global_options() {
  local -a options
  options=(
    '-h:show help'
    '--help:show help'
    '-V:show version'
    '--version:show version'
    '--online-help:show the versioned online help URL'
  )
  _describe -V 'option' options
}

_xsteg_report_ids() {
  local -a ids
  ids=(${(f)"$(xsteg ls 2>/dev/null | awk 'NR > 1 {print $1 ":" $NF}')"})
  if (( ${#ids} )); then
    _describe -V 'report' ids
  fi
}

_xsteg() {
  if (( CURRENT == 2 )); then
    _xsteg_commands
    _xsteg_global_options
    return
  fi

  case "${words[2]}" in
    ls)
      if (( CURRENT == 3 )); then
        _files
      fi
      ;;
    scan)
      if (( CURRENT == 3 )); then
        _files
      fi
      ;;
    extract)
      _arguments \
        '2:path:_files' \
        '(--manual)--auto[select automatic password analysis without prompting]' \
        '(--auto -w --wordlist --no-crack)--manual[prompt for one known passphrase]' \
        '(--manual -w --wordlist)'{-w,--wordlist}'[use one password wordlist]:wordlist:_files' \
        '(--manual)--no-crack[skip password wordlists]'
      ;;
    show)
      if (( CURRENT == 3 )); then
        _xsteg_report_ids
      elif (( CURRENT == 4 )); then
        _files
      fi
      ;;
  esac
}

_xsteg
