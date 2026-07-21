# xhydra Online Help

[日本語](./xhydra.ja.md)

`xhydra` integrates Hydra with ctx Targets, services, credentials, wordlist recommendations, and attempt history. It supports HTTP forms, SSH, FTP, and SMB. Use it only against explicitly authorized lab systems.

## Synopsis

```text
xhydra <http|ssh|ftp|smb> [options]
```

```sh
xhydra ssh -u testuser
xhydra ftp -u anonymous
xhydra smb -u smbuser -t 1
xhydra http -u admin --request login.req --fail-body 'Invalid password'
```

## Password and Username Searches

`-u, --username` performs a password search. Supplying a fixed `--password` without `-u` performs a username search. `-L, --user-list` overrides the username list; `-P, --password-list` overrides the password list.

Automatic lists come from ctx kinds `password` and `username` in priority order. State is scoped by mode, host, port, username, and related values. Repeating a scope skips attempted words. Changing the username creates a separate scope and starts from its highest-priority list. `password.max-requests` limits automatic attempts.

## SSH, FTP, and SMB

- `--host <host>`: override the Primary Target.
- `-p, --port <port>`: explicit port.
- `--service <number>`: choose a ctx service.
- `-t, --tasks <number>`: override Hydra parallel tasks.

Default ports are SSH 22, FTP 21, and SMB 445. A single matching ctx service is selected automatically; multiple matches prompt for selection. The current task default is 4 for each of SSH, FTP, and SMB. Lower it to 1 for fragile services.

SMB uses Hydra's `smb2` module. Share names such as `public` and `private` are not authentication targets. Use `xsmb` for share enumeration/file access and, where appropriate, NetExec for deeper protocol-specific checks.

## HTTP Forms

HTTP mode targets POST forms using either a raw request or URL/body template.

```sh
xhydra http -u admin -r login.req --fail-body 'Invalid credentials'
xhydra http -u admin --url http://target/login \
  --data 'username=^USER^&password=^PASS^' --fail-status 401
```

- `-r, --request <file>`: raw request in `req` format.
- `--url <url>` and `--data <body>`: construct a POST request.
- `--user-field` / `--password-field`: field names; `^USER^` and `^PASS^` placeholders are also supported.

`--request` cannot be combined with URL/body options. Authentication outcome can be defined with `--fail-json`, `--success-json`, `--fail-body`, `--success-body`, `--fail-status`, or `--success-redirect` (HTTP 302). Validate the real failure response first; a wrong condition creates false positives or misses.

## Status and Cache

```sh
xhydra ssh -u root --status
xhydra ssh -u root --clear-cache
```

Password progress operations require a username. Cache deletion is scoped, so clearing FTP does not clear SSH or another username.

## Credentials, Logs, and Troubleshooting

Successful combinations are saved to the matching ctx credential scope. Commands, status, output, and wordlist progress are logged. A password supplied directly on the command line may be visible in process listings or logs.

- SSH parallel warning: the default is already `-t 4`; lower it for restrictive targets.
- Connection refused: verify port, protocol, service state, source restrictions, and module compatibility.
- Hydra reports success but login fails: recheck the outcome condition and account restrictions.
- Restart a scope: run `--clear-cache` with the same mode/user/target.
- Inspect candidates: `ctx wordlist --kind password --usable-only`.
