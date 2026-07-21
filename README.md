# Kali Tools

This monorepo contains custom CLI tools for Kali Linux. Each tool has its own Go entrypoint and Debian package definition and can be installed from the APT repository.

## Tools

Usage and specifications are maintained in the command documentation.

- [req](docs/commands/req.md)
- [ctx](docs/commands/ctx.md)
- [xssh](docs/commands/xssh.md)
- [xscp](docs/commands/xscp.md)
- [xftp](docs/commands/xftp.md)
- [xsmb](docs/commands/xsmb.md)
- [xgobuster](docs/commands/xgobuster.md)
- [xffuf](docs/commands/xffuf.md)
- [xhydra](docs/commands/xhydra.md)
- [xwebshell](docs/commands/xwebshell.md)
- [xmagic](docs/commands/xmagic.md)
- [xsteg](docs/commands/xsteg.md)

## Installation

Add the public repository once:

```sh
echo "deb [trusted=yes] https://offsec.batako.net stable main" \
  | sudo tee /etc/apt/sources.list.d/batako-offsec.list
sudo apt update
```

Install the tools you need:

```sh
sudo apt install req ctx xssh xscp xftp xsmb xgobuster xffuf xhydra xwebshell xmagic xsteg
```

## Repository Layout

```text
.
├── cmd/                 # Go command entrypoints
├── internal/            # Tool implementations and tests
├── debian/              # Per-tool package metadata and VERSION files
├── scripts/             # Build, validation, and publication scripts
├── releases/            # English and Japanese release notes
├── docs/
│   └── commands/        # Per-command English and Japanese documentation
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

- Command usage and specifications: [Command Documentation](docs/commands/)
- Development, validation, and release flow: [Development Guide](docs/development.md)
- Custom command integration: [ctx Integration Guide](docs/integration.md)
- JSON API: [ctx JSON API](docs/api.md)
- Database: [Database Design](docs/database.md)
