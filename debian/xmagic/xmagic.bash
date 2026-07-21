_xmagic_completion() {
  local cur
  cur="${COMP_WORDS[COMP_CWORD]}"

  if [ "${COMP_CWORD}" -eq 1 ]; then
    COMPREPLY=($(compgen -W "ls set -h --help -V --version --online-help" -- "${cur}"))
    return
  fi

  if [ "${COMP_WORDS[1]}" = "set" ]; then
    if [ "${COMP_CWORD}" -eq 2 ]; then
      COMPREPLY=($(compgen -W "gif jpg jpeg png pdf zip" -- "${cur}"))
      return
    fi
    if [ "${COMP_CWORD}" -eq 3 ]; then
      COMPREPLY=($(compgen -f -- "${cur}"))
      return
    fi
  fi

  COMPREPLY=()
}

complete -F _xmagic_completion xmagic
