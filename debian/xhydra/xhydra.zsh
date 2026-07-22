#compdef xhydra

_xhydra_modes() {
  local -a modes
  modes=(
    'http:run Hydra against an HTTP form'
    'ssh:attack an SSH service'
    'ftp:attack an FTP service'
    'smb:attack an SMB service'
  )
  _describe -V 'mode' modes
}

_xhydra() {
  local mode="${words[2]}"
  case "${mode}" in
    http)
      _arguments -s \
        '1:mode:_xhydra_modes' \
        '(-h --help)'{-h,--help}'[show this help]' \
        '(-V --version)'{-V,--version}'[show version]' \
        '--online-help[show the versioned online help URL]' \
        '(-u --username)'{-u,--username}'[username to test]:username:' \
        '(-r --request)'{-r,--request}'[use a raw HTTP request template]:request:_files' \
        '--url[URL to test without a request file]:url:' \
        '--data[form body to test without a request file]:body:' \
        '--user-field[username form field; optional when body uses ^USER^]:field:' \
        '--password-field[password form field; optional when body uses ^PASS^]:field:' \
        '--fail-json[JSON response failure value]:field=value:' \
        '--success-json[JSON response success value]:field=value:' \
        '--fail-body[response body failure text]:text:' \
        '--success-body[response body success text]:text:' \
        '--fail-status[HTTP failure status; currently 401]:status:(401)' \
        '--success-redirect[treat HTTP 302 as success]' \
        '(-P --password-list)'{-P,--password-list}'[use an explicit password list]:password-list:_files'
      ;;
    ssh)
      _arguments -s \
        '1:mode:_xhydra_modes' \
        '(-h --help)'{-h,--help}'[show this help]' \
        '(-V --version)'{-V,--version}'[show version]' \
        '--online-help[show the versioned online help URL]' \
        '(-u --username)'{-u,--username}'[username to test]:username:' \
        '--password[fixed password for username search]:password:' \
        '(-L --user-list)'{-L,--user-list}'[use a username list]:user-list:_files' \
        '--host[target host override]:host:' \
        '(-p --port)'{-p,--port}'[target port override]:port:' \
        '--service[select a discovered service by number]:service:' \
        '(-t --tasks)'{-t,--tasks}'[override SSH parallel tasks]:tasks:' \
        '--status[show password wordlist progress; requires -u]' \
        '--clear-cache[clear scoped password search progress; requires -u]' \
        '(-P --password-list)'{-P,--password-list}'[use an explicit password list]:password-list:_files'
      ;;
    ftp)
      _arguments -s \
        '1:mode:_xhydra_modes' \
        '(-h --help)'{-h,--help}'[show this help]' \
        '(-V --version)'{-V,--version}'[show version]' \
        '--online-help[show the versioned online help URL]' \
        '(-u --username)'{-u,--username}'[username to test]:username:' \
        '--password[fixed password for username search]:password:' \
        '(-L --user-list)'{-L,--user-list}'[use a username list]:user-list:_files' \
        '--host[target host override]:host:' \
        '(-p --port)'{-p,--port}'[target port override]:port:' \
        '--service[select a discovered service by number]:service:' \
        '(-t --tasks)'{-t,--tasks}'[override FTP parallel tasks]:tasks:' \
        '--status[show password wordlist progress; requires -u]' \
        '--clear-cache[clear scoped password search progress; requires -u]' \
        '(-P --password-list)'{-P,--password-list}'[use an explicit password list]:password-list:_files'
      ;;
    smb)
      _arguments -s \
        '1:mode:_xhydra_modes' \
        '(-h --help)'{-h,--help}'[show this help]' \
        '(-V --version)'{-V,--version}'[show version]' \
        '--online-help[show the versioned online help URL]' \
        '(-u --username)'{-u,--username}'[username to test]:username:' \
        '--password[fixed password for username search]:password:' \
        '(-L --user-list)'{-L,--user-list}'[use a username list]:user-list:_files' \
        '--host[target host override]:host:' \
        '(-p --port)'{-p,--port}'[target port override]:port:' \
        '--service[select a discovered service by number]:service:' \
        '(-t --tasks)'{-t,--tasks}'[override SMB parallel tasks]:tasks:' \
        '--status[show password wordlist progress; requires -u]' \
        '--clear-cache[clear scoped password search progress; requires -u]' \
        '(-P --password-list)'{-P,--password-list}'[use an explicit password list]:password-list:_files'
      ;;
    *)
      _arguments -s \
        '1:mode:_xhydra_modes' \
        '(-h --help)'{-h,--help}'[show this help]' \
        '(-V --version)'{-V,--version}'[show version]' \
        '--online-help[show the versioned online help URL]'
      ;;
  esac
}

_xhydra "$@"
