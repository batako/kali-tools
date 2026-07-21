# xsmb Online Help

[日本語](./xsmb.ja.md)

`xsmb` enumerates SMB shares on the ctx Primary Target and opens the selected share in smbclient.

## Synopsis

```text
xsmb [credential-id|username]
```

## Processing Model

xsmb loads the Primary Target and matching SMB service, performs anonymous `smbclient -L` discovery, filters for Disk shares and excludes `IPC$`, prompts when several remain, then connects with the selected credential and logs the session result.

Share discovery is currently anonymous. A server that prohibits anonymous enumeration can therefore stop xsmb before connection even when a valid credential is stored.

## Target, Port, and Credentials

The fallback port is 445. One matching `smb`/`microsoft-ds` service is automatic; multiple services prompt. Current xsmb has no host/port option.

Credentials come from scope `smb`. No credential means anonymous `-N`; one is automatic; multiple prompt; a number selects an ID; text selects a username. An unknown username lets smbclient prompt. Stored passwords use the `PASSWD` environment variable and are omitted from logs.

After connection, use smbclient commands such as `ls`, `cd`, `get`, `put`, `recurse ON`, `prompt OFF`, and `exit`.

## Relationship to xhydra

`xhydra smb` tests username/password combinations against SMB2 authentication. `xsmb` enumerates and accesses shares. `public` and `private` are share names, not Hydra authentication destinations.

## Logging and Troubleshooting

The selected share, username, Target, port, status, and output are logged. Required commands are `ctx` and `smbclient`.

- No shares found: anonymous enumeration may be prohibited; verify with `smbclient -L //<IP> -U <user>`.
- Port 445 is open but login fails: inspect dialect, authentication method, and server policy.
- A share must be supplied directly: use smbclient; current xsmb selects from discovery.
