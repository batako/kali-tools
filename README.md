# Kali Tools

This repository manages self-made CLI tools for Kali Linux.

Current tool:

- `req`: send raw HTTP requests saved in `.req` files

Repository layout:

- `cmd/req/`: `req` command entrypoint
- `internal/req/`: `req` implementation
- `debian/req/`: Debian packaging files for `req`
- `scripts/`: project helper scripts
- `.github/workflows/`: test and APT publish workflows

Generated paths ignored on `main`:

- `dist/`
- `repo/dists/`
- `repo/pool/`

APT publishing layout:

- `apt-repo:dists/`: APT metadata
- `apt-repo:pool/`: package files

Branch roles:

- `main`: source code, tests, packaging definitions
- `apt-repo`: published APT repository contents only

Publish flow:

- Push to `main`
- GitHub Actions runs `go test ./...`
- GitHub Actions builds `dist/req_<version>_<architecture>.deb`
- GitHub Actions updates `repo/`
- GitHub Actions force-pushes only the generated repository files to `apt-repo`

AWS Amplify should target the `apt-repo` branch root, so the Go source tree on `main` is never published.

Debian package build:

- Run `./scripts/build-deb.sh`
- Output: `dist/req_0.1.0_<architecture>.deb`
- Current Debian architecture is detected with `dpkg --print-architecture`
- Package version is read from `debian/req/VERSION`

APT repository build:

- Run `./scripts/build-apt-repo.sh` after `./scripts/build-deb.sh`
- Metadata output: `repo/dists/stable/main/binary-<architecture>/Packages`
- Compressed metadata: `repo/dists/stable/main/binary-<architecture>/Packages.gz`
