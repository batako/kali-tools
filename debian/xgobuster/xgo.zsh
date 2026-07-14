#compdef xgo

autoload -Uz _xgobuster

_xgo() {
  _xgobuster "$@"
}
