# xscp Online Help

[日本語](./xscp.ja.md)

`xscp` transfers files between the local system and the ctx Primary Target over SCP, using the same SSH credential/service model as xssh.

## Synopsis

```text
xscp <upload|download> <source> [destination] [credential-id|username] [options]
```

```sh
xscp upload shell.php /tmp/shell.php testuser
xscp download /var/www/html/config.php ./config.php testuser
xscp upload report.txt --port 2222
```

`upload` interprets source as local and destination as remote. `download` does the opposite. If destination is omitted, the source basename is used.

## Credentials and Ports

Credentials come from ctx scope `ssh`. A numeric positional value selects an ID and text selects a username; omission follows the zero/one/many automatic-selection rules used by xssh. Stored passwords use `sshpass -e` and are not written into logs.

- `-p, --port <port>`: explicit SSH port.
- `--service <number>`: displayed ctx SSH service.

These options are mutually exclusive. Without either, port 22 is the fallback, one detected service is automatic, and multiple services prompt.

## Paths, Overwrites, and Logs

Paths are passed as single SCP operands. Take care with spaces, colons, and remote shell metacharacters. Recursive-directory transfer is not exposed by current xscp. Overwrite behavior follows SCP; xscp does not create backups or add a separate confirmation.

Direction, paths, username, Target, port, status, and output are logged. A successful stored credential becomes the next default. Required commands are `ctx`, `scp`, and optionally `sshpass`.

## Troubleshooting

- Credential is parsed as destination: specify destination explicitly before the credential.
- Permission denied: verify username, credential, and remote directory permissions.
- Multiple ports: use the prompt, `--service`, or `--port`.
- An existing local file was replaced: xscp has no backup facility; choose a destination explicitly.
