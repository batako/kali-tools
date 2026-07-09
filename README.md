# Kali Tools

This repository contains self-made CLI tools for Kali Linux.

## Current Tool

- `req`: A CLI tool for sending raw HTTP requests stored in `.req` files.
- `ctx`: A CLI tool for managing workspace context for targets and hosts.

## req Usage

```sh
req <REQ_FILE>
req -S <REQ_FILE>
req --help
req --version
req -V
```

Options:

- `-S`, `--https`: Force `https` when the request file does not imply a scheme
- `-h`, `--help`: Show help
- `-V`, `--version`: Show version

## ctx Usage

```sh
ctx workspace init
ctx status
ctx workspace ls
ctx workspace rm [id]
ctx note "SMB anonymous login possible"
ctx log
ctx log <id>
ctx prompt
ctx prompt --field target-ip
ctx prompt --format json
ctx x <command> [args...]
ctx --help
ctx --version
ctx -V
x <command> [args...]
```

`ctx x` runs the given command in the current ctx workspace, streams stdout/stderr to the terminal, and saves the command, expanded command, exit code, timestamps, stdout, and stderr to `ctx log`. If an argument contains `$IP` or `${IP}`, it is expanded to the current primary target IP before execution. After `ctx init-shell`, the `x` helper function is available as the short form of `ctx x`.

`ctx note <text>` saves a note as a `note:<id>` entry in the `ctx log` timeline. After `ctx init-shell`, use `xnote <text>` as its short form.

On a terminal, `ctx log` opens an interactive timeline. Use `j`/`k` or the arrow keys to move, Enter to open command details, and `q` to return or quit. Use `-p`/`--plain` for a compact text timeline, `-v`/`--verbose` for IDs and execution status, or `-i`/`--interactive` to request the TUI explicitly.

`ctx prompt` prints safely quoted shell variables for prompt integrations. It includes workspace, local interface/IP, and primary target data. Outside a workspace, `CTX_ACTIVE` is `0`. A minimal Powerlevel10k custom segment for `.p10k.zsh` is:

```zsh
function prompt_ctx() {
  eval "$(ctx prompt)" || return
  (( CTX_ACTIVE )) || return
  p10k segment -t "${CTX_LOCAL_IP} -> ${CTX_TARGET_IP}"
}
```

Add `ctx` to `POWERLEVEL9K_LEFT_PROMPT_ELEMENTS` or `POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS`, then choose colors, icons, and formatting in the segment as desired. Use `ctx prompt --field <name>` for one value or `ctx prompt --format json` for structured output.

`ctx workspace rm` removes the current workspace's marker, database records, and data directory after confirmation. Outside a workspace, it lists the registered workspaces for selection. Pass an ID to select one directly, or add `--yes` to skip confirmation.

## ctx Shell Setup

```sh
ctx completion zsh
ctx completion bash
ctx init-shell
ctx init-shell --remove
ctx doctor
```

`ctx completion zsh` and `ctx completion bash` only print shell scripts to stdout. They do not edit shell rc files.

`ctx init-shell` detects the current shell and writes a marked ctx block to `.zshrc` or `.bashrc`. It also enables x-prefixed helper functions, so `ctx workspace init` can be run as `xinit`, `ctx status` as `xstatus`, and `ctx hosts` as `xhosts`. ctx does not create aliases.

## Directory Structure

- `cmd/req/`: Entry point for the `req` command
- `internal/req/`: Implementation and tests for `req`
- `debian/req/`: Debian packaging files for `req`
- `debian/ctx/`: Debian packaging files for `ctx`
- `scripts/`: Build and publishing scripts
- `.github/workflows/`: GitHub Actions workflows

## Branch Roles

- `main`: Source code branch. A push to this branch triggers testing and publishes the APT repository if all checks pass.
- `apt-repo`: Published APT repository

The `apt-repo` branch contains only the generated APT repository. AWS Amplify publishes this branch only.

## Generated Files Not Tracked on `main`

- `dist/`
- `repo/dists/`
- `repo/pool/`

These paths are listed in `.gitignore` and must not be committed to the `main` branch.

## GitHub Actions

### `test.yml`

Runs on every `push` and executes:

```sh
go mod tidy
git diff --exit-code
go test ./...
./scripts/check-version.sh ctx
```

### `publish-apt-repo.yml`

Runs only when `main` is updated and executes the following steps:

```text
go mod tidy
git diff --exit-code
go test ./...
./scripts/check-version.sh ctx
./scripts/build-deb.sh req amd64
./scripts/build-deb.sh req arm64
./scripts/build-deb.sh ctx amd64
./scripts/build-deb.sh ctx arm64
Restore the existing apt-repo branch into repo/
./scripts/build-apt-repo.sh
Force-push to the apt-repo branch
```

If any test or the `go mod tidy` check fails, the repository is not published.

## Building the Debian Package

```sh
./scripts/build-deb.sh
./scripts/build-deb.sh ctx
```

You can also specify the package and target architecture explicitly:

```sh
./scripts/build-deb.sh req amd64
./scripts/build-deb.sh req arm64
./scripts/build-deb.sh ctx amd64
./scripts/build-deb.sh ctx arm64
```

Output:

```text
dist/<package>_<version>_<architecture>.deb
```

Notes:

- If no argument is given, the target architecture is detected using `dpkg --print-architecture`.
- The corresponding Go `GOARCH` value is derived from the Debian architecture.
- The package defaults to `req` when omitted.
- The `ctx` package installs `ctx`; `x` is provided by shell integration as a helper for `ctx x`.
- The package version is read from `debian/<package>/VERSION`.
- `./scripts/check-version.sh ctx` checks that `debian/ctx/VERSION` matches `internal/ctx.Version`.

## Release Checks

Before a release, run the combined version, Go module, test, Debian package, and packaged executable checks. The script APT-installs the `.deb` on the current Kali Linux system, checks basic operation, `postinst`, and removal, then reinstalls it. After installation, the installed ctx tests the current `.zshrc` or `.bashrc` for removal, registration, idempotency, updates, and loading. This requires administrator privileges through `sudo`. A fully successful check leaves the ctx configuration in place; failures and interruptions restore the original contents and modification time. When run from a terminal, success starts a shell with the updated configuration loaded so interactive checks can begin immediately:

```sh
./scripts/check-release.sh ctx
```

After publishing, verify the amd64/arm64 APT metadata and `.deb` files on `apt.batako.net`. HTTP requests are retried to allow for propagation delays. Updating from the public APT repository and installing a specific version are printed as `TODO` items.

```sh
./scripts/check-published.sh ctx
```

To check another repository:

```sh
APT_REPOSITORY_URL=https://example.net ./scripts/check-published.sh ctx
```

## Building the APT Repository

Run this after building the Debian package. The script copies new packages from `dist/` into `repo/pool/` without deleting existing packages, then regenerates metadata from every `.deb` stored in `repo/pool/` using `dpkg-scanpackages --multiversion`.

```sh
./scripts/build-apt-repo.sh
```

Output:

```text
repo/dists/stable/main/binary-all/Packages
repo/dists/stable/main/binary-all/Packages.gz
repo/dists/stable/main/binary-amd64/Packages
repo/dists/stable/main/binary-amd64/Packages.gz
repo/dists/stable/main/binary-arm64/Packages
repo/dists/stable/main/binary-arm64/Packages.gz
repo/pool/main/r/req/req_<version>_amd64.deb
repo/pool/main/r/req/req_<version>_arm64.deb
repo/pool/main/c/ctx/ctx_<version>_amd64.deb
repo/pool/main/c/ctx/ctx_<version>_arm64.deb
```

## Using the APT Repository

Add the repository:

```sh
echo "deb [trusted=yes] https://apt.batako.net stable main" \
| sudo tee /etc/apt/sources.list.d/batako.list
```

Update the package index:

```sh
sudo apt update
```

Install the latest package:

```sh
sudo apt install req
sudo apt install ctx
```

Install a specific version:

```sh
sudo apt install ctx=0.1.0
sudo apt install req=0.2.3
```

To remove the repository:

```sh
sudo rm -f /etc/apt/sources.list.d/batako.list
sudo apt update
```
