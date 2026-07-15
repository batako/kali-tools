_xhydra_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "${prev}" in
    xhydra)
      COMPREPLY=($(compgen -W "http -h --help -V --version" -- "${cur}"))
      return
      ;;
    http)
      COMPREPLY=($(compgen -W "-u --username -r --request --url --data --user-field --password-field --fail-json --success-json --fail-body --success-body --fail-status --success-redirect -P --password-list -h --help -V --version" -- "${cur}"))
      return
      ;;
    -u|--username|-r|--request|--url|--data|--user-field|--password-field|--fail-json|-P|--password-list)
      COMPREPLY=($(compgen -f -- "${cur}"))
      return
      ;;
  esac

  COMPREPLY=($(compgen -W "-u --username -r --request --url --data --user-field --password-field --fail-json --success-json --fail-body --success-body --fail-status --success-redirect -P --password-list -h --help -V --version" -- "${cur}"))
}

complete -F _xhydra_completion xhydra
