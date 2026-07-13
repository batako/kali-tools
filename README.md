# Kali Tools

This monorepo manages self-made CLI tools for Kali Linux. Each tool has an independent Go entrypoint and Debian package definition, and the packages are published through an APT repository.

## Tools

- `req`: Send raw HTTP requests saved as `.req` files.
- `ctx`: Manage workspace context, targets, services, credentials, notes, and logs.
- `xssh`: Connect to the current target over SSH using ctx credentials.
- `xftp`: Connect to the current target over FTP using ctx credentials.
- `xsmb`: Discover SMB shares and connect to a selected share using ctx credentials.

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
```

Dependencies are declared by each Debian package. For example, `xssh` uses `openssh-client` and `sshpass`, `xftp` uses `lftp`, and `xsmb` uses `smbclient`.

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

`debian/<tool>/VERSION` is the source of truth for that tool's package version. Current package versions are `req 0.1.0`, `ctx 1.2.0`, and `xssh`, `xftp`, `xsmb` `1.0.0`.

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
ctx-v1.2.0
xssh-v1.0.0
xftp-v1.0.0
xsmb-v1.0.0
req-v0.1.0
```

It verifies `releases/<tool>/<version>.md`, collects the matching packages from `apt-repo`, and creates the GitHub Release. The Japanese notes use the corresponding `.ja.md` file.

For local checks:

```sh
./scripts/check-version.sh xssh
./scripts/check-release.sh xssh
./scripts/check-published.sh xssh
```

`check-release.sh` performs the heavier local Debian/APT installation checks. `check-published.sh` verifies the public APT metadata and package files after publication.

## Documentation

See `docs/` for detailed ctx architecture, database, API, and add-on documentation. Run each command with `--help` for its current CLI syntax.
