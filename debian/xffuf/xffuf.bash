_xffuf_complete() {
  local cur prev mode common options word
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  mode=""
  for word in "${COMP_WORDS[@]:1}"; do
    case "${word}" in
      vhost|param) mode="${word}"; break ;;
    esac
  done

  if [[ "${mode}" == "vhost" && "${prev}" == "--service" ]]; then
    COMPREPLY=($(ctx service ls 2>/dev/null | awk '
      NR > 2 {
        port = $1
        name = tolower($3)
        if (port ~ /^80\// || port ~ /^443\// || name ~ /http/) print ++count
      }
    '))
    return
  fi
  if [[ "${mode}" == "param" && "${prev}" == "--profile" ]]; then
    COMPREPLY=($(compgen -W "parameter-name parameter-value-generic parameter-value-url parameter-value-file parameter-value-username parameter-value-number" -- "${cur}"))
    return
  fi

  common="-w --wordlist -u --url -c --cookies -k --no-tls-validation --tls-verify --trial --no-auto-filter --status --clear-cache --next --force -H -mc -ml -mr -ms -mw -fc -fl -fr -fs -fw -t -rate -timeout -h --help -V --version"
  case "${mode}" in
    vhost) options="${common} -d --domain --host --ip --service --suggest" ;;
    param) options="${common} --profile" ;;
    *) options="vhost param -h --help -V --version" ;;
  esac
  COMPREPLY=($(compgen -W "${options}" -- "${cur}"))
}
complete -F _xffuf_complete xffuf
