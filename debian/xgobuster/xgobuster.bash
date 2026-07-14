_xgobuster_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "${prev}" in
    --profile)
      COMPREPLY=($(compgen -W "web-quick web-standard web-deep" -- "${cur}"))
      return
      ;;
    --preset)
      COMPREPLY=($(compgen -W "php wordpress aspnet java node static" -- "${cur}"))
      return
      ;;
    -w|--wordlist|-u|--url|--host|-c|--cookies|--exclude-length)
      COMPREPLY=($(compgen -f -- "${cur}"))
      return
      ;;
  esac

  COMPREPLY=($(compgen -W "-w --wordlist -u --url --host --ip -c --cookies --exclude-length -k --no-tls-validation --tls-verify --preset --profile --status --sitemap --next --force -h --help -V --version" -- "${cur}"))
}

complete -F _xgobuster_completion xgobuster xgo
