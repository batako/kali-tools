#compdef xscp

_xscp_actions() {
  local -a actions
  actions=(
    'upload:copy a local file to the target'
    'download:copy a remote file to the local machine'
  )
  _describe -V 'action' actions
}

_xscp() {
  _arguments -s \
    '1:action:_xscp_actions' \
    '2:source:_files' \
    '3:destination:_files' \
    '4:credential-id or username:' \
    '(-p --port)'{-p,--port}'[override the SSH port]:port:' \
    '--service[select a discovered SSH service by number]:service:' \
    '(-h --help)'{-h,--help}'[show this help]' \
    '(-V --version)'{-V,--version}'[show version]'
    '--online-help[show the versioned online help URL]'
}

_xscp "$@"
