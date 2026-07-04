# Kali Tools

This repository contains self-made CLI tools for Kali Linux.

## Current Tool

- `req`: A CLI tool for sending raw HTTP requests stored in `.req` files.

## req Usage

```sh
req <REQ_FILE>
req -S <REQ_FILE>
req --help
```

Options:

- `-S`, `--https`: Force `https` when the request file does not imply a scheme
- `-h`, `--help`: Show help

## Directory Structure

- `cmd/req/`: Entry point for the `req` command
- `internal/req/`: Implementation and tests for `req`
- `debian/req/`: Debian packaging files for `req`
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
./scripts/build-deb.sh amd64
./scripts/build-deb.sh arm64
./scripts/build-apt-repo.sh
Force-push to the apt-repo branch
```

If any test or the `go mod tidy` check fails, the repository is not published.

## Building the Debian Package

```sh
./scripts/build-deb.sh
```

You can also specify the target architecture explicitly:

```sh
./scripts/build-deb.sh amd64
./scripts/build-deb.sh arm64
```

Output:

```text
dist/req_<version>_<architecture>.deb
```

Notes:

- If no argument is given, the target architecture is detected using `dpkg --print-architecture`.
- The corresponding Go `GOARCH` value is derived from the Debian architecture.
- The package version is read from `debian/req/VERSION`.

## Building the APT Repository

Run this after building the Debian package.

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

Install the package:

```sh
sudo apt install req
```

To remove the repository:

```sh
sudo rm -f /etc/apt/sources.list.d/batako.list
sudo apt update
```
