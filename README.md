# Kali Tools

This monorepo manages self-made CLI tools for Kali Linux. Each tool has an independent Go entrypoint and Debian package definition, and the packages are published through an APT repository.

## Tools

- `req`: Send raw HTTP requests saved as `.req` files.
- `ctx`: Manage workspace context, targets, services, credentials, notes, and logs.
- `xssh`: Connect to the current target over SSH using ctx credentials.
- `xftp`: Connect to the current target over FTP using ctx credentials.
- `xsmb`: Discover SMB shares and connect to a selected share using ctx credentials.
- `xgobuster`: Run Gobuster against the current target and save web discoveries to ctx.

`xssh`, `xftp`, and `xsmb` are ctx add-ons. They use the ctx JSON API and do not read the ctx SQLite database directly.

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
sudo apt install xftp
sudo apt install xsmb
sudo apt install xgobuster
```

Dependencies are declared by each Debian package. For example, `xssh` uses `openssh-client` and `sshpass`, `xftp` uses `lftp`, `xsmb` uses `smbclient`, and `xgobuster` uses `gobuster` and `wordlists`.

## Usage

### req

Replay a raw HTTP request. The request method, path, host, headers, and body are read from the file.

```sh
req login.req
req -S login.req
```

`-S`/`--https` forces HTTPS when the request file does not determine a scheme. `-h`/`--help` and `-V`/`--version` are also available. `Accept-Encoding` and `Content-Length` from the request file are not sent; Go's `net/http` handles gzip response decompression.

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
sudo apt install wordlists seclists dirb dirbuster
ctx config set web.directory.max-requests 1000000
ctx config set web.file.max-requests 200000
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
xftp
xftp ftpuser
xsmb
xsmb smbuser
```

The add-ons save connection start/end times, status, exit code, sanitized command, stdout, and stderr to ctx logs. Review them with `xlog`. Passwords are not stored in the command log.

`xsmb` lists disk shares, excludes `IPC$`, and lets you select the share before connecting. `xssh` defaults to port 22, `xftp` to port 21, and `xsmb` to port 445 when ctx has no matching service record.

`xgobuster` selects a web service on the current target and runs `gobuster dir`. When `xhost` has manually registered hostnames for the target, one hostname is used automatically; if multiple hostnames exist, `xgobuster` prompts for a hostname or the target IP. Use `--host <hostname>` for a deterministic registered-host selection, `--ip` to force the target IP, or `-u`/`--url` to provide a complete URL. It automatically selects directory wordlists under `/usr/share/wordlists` and continues while the configured request limit allows. `web.directory.max-requests` defaults to 1,000,000 and `web.file.max-requests` defaults to 200,000. File requests include the selected extensions in the count. `web-quick`, `web-standard`, and `web-deep` are search intensities shared by directory and file searches. Use `--next` to move to the next intensity, `--force` to rerun a completed wordlist, and `--status` to show the current search state. A `--preset` or `-x` option switches to file search; explicit `-x` extensions override the preset. Gobuster checks the extensionless path together with each extension, so extensionless paths are shared with directory-search history and are not requested twice. An explicit `-w` or `--wordlist` performs a one-off search. Parsed discoveries are saved for later review through ctx logs and discovery data.

Use `xgo --sitemap` to display the deduplicated paths collected for the current target, grouped by origin and sorted by URL. HTTP status codes are colorized in terminal output. For test targets with expired or self-signed certificates, pass `-k` or `--no-tls-validation` to disable TLS certificate validation.

`xgo` is a short alias for `xgobuster`.

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
./scripts/install-deb.sh ctx
./scripts/install-deb.sh xssh
./scripts/install-deb.sh xftp
./scripts/install-deb.sh xsmb
```

Build a package directly for either supported architecture:

```sh
./scripts/build-deb.sh xssh amd64
./scripts/build-deb.sh xssh arm64
```

The output is `dist/<tool>_<version>_<architecture>.deb`. `scripts/build-apt-repo.sh` copies packages from `dist/` and regenerates `Packages` and `Packages.gz` for each architecture.

## Automation

`.github/workflows/test.yml` runs on every push and checks module tidiness, tests, and tool/package version consistency.

`.github/workflows/publish-apt-repo.yml` runs only for pushes to `main`. It runs the same checks, builds `amd64` and `arm64` packages for all tools, regenerates the APT repository, and force-pushes only `repo/` contents to `apt-repo`. A failed check prevents publication.

`.github/workflows/publish-release.yml` runs for version tags such as:

```text
ctx-v<version>
xssh-v<version>
xftp-v<version>
xgobuster-v<version>
xsmb-v<version>
req-v<version>
```

It verifies `releases/<tool>/<version>.md`, collects the matching packages from `apt-repo`, and creates the GitHub Release. The Japanese notes use the corresponding `.ja.md` file.

For local checks:

```sh
./scripts/check-version.sh xssh
./scripts/check-version.sh xgobuster
./scripts/check-release.sh xssh
./scripts/check-published.sh xssh
```

`check-release.sh` performs the heavier local Debian/APT installation checks. `check-published.sh` verifies the public APT metadata and package files after publication.

## Documentation

See `docs/` for detailed ctx architecture, database, API, and add-on documentation. Run each command with `--help` for its current CLI syntax.
