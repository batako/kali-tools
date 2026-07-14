#compdef xgobuster

_xgobuster() {
  _arguments -s \
    '(-h --help)'{-h,--help}'[show this help]' \
    '(-V --version)'{-V,--version}'[show version]' \
    '--preset[select a technology preset]:preset:(php wordpress aspnet java node static)' \
    '--status[show wordlist search status]' \
    '--next[continue with the next automatic wordlist]' \
    '--force[rerun an already completed wordlist]' \
    '--profile[limit automatic selection to a profile]:profile:(web-quick web-standard web-deep)' \
    '(-w --wordlist)'{-w,--wordlist}'[use an explicit wordlist]:wordlist:_files' \
    '(-u --url)'{-u,--url}'[override the target URL]:url:' \
    '*:gobuster option:'
}
