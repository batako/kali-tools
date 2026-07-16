_xscp_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "${prev}" in
    -p|--port|--service)
      COMPREPLY=($(compgen -W "1 2 3 4 5 6 7 8 9 10 22 2222 2200" -- "${cur}"))
      return
      ;;
  esac

  case "${COMP_CWORD}" in
    1)
      COMPREPLY=($(compgen -W "upload download -h --help -V --version" -- "${cur}"))
      return
      ;;
    2|3)
      COMPREPLY=($(compgen -f -- "${cur}"))
      return
      ;;
  esac

  COMPREPLY=($(compgen -W "-p --port --service -h --help -V --version" -- "${cur}"))
}

complete -F _xscp_completion xscp
