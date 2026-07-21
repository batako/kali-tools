# xftp Online Help

[日本語](./xftp.ja.md)

`xftp` opens lftp against the ctx Primary Target using stored FTP credentials and detected services.

## Synopsis

```text
xftp [credential-id|username]
```

The Target is the Primary Target IP. With no detected FTP service, port 21 is used; one service is automatic; multiple services prompt. Current xftp has no host/port options.

## Credentials and Connection

Credentials come from scope `ftp`. With no credential, xftp attempts anonymous access. One stored credential is automatic; multiple prompt with the last successful ID as default. Numbers select IDs and text selects usernames; an unknown username is attempted without a stored password.

Stored passwords are passed through `LFTP_PASSWORD`. The logged command excludes them. xftp sets `net:max-retries 0`; authenticated connections issue `NOOP` so an initial authentication failure is not mistaken for a successful session.

Inside lftp, use commands such as `ls`, `pwd`, `get`, `put`, `mirror`, and `exit`. Transfer paths and overwrite behavior follow lftp.

## Logging, Requirements, and Problems

ctx saves username, Target, port, status, and output. Required commands are `ctx` and `lftp`.

- Anonymous access fails: register an FTP credential and select it.
- A different port is needed: ensure `ctx scan` detects it or use lftp directly.
- A password prompt appears: the selected credential has no stored password.
