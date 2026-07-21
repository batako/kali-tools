_xgobuster_completion() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "${prev}" in
    -d|--domain)
      COMPREPLY=($(compgen -W "example.com" -- "${cur}"))
      return
      ;;
    --preset)
      COMPREPLY=($(compgen -W "php wordpress aspnet java node static" -- "${cur}"))
      return
      ;;
    --service)
      COMPREPLY=($(ctx service ls 2>/dev/null | awk '
        NR > 2 {
          port = $1
          name = tolower($3)
          if (port ~ /^80\// || port ~ /^443\// || name ~ /http/) print ++count
        }
      '))
      return
      ;;
    -w|--wordlist|-u|--url|--host|-c|--cookies)
      COMPREPLY=($(compgen -f -- "${cur}"))
      return
      ;;
    --exclude-status|--exclude-length)
      COMPREPLY=()
      return
      ;;
  esac

  if [[ "${COMP_WORDS[1]}" == "dns" ]]; then
    COMPREPLY=($(compgen -W "-d --domain -w --wordlist -t --threads --resolver --timeout --wildcard --status --clear-cache -h --help -V --version" -- "${cur}"))
    return
  fi

  COMPREPLY=($(compgen -W "dns -w --wordlist -u --url --host --ip --service -c --cookies --exclude-status --exclude-length -k --no-tls-validation --tls-verify --preset --status --clear-cache -h --help -V --version" -- "${cur}"))
}

complete -F _xgobuster_completion xgobuster xgo
