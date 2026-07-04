# Kali Tools

This repository manages self-made CLI tools for Kali Linux.

Current tool:

- `req`: send raw HTTP requests saved in `.req` files

Repository layout:

- `cmd/req/`: `req` command entrypoint
- `internal/req/`: `req` implementation
- `debian/req/`: Debian packaging files for `req`
- `repo/`: published APT repository root for AWS Amplify

APT publishing layout:

- `repo/dists/stable/main/binary-amd64/`: APT metadata for the stable component
- `repo/pool/main/r/req/`: `req` package files

AWS Amplify publishes only `repo/` by using `amplify.yml`, so the Go source tree stays in the GitHub repository but is not exposed as static hosting output.
