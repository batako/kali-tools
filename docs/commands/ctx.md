# ctx Online Help

[日本語](./ctx.ja.md)

`ctx` manages targets and execution history for TryHackMe, Hack The Box, and similar lab environments. It keeps targets, hostnames, detected services, credentials, web discoveries, wordlist progress, notes, and command logs together in a workspace context.

## Synopsis

```text
ctx <command> [options]
```

```sh
ctx workspace init
ctx target add 10.10.10.20 --name target
ctx scan
ctx service ls
ctx log
```

`ctx` searches from the current directory upward for a `.ctx` marker. Most operations must be run inside an active workspace.

## Workspaces

```text
ctx workspace init
ctx workspace ls
ctx workspace rm [id] [-y|--yes]
```

`init` registers the current directory. The `.ctx` file contains only the normalized workspace UUID; SQLite data and tool state live in the ctx data directory. `rm` removes the workspace's ctx-managed data and marker after confirmation. It does not recursively delete ordinary files in the working directory.

## Projects

Projects are an optional convenience for creating workspaces below configured roots. A workspace can also be initialized in any directory without using Projects.

```text
ctx project root [path]
ctx project root add <path> [--name <name>]
ctx project root use <name>
ctx project root ls
ctx project root rm <name>
ctx project root move <from> <to> [--dry-run] [-y|--yes]
ctx project new <name>
ctx project ls
ctx project rm <id|name> [-y|--yes]
```

`root` displays or changes the active root. `root add` derives a name from the path when `--name` is omitted. `root move` moves ctx-managed Projects between registered roots; inspect it first with `--dry-run`. `ctx project <name>` is shorthand for `ctx project new <name>`.

## Targets and Hosts

### Targets

```text
ctx target set <ip>
ctx target add <ip> [--name <name>]
ctx target update <ip>
ctx target use <name>
ctx target rm <name>
ctx target ls
```

A workspace may contain multiple targets, but add-ons use one Primary Target by default. `ctx target <ip>` is shorthand for `target set`. `ctx ip` prints the Primary Target IP and `ctx ip <ip>` updates it.

### Hostnames

```text
ctx host add <hostname> [--target <name>]
ctx host rm <hostname>
ctx host ls
ctx host <hostname>
```

`ctx host <hostname>` is shorthand for `host add`. Web tools can prefer registered hostnames over a raw Target IP.

### `/etc/hosts` integration

```text
ctx hosts show
ctx hosts sync [--internal]
ctx hosts clean [--internal]
```

`sync` writes the ctx-managed block derived from Targets and Hosts and re-executes through sudo when required. `clean` removes only the managed block. `--internal` is reserved for that sudo re-execution and should not normally be supplied.

## Scans and Services

```text
ctx scan [ip] [-p|--ports <ports>] [-n|--dry-run] [-f|--force]
ctx service ls [--target <name>] [--format <shell|json>] [--format-version <version>]
```

`scan` runs Nmap with `-Pn -n -sV`, saves normal and XML output, and imports open ports and service metadata. `--ports` is passed as Nmap's `-p`; `--dry-run` prints the expanded command; `--force` bypasses successful-scan deduplication. An explicit IP overrides the Primary Target for that scan.

`service ls` shows saved services for the Primary or selected Target. Integrations should consume versioned JSON instead of parsing the human-readable table.

## Credentials

```text
ctx credential ls [scope]
ctx credential set <scope> <username> [password]
ctx credential add <scope> <username> [password]
ctx credential update <scope> <username> [password]
ctx credential rm <id|username|scope username> [-y|--yes]
```

Scopes such as `ssh`, `ftp`, and `smb` identify consumers. Omitting the password stores a credential without a password. `ctx credential <scope> <username> [password]` is shorthand for `set`. JSON output includes plaintext passwords; never save it to shared logs or artifacts.

## Web Discoveries

```text
ctx web ls [--target <name>] [--type <type>] [--format <shell|json>] [--format-version <version>]
ctx web show <id> [--target <name>]
ctx web clear [--target <name>]
```

Types are `path`, `param`, `param-name`, and `param-value`. `clear` removes the selected Target's web discoveries, wordlist runs, and xgobuster/xffuf searched-word cache after confirmation. It does not remove command logs or another Target's state.

## Wordlists

```text
ctx wordlist [ls] [path] [--kind <kind>] [--usable-only] [--format <table|json|markdown>]
ctx wordlist show <ID|path> [path]
ctx wordlist extract [-y|--yes] [--force] [--remove-source]
```

`ctx wordlist` and `ctx wordlist ls` are identical. By default ctx recursively inventories `/usr/share/wordlists`, following provider symlinks and recording format, compression, usability, known/unknown classification, and per-purpose priority.

Supported kinds are `all`, `directory`, `subdomain`, `parameter-name`, `parameter-value`, `password`, `username`, `endpoint`, and `unknown`. A kind query returns known, high-confidence files first in purpose-specific priority order. `--usable-only` excludes unusable entries. xffuf, xgobuster, xhydra, and xsteg share these recommendations.

`extract` only supports the built-in, SHA-256-verified `/usr/share/wordlists/rockyou.txt.gz` master. `--force` replaces an existing output, `--remove-source` deletes the compressed source after success, and `--yes` skips confirmation. ctx offers sudo re-execution when the destination is not writable.

## Timeline, Notes, and Logs

```text
ctx note <text>
ctx log [id] [-p|--plain|-v|--verbose|-i|--interactive]
```

`note` appends a workspace timeline note. `log` displays notes and command executions chronologically. `--plain` is compact, `--verbose` includes IDs/status/exit codes, and `--interactive` opens the selection UI. `ctx log <id>` shows the original and expanded command, status, stdout, and stderr.

For parent/child logs, the normal list shows the parent and the detail view exposes internal commands under `steps`. The add-on `log start` and `log finish` interfaces read versioned JSON from stdin.

## Running Arbitrary Commands

```text
ctx x <command> [args...]
```

The child process is executed directly, with stdout/stderr streamed and saved. `$IP` and `${IP}` inside each argument expand to the Primary Target IP.

```sh
ctx x nmap -sV '$IP'
ctx x curl 'http://${IP}/'
```

For pipes or redirects, invoke a shell explicitly, for example `ctx x sh -c '...'`. Arguments and output are logged, so do not use it with secrets unless that persistence is intended.

## Shell and JSON Integration

```text
ctx prompt [--format <shell|json>] [--format-version <version>] [--field <name>]
ctx formats [--format <shell|json>] [--format-version <version>]
ctx completion <zsh|bash> [--extra-shortcuts]
ctx init-shell [--remove|--extra-shortcuts]
```

`prompt` returns workspace, local network, and Primary Target data. Fields are `active`, `workspace-id`, `workspace-name`, `workspace-path`, `local-ip`, `local-interface`, `target-name`, and `target-ip`. `formats` lists available JSON endpoints and versions; integrations should negotiate these rather than relying on the ctx package version.

`init-shell` installs completion and shortcuts: `xinit`, `xconfig`, `xworkspace`, `xstatus`, `xproject`, `xnew`, `xtarget`, `xip`, `xhost`, `xhosts`, `xscan`, `xservice`, `xweb`, `xwordlist`, `xcredential`, `xnote`, `xlog`, `xprompt`, `xformats`, `x`, `xcompletion`, `xdoctor`, `xinit-shell`, and `xreset`. `--extra-shortcuts` also adds `pj`, `ta`, `cr`, and `sv`.

## Configuration

```text
ctx config ls
ctx config get <key>
ctx config set <key> <value>
```

Important keys include `project.root`, `web.directory.max-requests`, `web.file.max-requests`, `web.vhost.max-requests`, `web.vhost.calibration-samples`, `web.vhost.calibration-confidence`, `password.max-requests`, and `dns.max-queries`.

## Maintenance and Safety

- `ctx status` shows the active workspace.
- `ctx doctor` checks dependencies and environment state.
- `ctx reset [-y|--yes]` removes all ctx data/configuration but not workspace directories or shell history.
- `ctx -V|--version` prints the version.

Credentials may be stored in plaintext. Command output can contain tokens or target data. Keep the ctx data directory private, and use versioned APIs rather than editing SQLite directly.

## Troubleshooting

- No active workspace: initialize one or change into a registered workspace tree.
- No Primary Target: use `ctx target add` or `ctx target use`.
- `/etc/hosts` permission failure: allow the prompted sudo re-execution.
- A scan is skipped: the same scope already succeeded; use `--force` intentionally.
- JSON integration fails: inspect `ctx formats --format json --format-version 1.0`.
