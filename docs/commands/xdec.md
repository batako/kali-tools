# xdec

`xdec` is a unified analysis frontend for literal values, files, and stdin. With no arguments it prints root help; when input is present, the `decode` subcommand may be omitted.

## Usage

```sh
# Root help
xdec
xdec help
xdec --help

# Version
xdec version
xdec -V
xdec --version

# Pass a literal value
xdec decode 'QXJlYTUx'
xdec decode --string 'QXJlYTUx'
xdec 'QXJlYTUx'

# Read a file explicitly
xdec decode -f hashes.txt

# An existing regular file can be passed without -f
xdec decode ~/.ssh/id_ed25519
xdec ~/.ssh/id_ed25519

# Read from stdin
some-command | xdec decode --yes -w wordlist.txt
```

Options may also appear after the positional input:

```sh
xdec decode ~/.ssh/id_ed25519 --refresh --yes -w wordlist.txt
```

## Subcommands

| Subcommand | Description |
| --- | --- |
| `decode` | Decode values and recover passwords |
| `help [SUBCOMMAND]` | Show root help or help for the selected subcommand |
| `version` | Show version |

## Root options

| Option | Description |
| --- | --- |
| `-h`, `--help` | Show root help |
| `-V`, `--version` | Show version |
| `--online-help` | Show the versioned online help URL |

## Decode arguments and options

An existing regular-file positional input is read as a file; any other positional input is treated as a literal string. Use `--file` or `--string` to disambiguate. Only one positional input is allowed, and it cannot be combined with an explicit type flag. With no explicit or positional input, xdec reads stdin.

| Option | Description |
| --- | --- |
| `-f`, `--file FILE` | Read FILE as input; an existing positional file can also be used |
| `--string VALUE` | Treat VALUE as a string; cannot be combined with positional input |
| `-w`, `--wordlist SPEC` | ctx wordlist ID or path; may be repeated |
| `--scope SCOPE` | Scope used when saving a credential |
| `--username USER` | Username when the input does not contain one |
| `--save-credential` | Save a recovered credential to ctx |
| `--no-save-credential` | Disable automatic credential saving |
| `--yes` | Approve expensive recovery |
| `--refresh` | Discard saved state for the current input and analyze it again |
| `--dry-run` | Show the execution plan only |
| `--json` | Emit JSON results |
| `-h`, `--help` | Show decode help |

## Analysis flow

Base64 and hex values are decoded immediately. MD5, NTLM, MD4, SHA-1, SHA-256, bcrypt, and Argon2-prefixed values are classified and require confirmation when password recovery is needed.

Without `-w`, xdec uses the ctx password-wordlist set. Multiple wordlists are tried sequentially, continuing automatically when an earlier list does not recover the value. The confirmation prompt summarizes the number of lists instead of printing every path.

For piped or otherwise non-interactive input, confirmation cannot be answered through stdin. Use `--yes`:

```sh
cat md5.txt | xdec decode --yes -w wordlist.txt
```

## SSH private keys

SSH private-key files such as `id_rsa`, `id_ecdsa`, and `id_ed25519` can be passed directly. xdec checks whether the key is encrypted. An unencrypted key is not sent to John:

```text
xdec: SSH private key is not encrypted
xdec: no password required
```

Only encrypted keys are converted with `ssh2john` and passed to John for passphrase recovery:

```sh
xdec decode --yes -w wordlist.txt -f ~/.ssh/id_rsa
```

Private-key contents and recovered passphrases are not written to xlog.

## Usernames and credentials

xdec extracts usernames from both `admin:HASH` and `admin HASH` formats.

```sh
cat command-output.txt | xdec decode --scope ssh --yes -w wordlist.txt
```

Text output includes the username but not the scope:

```text
admin: password
alice: password
```

Recovered values are automatically saved to ctx credentials when both username and scope are known. Pure hashes or results without a scope are not saved automatically.

## State and logs

Execution history is available through `xlog` / `ctx log`. xdec creates one parent log and a child step for each wordlist.

When the same input is run again, completed wordlists are skipped and cached recovered values are displayed. Adding a new wordlist tries only that new list.

To discard the saved state for only the current input:

```sh
xdec decode --refresh --yes -w wordlist.txt -f bcrypt.txt
```

State is stored at `data/xdec/state.json` in the ctx workspace, or in the user cache outside a workspace, with mode 0600. It contains recovered plaintext and must be treated as sensitive.
