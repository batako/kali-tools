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
  _arguments -s \
    '(-h --help)'{-h,--help}'[show this help]' \
    '(-V --version)'{-V,--version}'[show version]' \
    '--preset[select a technology preset]:preset:(php wordpress aspnet java node static)' \
    '--status[show wordlist search status]' \
    '--sitemap[show collected paths as a site map]' \
    '--next[continue with the next automatic wordlist]' \
    '--force[rerun an already completed wordlist]' \
    '--profile[limit automatic selection to a profile]:profile:(web-quick web-standard web-deep)' \
    '(-w --wordlist)'{-w,--wordlist}'[use an explicit wordlist]:wordlist:_files' \
    '(-u --url)'{-u,--url}'[override the target URL]:url:' \
    '--host[use a registered xhost hostname for the target]:hostname:' \
    '--ip[use the target IP instead of an xhost hostname]' \
    '--service[select a web service by number]:service:_xgobuster_services' \
    '(-c --cookies)'{-c,--cookies}'[send cookies with requests]:cookie:' \
    '--exclude-length[exclude responses with these body sizes]:size:' \
    '(-k --no-tls-validation)'{-k,--no-tls-validation}'[disable TLS certificate validation]' \
    '--tls-verify[verify TLS certificates for this run]' \
    '*:gobuster option:'
}
