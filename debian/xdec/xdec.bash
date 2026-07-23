_xdec_completion() {
  local cur prev subcommand
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  subcommand="${COMP_WORDS[1]}"

  if [[ "$prev" == "-f" || "$prev" == "--file" || "$prev" == "-w" || "$prev" == "--wordlist" ]]; then
    COMPREPLY=($(compgen -f -- "$cur"))
    return
  fi

  if [[ "$prev" == "--string" || "$prev" == "--scope" || "$prev" == "--username" ]]; then
    COMPREPLY=()
    return
  fi

  if [[ "$subcommand" == "decode" || "$subcommand" == "recover" ]]; then
    if [[ "$subcommand" == "recover" ]]; then
      COMPREPLY=(
        $(compgen -W "--wordlist --scope --username --save-credential --no-save-credential --yes --refresh --dry-run --json" -- "$cur")
        $(compgen -f -- "$cur")
      )
    else
      COMPREPLY=(
        $(compgen -W "--json" -- "$cur")
        $(compgen -f -- "$cur")
      )
    fi
    return
  fi

  if [[ "$subcommand" == "help" ]]; then
    COMPREPLY=($(compgen -W "decode recover help version" -- "$cur"))
    return
  fi

  if [[ "$subcommand" == "version" ]]; then
    COMPREPLY=()
    return
  fi

  if [[ "$COMP_CWORD" -eq 1 ]]; then
    COMPREPLY=($(compgen -W "decode recover help version --online-help" -- "$cur"))
    return
  fi

  COMPREPLY=($(compgen -W "--online-help" -- "$cur"))
}

complete -F _xdec_completion xdec
