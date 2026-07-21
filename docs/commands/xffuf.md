# xffuf Online Help

[日本語](./xffuf.ja.md)

`xffuf` integrates ffuf with ctx Targets, web services, wordlist recommendations, and search history. It is intentionally focused on virtual-host and query-parameter enumeration.

## Synopsis

```text
xffuf <vhost|param> [options] [ffuf-options]
```

```sh
xffuf vhost --domain example.thm
xffuf param -u 'http://example.thm/?FUZZ=value'
xffuf param -u 'http://example.thm/?page=FUZZ' -mc 200,302
```

An active ctx workspace and Primary Target are required, as are `ctx` and `ffuf` in the Kali service.

## Modes

### `vhost`

Places words in the HTTP `Host` header. Without an explicit domain, ctx hostnames determine the domain; the HTTP service and destination IP also come from ctx. Unless manual match/filter options are present, xffuf samples nonexistent hosts and calibrates status, size, and word-count filters. Use `--no-auto-filter` to disable this.

### `param`

Exactly one `FUZZ` must appear in the query. Its position selects the ctx wordlist kind:

```text
?FUZZ=value   enumerate parameter names (`parameter-name`)
?name=FUZZ    enumerate parameter values (`parameter-value`)
```

To find parameter names that trigger redirects:

```sh
xffuf param -u 'http://target.thm/?FUZZ=https://example.com' -mc 301-308
```

`-mc` keeps matching responses. `-fc` excludes them, so `-fc 301-308` does not search specifically for redirects.

## Automatic Wordlists and Resume

Without `-w`, vhost uses ctx kind `subdomain`, while param uses `parameter-name` or `parameter-value`. Recommendations are consumed in priority order. Search state is scoped by Target, URL, mode, and related settings; rerunning the same command skips already searched words and continues with the highest-priority unfinished list. There is no `--next` flag.

`-w, --wordlist <path>` bypasses automatic recommendations.

## Target Selection

- `-d, --domain <domain>`: base vhost domain; unavailable in param mode.
- `-u, --url <url>`: explicit URL instead of a ctx service.
- `--host <hostname>`: use a registered xhost hostname.
- `--ip`: use the Target IP as the HTTP host.
- `--service <number>`: select a displayed web service.

`--url` cannot be combined with `--host`, `--ip`, or `--service`.

## Request, TLS, Match, and Filter

- `-c, --cookies <value>`: send cookies.
- `-k, --no-tls-validation`: disable certificate verification.
- `--tls-verify`: require certificate verification.
- Match options: `-mc`, `-ml`, `-mr`, `-ms`, `-mw`.
- Filter options: `-fc`, `-fl`, `-fr`, `-fs`, `-fw`.

Match options describe responses to retain; filter options describe responses to discard. Manual filters suppress automatic calibration.

## Calibration and Trials

- `--suggest`: display calibration and offer a trial; incompatible with manual filters.
- `--trial`: run without ctx logs, cache updates, or host registration.
- `--no-auto-filter`: disable automatic vhost calibration.

A trial never advances saved progress.

## Status and Cleanup

```sh
xffuf vhost --status
xffuf vhost --clear-cache
```

`--status` reports wordlist progress for the scope. `--clear-cache` removes that search progress. Use `ctx web clear` to remove discoveries and broader web search state.

## Persistence and Troubleshooting

Normal runs save the expanded ffuf command, exit state, output, wordlist progress, and discovered hosts or parameters. Cookies supplied on the command line may appear in logs.

- No wordlist found: inspect `ctx wordlist --kind <kind> --usable-only`, or pass `-w`.
- Too many vhost results: keep calibration enabled or add a size/status filter.
- Restart from the first list: verify the scope, then use `--clear-cache`.
- A URL containing `&` breaks: quote the entire URL.
