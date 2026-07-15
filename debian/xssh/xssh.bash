_xssh_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  if [ "${COMP_CWORD}" -eq 1 ]; then
    COMPREPLY=($(compgen -W "key -h --help -V --version" -- "${cur}"))
    return
  fi

  COMPREPLY=($(compgen -W "-h --help -V --version" -- "${cur}"))
}

complete -F _xssh_completion xssh
