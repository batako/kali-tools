#compdef xmagic

_xmagic_commands() {
  local -a commands
  commands=(
    'ls:list supported magic-number types'
    'set:replace or prepend a magic number in a copy'
  )
  _describe -V 'command' commands
}

_xmagic_types() {
  local -a types
  types=(
    'gif:GIF89a image'
    'jpg:JPEG image'
    'jpeg:JPEG image alias'
    'png:PNG image'
    'pdf:PDF document'
    'zip:ZIP archive'
  )
  _describe -V 'type' types
}

_xmagic() {
  if (( CURRENT == 2 )); then
    _xmagic_commands
    return
  fi

  case "${words[2]}" in
    set)
      if (( CURRENT == 3 )); then
        _xmagic_types
      elif (( CURRENT == 4 )); then
        _files
      fi
      ;;
  esac
}

_xmagic
