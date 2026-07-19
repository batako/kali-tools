# Kali Tools

This monorepo manages self-made CLI tools for Kali Linux. Each tool has an independent Go entrypoint and Debian package definition, and the packages are published through an APT repository.

## Tools

- `req`: Send raw HTTP requests saved as `.req` files.
- `ctx`: Manage workspace context, targets, services, credentials, notes, and logs.
- `xssh`: Connect to the current target over SSH using ctx credentials.
- `xscp`: Transfer files to and from the current target over SSH using ctx credentials.
- `xftp`: Connect to the current target over FTP using ctx credentials.
- `xsmb`: Discover SMB shares and connect to a selected share using ctx credentials.
- `xgobuster`: Run Gobuster against the current target and save web discoveries to ctx.
- `xffuf`: Enumerate HTTP virtual hosts with ffuf and ctx calibration.

`xssh`, `xscp`, `xftp`, and `xsmb` are ctx add-ons. They use the ctx JSON API and do not read the ctx SQLite database directly.

## Installation

Add the public repository once:

```sh
echo "deb [trusted=yes] https://offsec.batako.net stable main" \
  | sudo tee /etc/apt/sources.list.d/batako-offsec.list
sudo apt update
```

Install the tools you need:

```sh
sudo apt install req
sudo apt install ctx
sudo apt install xssh
sudo apt install xscp
sudo apt install xftp
sudo apt install xsmb
sudo apt install xgobuster
sudo apt install xffuf
```

Dependencies are declared by each Debian package. For example, `xssh` and `xscp` use `openssh-client` and `sshpass`, `xftp` uses `lftp`, `xsmb` uses `smbclient`, `xgobuster` uses `gobuster` and `wordlists`, and `xffuf` uses `ffuf` and `seclists`.

## Usage

### req

Replay a raw HTTP request. The request method, path, host, headers, and body are read from the file.

```sh
req login.req
req -S login.req
req -k login.req
```

`-S`/`--https` forces HTTPS when the request file does not determine a scheme. Use `-k`/`--no-tls-validation` for test targets with expired or self-signed certificates; `--tls-verify` explicitly keeps validation enabled. `-h`/`--help` and `-V`/`--version` are also available. `Accept-Encoding` and `Content-Length` from the request file are not sent; Go's `net/http` handles gzip response decompression. `req` is distributed as the standalone `req` Debian package.

### ctx

Create or select a workspace, then register targets and credentials as needed:

```sh
ctx workspace init
ctx status
ctx target add 10.10.10.20 --name target
ctx credential set ssh root password
ctx scan
ctx service ls
ctx log
```

Useful commands include `ctx workspace ls`, `ctx workspace rm`, `ctx note`, `ctx prompt`, `ctx reset`, and `ctx x <command> [args...]`. Run `ctx --help` for the complete command reference.

Discovery integrations use `/usr/share/wordlists` automatically. `xgobuster` selects directory-discovery lists from the installed `dirb`, `dirbuster`, and `seclists` trees. Password, parameter, and fuzzing lists are excluded from normal directory scans.

```sh
sudo apt install wordlists seclists dirb dirbuster ffuf
ctx config set web.directory.max-requests 1000000
ctx config set web.file.max-requests 200000
ctx config set web.vhost.max-requests 10000
ctx config set web.vhost.calibration-samples 10
ctx config set web.vhost.calibration-confidence 90
ctx config set web.tls.verify false
```

Install shell integration when using the `x` helpers:

```sh
ctx completion zsh
ctx completion bash
ctx init-shell
```

### Add-ons

Each add-on accepts an optional credential ID or username. With multiple credentials or service ports, it displays a selection. When no matching credential is registered, the underlying client is used with the supplied username or without a username.

```sh
xssh
xssh root
xscp upload ./local.txt /tmp/remote.txt
xscp download /tmp/remote.txt ./local.txt
xftp
xftp ftpuser
xsmb
xsmb smbuser
```

The add-ons save connection start/end times, status, exit code, sanitized command, stdout, and stderr to ctx logs. Review them with `xlog`. Passwords are not stored in the command log.

`xsmb` lists disk shares, excludes `IPC$`, and lets you select the share before connecting. `xssh` defaults to port 22, `xftp` to port 21, and `xsmb` to port 445 when ctx has no matching service record.

`xscp upload` copies a local file to the target and `xscp download` copies a remote file locally. Both commands use the same SSH credential and service selection as `xssh`.

`xgobuster` selects a web service on the current target and runs `gobuster dir`. When `xhost` has manually registered hostnames for the target, one hostname is used automatically; if multiple hostnames exist, `xgobuster` prompts for a hostname or the target IP. Use `--host <hostname>` for a deterministic registered-host selection, `--ip` to force the target IP, or `-u`/`--url` to provide a complete URL. It automatically selects directory wordlists under `/usr/share/wordlists` and continues while the configured request limit allows. `web.directory.max-requests` defaults to 1,000,000 and `web.file.max-requests` defaults to 200,000. File requests include the selected extensions in the count. `web-quick`, `web-standard`, and `web-deep` are search intensities shared by directory and file searches. Use `--next` to move to the next intensity, `--force` to rerun a completed wordlist, and `--status` to show the current search state. A `--preset` or `-x` option switches to file search; explicit `-x` extensions override the preset. Gobuster checks the extensionless path together with each extension, so extensionless paths are shared with directory-search history and are not requested twice. An explicit `-w` or `--wordlist` performs a one-off search. Parsed discoveries are saved for later review through ctx logs and discovery data.

Use `-c` or `--cookies` to send cookies with Gobuster requests. Use `--exclude-length <size>` to ignore common response bodies such as wildcard 403 pages. Use `xgo --sitemap` to display the deduplicated paths collected for the current target, grouped by origin and sorted by URL. HTTP status codes are colorized in terminal output. For test targets with expired or self-signed certificates, pass `-k` or `--no-tls-validation` to disable TLS certificate validation.

`xgo` is a short alias for `xgobuster`.

`xffuf vhost` enumerates HTTP virtual hosts with ffuf. It selects the HTTP service and domain from ctx, calibrates against random hostnames, and proposes a stable response filter. A normal run applies the filter automatically when the configured confidence threshold is met. After `xffuf vhost --suggest` displays its calibration statistics, it asks whether to test the proposed filter against a real wordlist. The optional trial does not create command logs, cache progress, or register hosts. The same trial can be started directly with `xffuf vhost --trial -fw 125`. Manual ffuf filters such as `-fw`, `-fs`, `-fl`, `-fc`, and `-fr` are supported. Only a completed normal run registers results in `xhost` and updates the wordlist cache. Results that are unusually numerous are kept out of automatic registration and cache updates.

For DNS subdomain enumeration, use `xgobuster dns`. If one hostname is registered with `xhost` for the current target it is selected automatically; multiple hostnames are presented for selection. Use `-d` or `--domain` to select a domain without prompting, and `-w` or `--wordlist` for a one-off wordlist. DNS searches use a target-and-domain-specific cache, `--status` shows its progress, `--clear-cache` clears only that DNS cache, `--next` moves to the next wordlist, and `--force` reruns the first one. The default DNS query limit is controlled by `dns.max-queries` and is 10,000. Additional Gobuster DNS options can be passed after the xgobuster options.

The `xgobuster` package installs bash and zsh completion files for both commands. Start a new shell, or reload your shell completion system after installation.

Limit status or automatic escalation to one profile when needed:

```sh
xgobuster --status --profile web-quick
xgobuster --next --profile web-standard
```

## Repository Layout

```text
.
├── cmd/                 # Go command entrypoints
├── internal/            # Tool implementations and tests
├── debian/              # Per-tool package metadata and VERSION files
├── scripts/             # Build, validation, and publication scripts
├── releases/            # English and Japanese release notes
├── docs/                # Detailed ctx and API documentation
├── .github/workflows/   # Test, APT publication, and release workflows
├── go.mod
└── README.md
```

Each tool's package version is defined by `debian/<tool>/VERSION`; current version numbers are intentionally not repeated here.

## Branches and Generated Files

- `main`: Source code, tests, package definitions, scripts, and documentation.
- `dev`: Development branch. Pushes run tests only.
- `apt-repo`: Generated APT repository only. AWS Amplify publishes this branch.

The following are generated and must not be committed to `main`:

```text
dist/
repo/dists/
repo/pool/
```

## Development

Run tests in the Kali development container:

```sh
docker-compose exec -w /tools kali go test ./...
docker-compose exec -w /tools kali gofmt -w cmd internal
```

Build and install one package locally:

```sh
./scripts/install-deb.sh <tool>
```

Build a package directly for either supported architecture:

```sh
./scripts/build-deb.sh <tool> amd64
./scripts/build-deb.sh <tool> arm64
```

The output is `dist/<tool>_<version>_<architecture>.deb`. `scripts/build-apt-repo.sh` copies packages from `dist/` and regenerates `Packages` and `Packages.gz` for each architecture.

## Automation

`.github/workflows/test.yml` runs for pushes to `dev` and `main`. It checks formatting, module tidiness, tests, package structure, and every tool/package version pair. It does not publish packages.

`.github/workflows/release.yml` is the only publication workflow. A `<tool>/v<version>` tag must point to a commit contained in `main`. The workflow validates the tag and release notes, builds reproducible `amd64` and `arm64` packages from that exact commit, updates `apt-repo`, and creates the GitHub Release. An existing package version is never overwritten with different content.

`.github/workflows/audit-releases.yml` is manual-only. It audits every release tag against the APT repository, local English and Japanese release notes, and published GitHub Releases.

After validating a package with `install-deb.sh`, use this release flow:

```sh
./scripts/check-release.sh <tool>
git push origin dev

# After merging dev into main and pushing main:
git switch main
./scripts/tag-release.sh <tool>
git push origin <tool>/v<version>
```

`check-release.sh` is non-destructive: it runs tests and builds both architectures but does not install or remove packages. After publication, `check-published.sh <tool>` verifies the public APT metadata and package files. See [`docs/development.ja.md`](docs/development.ja.md) for the complete development flow and script responsibilities.

## Documentation

See `docs/` for the development workflow, ctx architecture, database, API, and add-on documentation. Run each command with `--help` for its current CLI syntax.
