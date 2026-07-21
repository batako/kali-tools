#compdef xssh

_xssh() {
  _arguments -s \
    '1:credential or command:(key)' \
    '(-h --help)'{-h,--help}'[show this help]' \
    '(-V --version)'{-V,--version}'[show version]'
    '--online-help[show the versioned online help URL]'
}

_xssh
