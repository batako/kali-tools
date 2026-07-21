_xsteg_completion() {
  local cur prev command
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  command="${COMP_WORDS[1]}"

  if [ "${COMP_CWORD}" -eq 1 ]; then
    COMPREPLY=($(compgen -W "ls scan extract show doctor -h --help -V --version --online-help" -- "${cur}"))
    return
  fi

  if [ "${prev}" = "-w" ] || [ "${prev}" = "--wordlist" ]; then
    COMPREPLY=($(compgen -f -- "${cur}"))
    return
  fi

  case "${command}" in
    ls|scan)
      if [ "${COMP_CWORD}" -eq 2 ]; then
        COMPREPLY=($(compgen -f -- "${cur}"))
      fi
      ;;
    extract)
      if [ "${COMP_CWORD}" -eq 2 ]; then
        COMPREPLY=($(compgen -f -- "${cur}"))
      else
        COMPREPLY=($(compgen -W "--auto --manual -w --wordlist --no-crack" -- "${cur}"))
      fi
      ;;
    show)
      if [ "${COMP_CWORD}" -eq 2 ]; then
        COMPREPLY=($(xsteg ls 2>/dev/null | awk -v prefix="${cur}" 'NR > 1 && index($1, prefix) == 1 {print $1}'))
      elif [ "${COMP_CWORD}" -eq 3 ]; then
        COMPREPLY=($(compgen -f -- "${cur}"))
      fi
      ;;
  esac
}

complete -F _xsteg_completion xsteg
