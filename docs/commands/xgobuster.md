# xgobuster Online Help

[日本語](./xgobuster.ja.md)

`xgobuster` integrates Gobuster with ctx for web path/file and DNS subdomain enumeration. `xgo` is a shortcut to the same executable.

## Synopsis

```text
xgobuster [options] [gobuster-options]
xgobuster dns [options] [gobuster-options]
```

```sh
xgo
xgo --preset php
xgo -x php,txt
xgo dns --domain example.thm
```

The default is Gobuster `dir`; the `dns` subcommand selects DNS mode. An active ctx workspace, Primary Target, `ctx`, and `gobuster` are required.

## Web Enumeration

The URL is derived from ctx web services, hostnames, and Target IP. Searches with extensions or a preset are tracked as file searches; otherwise they are directory searches.

Technology presets configure extensions:

| preset | extensions |
| --- | --- |
| `php`, `wordpress` | `php,inc,phps` |
| `aspnet` | `asp,aspx,config` |
| `java` | `jsp,do,action` |
| `node` | `js,json` |
| `static` | `html,htm,js` |

`-x, --extensions <list>` passes an explicit list. Gobuster still controls whether extensionless words are also requested.

## DNS Enumeration

```sh
xgobuster dns --domain target.thm
```

Without `--domain`, ctx hostname data supplies the domain. Discovered subdomains are persisted and can become hostname candidates for later web enumeration.

## Wordlists, Priority, and Limits

Web searches use ctx kind `directory`; extension searches expand the same base words for each extension. DNS uses `subdomain`. Recommendations run in purpose-specific priority order. Repeating the same scope removes already searched words and continues from the highest-priority unfinished candidate.

Automatic work is capped by `web.directory.max-requests`, `web.file.max-requests`, and `dns.max-queries`. `-w, --wordlist <path>` bypasses recommendations.

## Target and Response Options

- `-u, --url <url>`: explicit URL.
- `--host <hostname>`: registered hostname.
- `--ip`: use the Target IP.
- `--service <number>`: choose a discovered web service.
- `-d, --domain <domain>`: DNS domain.
- `-c, --cookies <value>`: send cookies.
- `--exclude-status <code>`: exclude statuses.
- `--exclude-length <size>`: exclude body sizes.
- `-k, --no-tls-validation`: disable TLS verification.
- `--tls-verify`: require TLS verification.

`--url` cannot be combined with host/IP/service selection; `--host` and `--ip` are mutually exclusive. TLS options are mutually exclusive. Wildcard sites usually require status or length exclusion.

## Status, Cache, and Persistence

```sh
xgo --status
xgo --preset php --status
xgo dns --domain target.thm --status
xgo --clear-cache
```

State is scoped by mode, Target/URL/domain, preset, and extensions. `--clear-cache` removes only scoped word progress and does not run Gobuster. `ctx web clear` also removes discoveries.

Runs save commands, status, output, wordlist/word progress, and discovered paths, files, or subdomains. Inspect results with `ctx web ls` and executions with `ctx log`.

## Troubleshooting

- It does not advance: inspect `--status` and configured request limits.
- Files with extensions are needed: use `--preset` or `-x`.
- Gobuster reports wildcard responses: add an exclusion.
- Hostnames do not resolve: inspect `ctx host ls` and run `ctx hosts sync`.
