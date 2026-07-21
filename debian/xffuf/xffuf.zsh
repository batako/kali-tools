#compdef xffuf

_xffuf_modes() {
  local -a modes
  modes=(
    'vhost:enumerate HTTP virtual hosts'
    'param:fuzz query parameter names or values'
  )
  _describe 'xffuf mode' modes
}

_xffuf_services() {
  local -a services
  services=($(ctx service ls 2>/dev/null | awk '
    NR > 2 {
      port = $1
      name = tolower($3)
      if (port ~ /^80\// || port ~ /^443\// || name ~ /http/) {
        printf "%d:%s %s\n", ++count, port, $3
      }
    }
  '))
  _describe 'web service' services
}

_xffuf() {
  local -a common
  common=(
    '1:mode:_xffuf_modes'
    '(-w --wordlist)'{-w,--wordlist}'[wordlist path]:file:_files'
    '(-u --url)'{-u,--url}'[target URL]:url:'
    '(-c --cookies)'{-c,--cookies}'[cookies]:cookies:'
    '(-k --no-tls-validation)'{-k,--no-tls-validation}'[disable TLS verification]'
    '--tls-verify[verify TLS certificates]'
    '--trial[scan without saving logs, cache, or discoveries]'
    '--no-auto-filter[disable automatic filtering]'
    '--status[show wordlist status]'
    '--clear-cache[clear wordlist cache]'
    '-H[HTTP header]:header:'
    '-mc[match response status]:status:'
    '-ml[match response lines]:number:'
    '-mr[match response regex]:regex:'
    '-ms[match response size]:number:'
    '-mw[match response words]:number:'
    '-fc[filter response status]:status:'
    '-fl[filter response lines]:number:'
    '-fr[filter response regex]:regex:'
    '-fs[filter response size]:number:'
    '-fw[filter response words]:number:'
    '(-h --help)'{-h,--help}'[show help]'
    '(-V --version)'{-V,--version}'[show version]'
  )

  case ${words[2]} in
    vhost)
      _arguments "${common[@]}" \
        '(-d --domain)'{-d,--domain}'[target domain]:domain:' \
        '--host[registered xhost hostname]:hostname:' \
        '--ip[use target IP as HTTP host]' \
        '--service[select HTTP service]:service:_xffuf_services' \
        '--suggest[show calibration and optionally run a trial]'
      ;;
    param)
      _arguments "${common[@]}"
      ;;
    *)
      _arguments \
        '1:mode:_xffuf_modes' \
        '(-h --help)'{-h,--help}'[show help]' \
        '(-V --version)'{-V,--version}'[show version]'
      ;;
  esac
}

_xffuf "$@"
