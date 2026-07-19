# ctx Integration Guide

`ctx` does not attempt to implement every tool needed for THM or other security labs. Instead, it provides integration points that let independent commands reuse the current investigation context and saved findings.

This guide explains how to choose an integration level. It does not require a specific programming language, SDK, or add-on framework.

## Choose an Integration Level

| Need | Integration | Use when |
|---|---|---|
| No ctx data | Independent command | The command does not need the current target or saved findings |
| Run and record an existing command | `ctx x` | Whole-process logging and target IP expansion are sufficient |
| Read context or saved findings | Normal ctx commands and JSON output | The command needs targets, services, or credentials |
| Save a finding | Existing ctx registration commands | A matching command such as `ctx host add` already exists |
| Control a command log | `ctx log start/finish` | The command must control the logged command, output, status, or timing |
| Perform unsupported queries or prototypes | Direct SQLite access | Schema compatibility is not required |

Start with the least coupled option that provides the required capability. Move to JSON or structured logging only when the simpler option is insufficient.

## Independent Commands

A command that does not need ctx data can remain completely independent. It does not need to use a ctx package, database, naming convention, or installation method.

This is the most stable option because the command has no ctx dependency.

## Run an Existing Command with `ctx x`

Use `ctx x` when an existing command only needs to run against the current target and be recorded in the workspace log.

```sh
ctx x nmap -sV '$IP'
```

`ctx x` currently:

- expands `$IP` and `${IP}` inside individual arguments to the primary target IP;
- passes stdin to the child process;
- streams stdout and stderr to the terminal;
- saves the command, expanded command, output, status, and exit code to the ctx log; and
- returns the child process exit code.

Quote `$IP` so the invoking shell does not expand it before ctx receives it.

### Arguments and target expansion

`ctx x` starts the named executable directly without invoking a shell. Every argument after `ctx x` is passed as a separate child-process argument. Shell operators such as pipes, redirects, and variable expansion are not interpreted unless the caller explicitly runs a shell such as `ctx x sh -c '...'`.

Every occurrence of `$IP` and `${IP}` inside an argument is replaced with the primary target IP. Expansion is performed only by ctx and only on these two placeholders. A primary target is required when either placeholder is present; commands without a placeholder can run without a target.

### Input, output, and logs

The child receives the caller's stdin. Its stdout and stderr are streamed to the corresponding ctx streams while also being captured in the command log. ctx creates a `running` log before starting the child and finishes it after the child exits.

The log stores the original command, expanded command, stdout, stderr, start and end times, status, and exit code. Arguments and output may therefore contain secrets; do not use `ctx x` for a command whose recorded values must remain secret.

If the log cannot be started, the child is not executed. If final log persistence fails, ctx reports failure even if the child already completed.

### Exit and interruption behavior

On normal completion, `ctx x` returns the child's exit code. Exit code `0` is logged as `success`; other ordinary exit codes are logged as `failed`.

If the executable cannot be started, ctx returns `127`, writes the error to stderr, and stores a failed log with the same diagnostic. If the child is terminated by a signal, ctx returns `128 + signal` (for example, `143` for SIGTERM) and stores the log as `interrupted` without an exit code.

Use another integration level when the command needs a selected service, credentials, or a way to save parsed findings.

## Read Investigation Data

Use a scalar command when only one value is required.

```sh
ctx prompt --field target-ip
ctx prompt --field workspace-path
```

Use JSON when reading multiple values, lists, nullable values, or data whose human-readable table may change.

```sh
ctx prompt --format json --format-version 1.0
ctx service ls --format json --format-version 1.0
ctx credential ls ssh --format json --format-version 1.0
```

Check available JSON outputs and versions before depending on them:

```sh
ctx formats --format json --format-version 1.0
```

Treat each key in `data.formats` as a supported integration capability. The currently defined capability keys are:

| Capability key | Operation |
|---|---|
| `formats` | Discover supported capability keys and format versions |
| `prompt` | Read the current workspace and target context |
| `credential` | List stored credentials |
| `service` | List stored services |
| `log` | Start and finish structured command logs |

Before using a required capability, verify that its key exists and that its version array contains the exact format version the command will request. A missing key, a missing version, a nonzero exit code, or `success: false` means the required capability is unavailable. Do not infer capability support from the ctx package version.

The existing `formats` output is the capability-discovery interface. A separate feature endpoint is intentionally not provided.

Do not parse human-readable tables with `grep`, `awk`, column positions, or regular expressions. Their wording, spacing, columns, ordering, and decoration are not an integration contract.

Credential JSON includes plaintext passwords. Do not expose them in logs, command arguments, diagnostics, process listings, or persistent temporary files.

See [ctx JSON API](api.md) for the response envelope, fields, errors, exit codes, and format version rules.

## Language-Independent Integration Procedure

The integration contract is the ctx process interface, not a library or programming language. Apply the following sequence in any language that can start a process, pass separate arguments, write stdin, and parse JSON.

1. Select a trusted absolute ctx executable path. The current APT installation uses `/usr/local/bin/ctx`. Start it directly with an argument array; do not construct a shell command string.

2. Choose the exact API format version required by the custom command. Do not derive it from the ctx package version.

3. Call `formats` and verify that every required capability contains that exact version in `data.formats`. Treat a missing capability or version as unsupported.

4. Call the required ctx operation with `--format json --format-version <version>`. Capture stdout, stderr, and the process exit status separately. For an operation with a JSON request body, serialize one JSON value to stdin rather than placing it in argv.

5. Parse stdout as one JSON response envelope. Do not parse stderr or a human-readable table as data. Stderr is diagnostic context and may be empty.

6. Verify that `format_version` is the exact version requested. For `success: true`, validate the documented `data` shape before using fields. Preserve the distinction between null, an empty array, and an absent field.

7. For `success: false`, branch on `error.code`, using its parent prefix only when an unknown child code is returned. Keep `error.message` and `error.details` for diagnostics, not control flow. Invalid requests normally exit with `2`; missing resources and execution failures normally exit with `1`.

8. Treat malformed stdout, a success envelope with a nonzero exit status, or an envelope whose version does not match as a protocol failure. A nonzero exit with a valid failure envelope still provides the authoritative `error.code`.

9. Save findings through a documented registration command when one exists. Depend on its syntax and exit status, not its human-readable message. Keep unsupported result types in the custom command rather than writing SQLite directly.

10. If the command owns a structured log lifecycle, start the log before the external action, retain the returned log ID, and finish that same ID with the final status. Do not start the external action when log creation fails.

Language-neutral pseudocode for a JSON read:

```text
ctx_path = trusted_absolute_path()
required_version = "1.0"

capabilities = run_process(
  executable = ctx_path,
  arguments = ["formats", "--format", "json", "--format-version", required_version],
  stdin = empty
)
formats_envelope = parse_and_validate_envelope(capabilities)
require formats_envelope.data.formats["prompt"] contains required_version

response = run_process(
  executable = ctx_path,
  arguments = ["prompt", "--format", "json", "--format-version", required_version],
  stdin = empty
)
prompt_envelope = parse_and_validate_envelope(response)
require prompt_envelope.success
use prompt_envelope.data
```

The same sequence applies when JSON stdin is required; only `stdin` changes from empty to serialized JSON bytes. Never include credential JSON, passwords, tokens, or Cookie values in diagnostic logs.

## Save Investigation Data

Use an existing ctx command when it already represents the finding being saved.

```sh
ctx host add admin.example.thm
ctx credential add ssh root '<password>'
ctx note 'admin.example.thm redirects to the management portal'
```

For machine integration, depend on the documented command syntax and exit code. Do not parse the human-readable success or error text.

There is no separate generic result-storage API. There is also no generic public command for writing every internal record type. If ctx cannot represent a concrete finding, keep it in the custom tool until a real integration use case justifies a new ctx command or machine-readable input.

Passing a password as an argument may expose it through shell history or the process list. Existing command behavior is shown here, not declared safe for unattended secret transfer. A safer machine input should be added to the specific command when a concrete integration requires it.

The stable command forms, duplicate behavior, exit codes, and current limitations are defined in [ctx Registration Commands](registration.md).

## Control Structured Logs

Use `ctx log start/finish` when `ctx x` does not provide enough control. For example, an interactive add-on may need to sanitize its expanded command or collect output itself.

Start the log by sending JSON through standard input:

```sh
printf '%s\n' '{"command":"custom-scan","expanded_command":"scanner 10.10.10.10","started_at":"2026-07-19T00:00:00Z"}' | \
  ctx log start --format json --format-version 1.0
```

After reading the returned log ID, finish it with the result:

```sh
printf '%s\n' '{"status":"success","exit_code":0,"stdout":"","stderr":"","ended_at":"2026-07-19T00:01:00Z"}' | \
  ctx log finish 1 --format json --format-version 1.0
```

Do not include passwords, authentication command arguments, tokens, or cookies in log fields. See [ctx JSON API](api.md#log) for the lifecycle format.

## Direct SQLite Access

Direct SQLite access is available for prototypes, unsupported queries, and advanced local analysis. It is not the preferred compatibility boundary for a maintained custom command.

The database schema is an implementation detail and may change in a ctx update. Direct writes can bypass validation, related updates, and transactions, and may corrupt the workspace. Back up the database before experimenting and prefer read-only access.

See [Database Design](database.md) for the current schema and migration policy.

## Data Ownership

Ownership identifies the component responsible for a data format, validation, migration, and lifecycle. It is not determined only by where a file is stored. A tool-specific cache may live under a ctx workspace while still belonging to that tool.

### Data Owned by ctx

ctx owns durable data that represents shared investigation context or findings.

| Data | ctx responsibility | Examples of producers and consumers |
|---|---|---|
| Workspace identity and metadata | Resolve the active workspace, maintain its identity, and migrate its database | All integrated commands |
| Project roots and project-to-workspace relationship | Store named roots, select the active root, validate project names, and associate project directories with workspaces | `ctx project`, shell integration |
| Targets | Maintain target identity, primary selection, IP changes, and related records | `ctx target`, scanners, connection tools |
| Hosts | Validate and associate hostnames with targets | Manual registration, DNS and vhost discovery tools |
| Services and scan history | Store normalized ports, protocols, products, versions, and scan provenance | `ctx scan`, service-aware add-ons |
| Credentials | Associate scoped usernames and passwords with targets and preserve verification evidence | Manual registration, authentication tools, connection tools |
| Notes | Store workspace-level human findings in the timeline | Users and note-producing tools |
| Command logs | Store command lifecycle, sanitized command text, output, status, and exit code | `ctx x`, add-ons using `ctx log` |
| Shared web discoveries | Store target paths, response metadata, source tool, wordlist, and log provenance | Web discovery and site-map features |
| Shared web wordlist run history | Coordinate completed and interrupted searches across compatible discovery commands | `xgobuster`, `xffuf` |
| Configuration storage | Store configured values and provide validation and defaults | ctx and tools that consume a documented key |

A producer does not become the owner of shared data by creating it. For example, a vhost scanner may register a host, but ctx remains responsible for hostname validation, target association, persistence, and migration.

ctx owns the named project root settings, active-root selection, workspace marker, and project-to-workspace relationship. It does not own the user's project files or arbitrary files created inside the project directory.

Some ctx-owned data does not yet have a public integration command or JSON endpoint. In particular, internal support for web discoveries or wordlist run history is not itself a public API. Custom commands must not infer a stable interface from an exported Go function or database table. A public operation should be added only when a concrete integration needs it.

### Data Owned by Each Tool

Each external or custom command owns implementation data that is meaningful only to that tool.

| Data | Tool responsibility | Examples |
|---|---|---|
| External command construction | Select the executable, flags, arguments, and environment | Gobuster, Hydra, ffuf, SSH invocation |
| Output parsing and classification | Interpret tool-specific output and decide what is a finding | HTTP response filtering, Hydra success parsing |
| Search strategy | Choose wordlists, profiles, request limits, escalation, and retry behavior | Directory, vhost, password, and username searches |
| Rebuildable progress cache | Define, update, clear, and migrate or discard private cache formats | searched-word files and per-tool completion state |
| Temporary files | Create securely and remove after use | Filtered wordlists and machine-readable result files |
| Interactive behavior | Prompts, selection order, defaults, terminal handling, and completion | Service, credential, and share selection |
| Tool-specific configuration semantics | Interpret a setting and apply it to external-tool behavior | TLS, request limits, calibration thresholds |
| Unregistered raw artifacts | Manage output files or evidence until deliberately promoted into ctx | Raw scanner output and downloaded files |

Tool-owned caches are disposable optimization data, not investigation findings. Removing a cache may cause work to be repeated, but it must not remove hosts, services, credentials, notes, logs, or other promoted findings.

### Shared Storage Does Not Mean a Shared Format

ctx may provide a workspace data directory so tool state follows the workspace. That location does not make every file inside it part of the public ctx schema.

The following rules apply:

- ctx-owned records are read or written through a documented ctx interface when one exists;
- tool-owned files are accessed only by the tool that defines their format;
- one tool must not parse another tool's private cache;
- a reusable result is promoted through a ctx registration interface instead of being shared by copying cache files; and
- deleting a workspace may remove both ctx-owned records and tool-owned state stored under that workspace, so tools must not treat the workspace data directory as independent storage.

### Configuration Has Split Responsibility

ctx owns configuration storage, key validation, defaults exposed by ctx, and persistence. The tool using a key owns the operational meaning of that setting and must handle it consistently.

Adding a private file under a tool directory is appropriate for data that no other command needs. Adding a ctx configuration key is appropriate only when the value is part of the shared user-facing ctx workflow.

### Promotion of Findings

External-tool output remains tool-owned until it has been validated and deliberately registered. Promotion follows this sequence:

1. the tool executes and parses its own output;
2. the tool rejects noise, incomplete output, and trial results;
3. the tool converts a confirmed result into a ctx concept such as a host or credential; and
4. ctx validates and stores the shared record with available provenance.

Trial runs and calibration must not register findings automatically. A failed or interrupted command may still create a command log, but partial output is not automatically a confirmed shared finding.

## Compatibility Rules

For integrations intended to survive ctx updates:

- use documented command syntax and exit codes;
- use JSON rather than human-readable tables;
- request an explicit format version;
- inspect `success` and `error.code` instead of matching messages;
- tolerate compatible fields added to JSON objects;
- check supported formats when a required endpoint or version may be unavailable; and
- avoid depending on the SQLite schema.

Published interfaces are retained as long as practical. If retaining an old format becomes excessively burdensome, ctx may remove it by changing the format version and documenting the breaking change in the release notes.

## Official and Custom Commands

Official `x*` commands may implement tool-specific behavior directly. At the boundary where a separate binary accesses data owned by ctx, the public integration contract is preferred when it can represent the operation without unreasonable complexity.

Custom commands receive the same contract. They do not need to use Go, copy an official command, or become part of this repository.

### Starting ctx safely

Bundled commands that start ctx as a separate process use the fixed absolute path `/usr/local/bin/ctx` in the current APT packages. They do not resolve `ctx` through `PATH`, and they pass arguments directly without invoking a shell. This prevents another executable named `ctx` earlier in `PATH` from receiving investigation data or credentials.

The shared internal launcher rejects relative executable paths. Tests can replace the path and runner independently. A distribution that installs ctx elsewhere must embed its trusted absolute path when building the dependent commands:

```sh
go build -ldflags '-X req/internal/ctxexec.ExecutablePath=/opt/ctx/bin/ctx' ./cmd/xssh
```

`internal/ctxexec` is an implementation helper for this repository, not a public Go SDK. Custom commands should likewise execute a trusted absolute ctx path selected by their installer or build, keep arguments separate, and avoid a shell unless shell behavior is explicitly required.

## Current Boundaries

The integration surface is intentionally limited to capabilities that already have concrete users. The following are not promised by this guide:

- JSON support for every ctx command;
- a generic CRUD or result-storage API;
- stable human-readable table output;
- a stable SQLite schema;
- an HTTP API; or
- a public language-specific SDK.

New integration capabilities should be added only after a real command demonstrates the missing operation.
