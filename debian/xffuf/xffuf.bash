_xffuf_complete() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  if [[ "${prev}" == "--service" ]]; then
    COMPREPLY=($(ctx service ls 2>/dev/null | awk '
      NR > 2 {
        port = $1
        name = tolower($3)
        if (port ~ /^80\// || port ~ /^443\// || name ~ /http/) print ++count
      }
    '))
    return
  fi
  COMPREPLY=($(compgen -W "vhost -d --domain -w --wordlist -u --url --host --ip --service -c --cookies -k --no-tls-validation --tls-verify --suggest --trial --no-auto-filter --status --clear-cache --next --force -H -fw -fs -fl -fc -fr -t -rate -timeout -h --help -V --version" -- "${cur}"))
}
complete -F _xffuf_complete xffuf
