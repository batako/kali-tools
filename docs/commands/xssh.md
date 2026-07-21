# xssh Online Help

[日本語](./xssh.ja.md)

`xssh` connects to the ctx Primary Target using detected SSH services and stored credentials, then records connection metadata in ctx.

## Synopsis

```text
xssh [credential-id|username|key]
```

The destination is the Primary Target IP. If ctx has no SSH service, port 22 is used; one service is automatic; multiple services prompt for selection. xssh has no host or port option—switch the Primary Target or update service data when needed.

## Credential Selection

Credentials come from scope `ssh`. With no argument, zero credentials starts plain SSH, one is automatic, and multiple prompt with the last successful ID as default. A numeric argument selects an ID; text selects a username. An unknown username is still used without a stored password.

Stored passwords are passed to `sshpass` through `SSHPASS`, not embedded in the logged command. Connections use `StrictHostKeyChecking=accept-new`: new keys are accepted, but changed known keys remain an error.

## `xssh key`

Ensures `~/.ssh/id_ed25519.pub` exists, invoking `ssh-keygen -t ed25519` when necessary, and prints a command to run on the target. That command creates `authorized_keys` with safe permissions and avoids duplicate keys. It does not connect or upload the key automatically.

## Logging and Requirements

ctx records username, host, port, status, and exit code. Interactive SSH input/output is intentionally not stored. Required commands are `ctx`, `ssh`, and `sshpass` for stored passwords.

## Troubleshooting

- No workspace/target: initialize a workspace and select a Target.
- Changed host key: verify whether the IP was reused or the connection is being intercepted before editing known_hosts.
- Plain ssh works but xssh fails: compare ctx Target, selected username, and detected port.
- A direct custom port is required: use plain `ssh -p`; current xssh has no port option.
