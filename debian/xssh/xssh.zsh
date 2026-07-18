#compdef xssh

_xssh() {
  _arguments -s \
    '1:credential or command:(key)' \
    '(-h --help)'{-h,--help}'[show this help]' \
    '(-V --version)'{-V,--version}'[show version]'
}

_xssh
