#compdef xffuf

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
  _arguments \
    '1:mode:(vhost)' \
    '(-d --domain)'{-d,--domain}'[target domain]:domain:' \
    '(-w --wordlist)'{-w,--wordlist}'[wordlist path]:file:_files' \
    '(-u --url)'{-u,--url}'[override HTTP service URL]:url:' \
    '--host[registered xhost hostname]:hostname:' \
    '--ip[use target IP as HTTP host]' \
    '--service[select HTTP service]:service:_xffuf_services' \
    '(-c --cookies)'{-c,--cookies}'[cookies]:cookies:' \
    '(-k --no-tls-validation)'{-k,--no-tls-validation}'[disable TLS verification]' \
    '--tls-verify[verify TLS certificates]' \
    '--suggest[show calibration and optionally run a trial]' \
    '--trial[scan without logs, cache, or host registration]' \
    '--no-auto-filter[disable automatic filtering]' \
    '--status[show vhost wordlist status]' \
    '--clear-cache[clear vhost wordlist cache]' \
    '--next[continue with the next wordlist]' \
    '--force[rerun the first available wordlist]' \
    '-H[HTTP header]:header:' \
    '-fw[filter response words]:number:' \
    '-fs[filter response size]:number:' \
    '-fl[filter response lines]:number:' \
    '-fc[filter response status]:status:' \
    '-fr[filter response regex]:regex:' \
    '(-h --help)'{-h,--help}'[show help]' \
    '(-V --version)'{-V,--version}'[show version]'
}

_xffuf "$@"
