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
ctx init
ctx status
ctx --help
ctx --version
ctx -V
```

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
```

### `publish-apt-repo.yml`

Runs only when `main` is updated and executes the following steps:

```text
go mod tidy
git diff --exit-code
go test ./...
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
- The package version is read from `debian/<package>/VERSION`.

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
