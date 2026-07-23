# xdec test data

All hashes in this directory correspond to the plaintext `password`. These
values are for testing only.

## Immediate decoding

```sh
xdec base64.txt
# password

xdec hex.txt
# password
```

## Hash detection and confirmation

```sh
xdec md5.txt
xdec sha1.txt
xdec sha256.txt
xdec ntlm.txt
xdec bcrypt.txt
```

Recovery normally asks for confirmation because it may take a long time.
Without `-w`, xdec tries the available password wordlists recommended by ctx.
To use the test wordlist explicitly:

```sh
xdec --yes -w wordlist.txt md5.txt
```

## Hashes with usernames

```sh
xdec --scope ssh --yes -w wordlist.txt user-hashes.txt
```

When a ctx workspace and primary target are configured, successfully recovered
credentials are saved automatically when `--scope ssh` is provided.

```sh
xcredential ls ssh
xlog
```

## Multi-line command output

```sh
cat command-output.txt | xdec --dry-run
cat command-output.txt | xdec --scope ssh --yes -w wordlist.txt
```

## Parent and child logs for multiple wordlists

```sh
xdec --yes \
  -w wordlist-empty.txt \
  -w wordlist.txt \
  user-hashes.txt
```

Use `xlog` to verify that xdec creates a parent log with a child step for each
wordlist.

## Non-interactive stdin

```sh
cat md5.txt | xdec -w wordlist.txt
# --yes is required because piped input cannot answer the confirmation prompt

cat md5.txt | xdec --yes -w wordlist.txt

# Discard saved state and analyze only this input again
xdec --refresh --yes -w wordlist.txt md5.txt
```

## SSH private keys

SSH private keys such as `id_rsa` can be passed directly. xdec detects whether
the key is encrypted. Unencrypted keys are not cracked; only encrypted keys
are converted through `ssh2john` for passphrase recovery.

```sh
xdec --yes -w wordlist.txt ~/.ssh/id_rsa
```

The encrypted-key fixtures use the passphrase `password`. Unencrypted fixtures
must report that no password is required and must not run John.

Automatic routing:

```sh
xdec id_ed25519_encrypted
xdec id_ed25519_unencrypted
xdec id_rsa_encrypted
xdec id_rsa_unencrypted
```

`decode` performs immediate transformations only. When given a hash or an
encrypted private key, it does not start recovery and reports
`recover-required` instead.
