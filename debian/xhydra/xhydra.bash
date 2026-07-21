_xhydra_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "${prev}" in
    xhydra)
      COMPREPLY=($(compgen -W "http ssh ftp smb -h --help -V --version" -- "${cur}"))
      return
      ;;
    http)
      COMPREPLY=($(compgen -W "-u --username -r --request --url --data --user-field --password-field --fail-json --success-json --fail-body --success-body --fail-status --success-redirect -P --password-list -h --help -V --version" -- "${cur}"))
      return
      ;;
    ssh)
      COMPREPLY=($(compgen -W "-u --username --password -L --user-list --host -p --port --service -t --tasks --status --clear-cache -P --password-list -h --help -V --version" -- "${cur}"))
      return
      ;;
    ftp)
      COMPREPLY=($(compgen -W "-u --username --password -L --user-list --host -p --port --service -t --tasks --status --clear-cache -P --password-list -h --help -V --version" -- "${cur}"))
      return
      ;;
    smb)
      COMPREPLY=($(compgen -W "-u --username --password -L --user-list --host -p --port --service -t --tasks --status --clear-cache -P --password-list -h --help -V --version" -- "${cur}"))
      return
      ;;
    -u|--username|--password|-L|--user-list|-r|--request|--url|--data|--user-field|--password-field|--fail-json|--success-json|--fail-body|--success-body|--fail-status|-P|--password-list)
      COMPREPLY=($(compgen -f -- "${cur}"))
      return
      ;;
    --host|-p|--port|--service)
      COMPREPLY=($(compgen -f -- "${cur}"))
      return
      ;;
    -t|--tasks)
      COMPREPLY=()
      return
      ;;
  esac

  case "${COMP_WORDS[1]}" in
    http) COMPREPLY=($(compgen -W "-u --username -r --request --url --data --user-field --password-field --fail-json --success-json --fail-body --success-body --fail-status --success-redirect -P --password-list -h --help -V --version" -- "${cur}")) ;;
    ssh) COMPREPLY=($(compgen -W "-u --username --password -L --user-list --host -p --port --service -t --tasks --status --clear-cache -P --password-list -h --help -V --version" -- "${cur}")) ;;
    ftp) COMPREPLY=($(compgen -W "-u --username --password -L --user-list --host -p --port --service -t --tasks --status --clear-cache -P --password-list -h --help -V --version" -- "${cur}")) ;;
    smb) COMPREPLY=($(compgen -W "-u --username --password -L --user-list --host -p --port --service -t --tasks --status --clear-cache -P --password-list -h --help -V --version" -- "${cur}")) ;;
    *) COMPREPLY=($(compgen -W "-u --username --password -L --user-list --host -p --port --service --status --clear-cache -P --password-list -h --help -V --version" -- "${cur}")) ;;
  esac
}

complete -F _xhydra_completion xhydra
