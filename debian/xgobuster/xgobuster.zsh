#compdef xgobuster

_xgobuster_services() {
  local -a services
  services=("${(@f)$(ctx service ls 2>/dev/null | awk '
    NR > 2 {
      port = $1
      name = tolower($3)
      if (port ~ /^80\// || port ~ /^443\// || name ~ /http/) {
        printf "%d:%s %s\n", ++count, port, $3
      }
    }
  ')}")
  _describe 'web service' services
}

_xgobuster() {
  local mode="${words[2]}"
  if [[ "${mode}" == "dns" ]]; then
    _arguments -s \
      '1:mode:(dns)' \
      '(-h --help)'{-h,--help}'[show this help]' \
      '(-V --version)'{-V,--version}'[show version]' \
      '--online-help[show the versioned online help URL]' \
      '(-d --domain)'{-d,--domain}'[DNS domain]:domain:' \
      '(-w --wordlist)'{-w,--wordlist}'[use an explicit wordlist]:wordlist:_files' \
      '--threads[DNS threads]:threads:' \
      '--resolver[DNS resolver]:resolver:' \
      '--timeout[DNS timeout]:timeout:' \
      '--wildcard[continue with wildcard DNS results]' \
      '--status[show DNS wordlist search status]' \
      '--clear-cache[clear DNS search cache]' \
      '*:gobuster option:'
    return
  fi

  _arguments -s \
    '1:mode:(dns)' \
    '(-h --help)'{-h,--help}'[show this help]' \
    '(-V --version)'{-V,--version}'[show version]' \
    '--online-help[show the versioned online help URL]' \
    '--preset[select a technology preset]:preset:(php wordpress aspnet java node static)' \
    '--status[show wordlist search status]' \
    '--clear-cache[clear scoped wordlist progress]' \
    '(-w --wordlist)'{-w,--wordlist}'[use an explicit wordlist]:wordlist:_files' \
    '(-u --url)'{-u,--url}'[override the target URL]:url:' \
    '--host[use a registered xhost hostname for the target]:hostname:' \
    '--ip[use the target IP instead of an xhost hostname]' \
    '--service[select a web service by number]:service:_xgobuster_services' \
    '(-c --cookies)'{-c,--cookies}'[send cookies with requests]:cookie:' \
    '--exclude-status[exclude responses with these status codes]:status code:' \
    '--exclude-length[exclude responses with these body sizes]:size:' \
    '(-k --no-tls-validation)'{-k,--no-tls-validation}'[disable TLS certificate validation]' \
    '--tls-verify[verify TLS certificates for this run]' \
    '*:gobuster option:'
}
