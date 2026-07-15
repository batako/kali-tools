_xwebshell_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "${prev}" in
    show|export)
      local ids
      ids="$(xwebshell ls 2>/dev/null | awk '$1 ~ /^[0-9]+$/ {print $1}')"
      COMPREPLY=($(compgen -W "${ids}" -- "${cur}"))
      return
      ;;
  esac

  COMPREPLY=($(compgen -W "ls show export -h --help -V --version" -- "${cur}"))
}

complete -F _xwebshell_completion xwebshell
