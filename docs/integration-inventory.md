# ctx Integration Inventory

This document records how the current binaries integrate with ctx. It is an implementation inventory, not a public API contract. The intended ownership boundary and user-facing choices are defined in the [ctx Integration Guide](integration.md).

## Integration Paths

The repository currently uses two main integration paths.

1. `xssh`, `xftp`, `xsmb`, and `xscp` execute the `ctx` process and consume versioned JSON.
2. `xgobuster`, `xhydra`, `xffuf`, and `xwebshell` import `internal/ctx` and call Go functions directly.

`xgobuster` is a hybrid: it reads prompt and service JSON through the ctx process, then uses `internal/ctx` for the remaining operations.

No `x*` command opens SQLite with SQL directly. Calls through `internal/ctx` can still read or write the database, so they are direct implementation dependencies rather than public integration contracts.

## Capability Matrix

| Capability | Public JSON process users | Direct `internal/ctx` users | Current gap |
|---|---|---|---|
| Current workspace and target | `xssh`, `xftp`, `xsmb`, `xscp`, `xgobuster` use `prompt` | `xhydra`, `xffuf`, `xwebshell` use `LoadPromptData`; discovery tools also initialize the workspace and load the primary target | Direct users depend on Go internals |
| Hosts | None | `xgobuster`, `xffuf` use `ListHosts` | No host JSON output |
| Services | `xssh`, `xftp`, `xsmb`, `xscp`, `xgobuster` use `service` | `xhydra`, `xffuf` use `ListServices` | Two implementations of service loading and selection |
| Credentials read | `xssh`, `xftp`, `xsmb`, `xscp` use `credential` | None | Four copies of JSON response and filtering logic |
| Credentials write | None | `xhydra` uses `SetCredential` | Existing CLI registration is not used by the internal command |
| Hosts write | None | `xgobuster`, `xffuf` use `AddHost` | Existing CLI registration is not used by the internal commands |
| Services write | None | `ctx scan` uses `UpsertService`; no separate `x*` writer | No public service registration operation |
| Notes | None | None | Available to custom commands through the normal `ctx note` command only |
| Command logs | `xssh`, `xftp`, `xsmb`, `xscp` use `log start/finish` | `xgobuster`, `xhydra`, `xffuf` use `StartCommandLog` and `FinishCommandLog` | JSON logger is duplicated; direct users bypass it |
| Web discoveries | None | `xgobuster` saves and lists discoveries | No public web-discovery operation |
| Web wordlist run history | None | `xgobuster`, `xffuf` start, finish, and list runs | No public run-history operation |
| Configuration | None | `xgobuster`, `xhydra`, `xffuf` use `LoadConfig` | No machine-readable config API; consumers depend on the Go struct |
| Wordlist discovery | None | `xgobuster`, `xhydra`, `xffuf` use ctx wordlist helpers | Selection policy and ctx persistence are coupled in one internal package |
| Feature and format discovery | None | None | No add-on currently calls `ctx formats` before using an endpoint |

The absence of a public operation is an inventory result, not an instruction to add one. A new operation is justified only by a concrete integration that cannot use an existing command safely.

## Command Inventory

### `xssh`

- JSON reads: `prompt`, `credential ls ssh`, `service ls`
- JSON writes: `log start`, `log finish`
- JSON client: shared `internal/ctxapi`
- Normal human-output ctx commands: none
- `internal/ctx`: none
- ctx configuration: none
- Private state: last credential ID in `${XDG_STATE_HOME:-$HOME/.local/state}/xssh/state`
- Shared findings written: command logs only

### `xftp`

- JSON reads: `prompt`, `credential ls ftp`, `service ls`
- JSON writes: `log start`, `log finish`
- JSON client: shared `internal/ctxapi`
- Normal human-output ctx commands: none
- `internal/ctx`: none
- ctx configuration: none
- Private state: last credential ID in `${XDG_STATE_HOME:-$HOME/.local/state}/xftp/state`
- Shared findings written: command logs only

### `xsmb`

- JSON reads: `prompt`, `credential ls smb`, `service ls`
- JSON writes: `log start`, `log finish`
- JSON client: shared `internal/ctxapi`
- Normal human-output ctx commands: none
- `internal/ctx`: none
- ctx configuration: none
- Private state: last credential ID in `${XDG_STATE_HOME:-$HOME/.local/state}/xsmb/state`
- Shared findings written: command logs only

### `xscp`

- JSON reads: `prompt`, `credential ls ssh`, `service ls`
- JSON writes: `log start`, `log finish`
- JSON client: shared `internal/ctxapi`
- Normal human-output ctx commands: none
- `internal/ctx`: none
- ctx configuration: none
- Private state: reuses the xssh last-credential state at `${XDG_STATE_HOME:-$HOME/.local/state}/xssh/state`
- Shared findings written: command logs only

### `xgobuster`

- JSON reads: `prompt`, `service ls`
- JSON client: shared `internal/ctxapi`
- JSON writes: none
- Normal human-output ctx commands: none
- `internal/ctx` reads: workspace, primary target, hosts, configuration, wordlist definitions, web discoveries, web wordlist run history
- `internal/ctx` writes: hosts, command logs, web discoveries, web wordlist run history
- ctx configuration: `web.directory.max-requests`, `web.file.max-requests`, `dns.max-queries`, `web.tls.verify`
- Private workspace state: searched words for web and DNS, extension coverage, active web strategy
- Temporary state: filtered wordlists
- Shared findings written: hosts and web discoveries; command and wordlist run history

### `xhydra`

- JSON process calls: none
- Normal human-output ctx commands: none
- `internal/ctx` reads: prompt data, workspace, primary target, services, configuration, password and username wordlist definitions
- `internal/ctx` writes: credentials and command logs
- ctx configuration: `password.max-requests`
- Private workspace state: searched password and username words scoped by target, service, endpoint, and fixed input
- Temporary state: filtered password or username lists
- Shared findings written: successful credentials and command logs

### `xffuf`

- JSON process calls: none
- Normal human-output ctx commands: none
- `internal/ctx` reads: prompt data, workspace, primary target, hosts, services, configuration, wordlist definitions
- `internal/ctx` writes: hosts, command logs, web wordlist run history
- ctx configuration: `web.vhost.max-requests`, `web.vhost.calibration-samples`, `web.vhost.calibration-confidence`, `web.tls.verify`
- Private workspace state: searched vhost words scoped by target, URL, and domain
- Temporary state: calibration wordlists, filtered wordlists, ffuf JSON result files
- Shared findings written: confirmed hosts, command logs, and web wordlist run history; trial mode writes none

### `xwebshell`

- JSON reads: `prompt`
- JSON client: shared `internal/ctxapi`
- Normal human-output ctx commands: none
- `internal/ctx`: none
- ctx configuration: none
- Private state: none
- External files: reads system webshell templates and writes configured output to the current directory
- Shared findings written: none

### `req`

`req` is intentionally independent. It does not execute ctx, import `internal/ctx`, read ctx configuration, or use ctx workspace state.

## Current Duplication

The connection and transfer commands now share their JSON transport through `internal/ctxapi`. The following tool-specific code intentionally remains separate:

- prompt, credential, and service data structs;
- protocol-specific service filtering and command construction;
- child-process error translation; and
- last-credential state handling, with xscp intentionally sharing xssh state.

Service filtering and interactive selection are also repeated across several tools, although protocol-specific matching rules differ.

`internal/ctxapi` provides the shared process client for the existing versioned JSON APIs. It appends the JSON format arguments, sends optional JSON through stdin, validates the response envelope and format version, preserves structured API errors, and distinguishes malformed JSON, missing data, and process failures. `xssh` uses it for prompt, credential, service, and log operations.

Process-based ctx calls use `internal/ctxexec`, which executes the fixed absolute path `/usr/local/bin/ctx` without a shell. The path can be replaced at build time for another distribution and injected in tests.

## Current Inconsistencies and Gaps

- No add-on checks endpoint availability through `ctx formats` before issuing its first JSON request.
- Process-based add-ons depend on public JSON, while discovery and authentication tools depend on `internal/ctx`, creating different compatibility boundaries for official commands.
- Hosts, configuration, web discoveries, and web wordlist history have no current public JSON operation, which prevents a direct migration without first evaluating a concrete interface.
- Official writers call `AddHost` or `SetCredential` directly instead of using existing human-facing registration commands. Those commands do not yet provide a complete safe machine-input contract for every use case.
- No `x*` command parses a human-readable ctx table, which is the desired current behavior.
- No `x*` command executes SQL directly, which avoids duplicating schema knowledge outside `internal/ctx`.

These findings provide input for later TODO items. They do not by themselves require all official commands to migrate to process-based APIs.

## Migration Decisions

| Command | Decision | Reason |
|---|---|---|
| `xftp` | Migrate prompt, credential, service, and log operations to `internal/ctxapi` | Every ctx operation already uses the public versioned JSON contract; the migration removes duplicated transport code without changing behavior |
| `xsmb` | Migrate prompt, credential, service, and log operations to `internal/ctxapi` | Same complete public-API boundary as xssh, with SMB-specific selection remaining local |
| `xscp` | Migrate prompt, credential, service, and log operations to `internal/ctxapi` | Same complete public-API boundary as xssh; xssh credential-state sharing remains tool-specific |
| `xgobuster` | Keep prompt and service reads on `internal/ctxapi`; retain other `internal/ctx` operations | These reads use public JSON, while hosts, configuration, logs, discoveries, and wordlist history have no equivalent complete public contract |
| `xhydra` | Keep `internal/ctx` for now | Its workflow combines configuration, wordlist policy, scoped cache state, command logs, and credential writes; partial process migration would add a second boundary without removing the internal dependency |
| `xffuf` | Keep `internal/ctx` for now | Its workflow combines configuration, services, hosts, calibration state, logs, host writes, and wordlist history; the required public operations are incomplete |
| `xwebshell` | Use `internal/ctxapi` for prompt reads | It only needs public prompt data for callback-IP selection and has no `internal/ctx` dependency |

The retained direct dependencies are explicit implementation choices, not public APIs for custom commands. They should be reconsidered only when a concrete public operation can replace the complete workflow rather than merely moving one read.
