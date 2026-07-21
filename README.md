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
- `xffuf`: Enumerate HTTP virtual hosts and query parameters with ffuf and ctx integration.
- `xhydra`: Assist Hydra credential discovery and save successful results to ctx.
- `xwebshell`: Select and export Kali Linux web shell templates.
- `xmagic`: Create copies with spoofed magic numbers without modifying source files.
- `xsteg`: Detect, crack, and extract data hidden in local files.

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
sudo apt install xhydra
sudo apt install xwebshell
sudo apt install xmagic
sudo apt install xsteg
```

## Usage

### req

Replay a raw HTTP request. The request method, path, host, headers, and body are read from the file.

```sh
req login.req
```

Run `req --help` for HTTPS and TLS validation options.

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

Run `ctx --help` and `ctx config ls` for the complete command and configuration references.

Install shell integration when using the `x` helpers:

```sh
ctx completion zsh
ctx completion bash
ctx init-shell
```

### ctx-integrated tools

Common connection, transfer, and discovery operations:

```sh
xssh
xssh root
xscp upload ./local.txt /tmp/remote.txt
xscp download /tmp/remote.txt ./local.txt
xftp
xftp ftpuser
xsmb
xgo
xgo dns
xweb
xffuf vhost --suggest
xffuf param -u 'http://nahamstore.thm/?FUZZ=fuga'
xffuf param -u 'http://nahamstore.thm/?hoge=FUZZ'
xweb --type param
xhydra --help
xwebshell ls
```

These tools use ctx targets, services, credentials, and other saved context as needed. Run each command with `--help` for its available operations and options.

### xmagic

Create a copy with a spoofed magic number without requiring a ctx workspace:

```sh
xmagic set gif shell.php
```

### xsteg

Scan files or directories with format-appropriate backends. For password-protected steghide payloads, xsteg tries password wordlists in the order recommended by `ctx wordlist`.

```sh
xsteg scan suspicious.png
xsteg extract suspicious.jpg       # choose only when a protected candidate is detected
xsteg extract suspicious.jpg --auto
xsteg extract suspicious.jpg --manual
xsteg
xsteg show 1
```

`scan` only detects hidden-data candidates and never extracts data or requests a passphrase. `extract` requires a completed scan and uses its same-SHA-256 result without repeating detection. It prompts for automatic analysis, a known passphrase, or skipping only when the scan found a relevant candidate. Without a scan, it tells you to run `xsteg scan <path>` first. Rejected passphrases produce `FAILED`, while only successful extraction produces `EXTRACTED`.

Only `scan` performs StegHide seed detection. It exits early when it finds a candidate but must reach 100% when no candidate exists, so xsteg displays progress while it runs.

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

## Documentation

- CLI syntax: run each command with `--help`.
- Development, validation, and release flow: [Development Guide](docs/development.md)
- Custom command integration: [ctx Integration Guide](docs/integration.md)
- JSON API: [ctx JSON API](docs/api.md)
- Database: [Database Design](docs/database.md)
- Registration commands: [ctx Registration Commands](docs/registration.md)
