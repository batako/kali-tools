# ctx JSON API

`ctx` provides JSON output that allows add-ons and external tools to use stored data.

This is not an HTTP API. Use the JSON written to standard output by passing `--format json` to a `ctx` command.

## Use Cases

The JSON API is intended to let external tools reuse information stored in `ctx`, including:

- Retrieving stored target information
- Retrieving stored credentials
- Retrieving stored service information
- Retrieving the current workspace state
- Checking available JSON output versions

You may access the SQLite database directly, but the JSON API is recommended when you want to avoid depending on the database schema.

## Basic Usage

```bash
ctx <command> [arguments] --format json [--format-version <version>]
```

Examples:

```bash
ctx prompt --format json
ctx credential ls ssh --format json
ctx service ls --format json
ctx formats --format json
```

To pin a format version:

```bash
ctx prompt --format json --format-version 1.0
```

## Format Versions

Format versions use the `MAJOR.MINOR` form.

```text
1.0
1.1
2.0
```

Version selection:

```text
omitted  latest available version
1        latest 1.x version
1.1      exact 1.1 version
```

Examples:

```bash
ctx prompt --format json
ctx prompt --format json --format-version 1
ctx prompt --format json --format-version 1.1
```

The `format_version` field in the response contains the exact version that was actually used.

```json
{
  "format_version": "1.1"
}
```

## Common Response

All JSON outputs use the same response envelope.

### Success

```json
{
  "success": true,
  "format_version": "1.0",
  "data": {},
  "error": null
}
```

### Failure

```json
{
  "success": false,
  "format_version": "1.0",
  "data": null,
  "error": {
    "code": "NOT_FOUND.WORKSPACE",
    "message": "no active workspace",
    "details": {}
  }
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `success` | boolean | Whether the operation succeeded |
| `format_version` | string \| null | The format version actually used |
| `data` | object \| array \| null | Data returned on success |
| `error` | object \| null | Error information returned on failure |

## Errors

### Structure

```json
{
  "code": "NOT_FOUND.WORKSPACE",
  "message": "no active workspace",
  "details": {}
}
```

- `code`: Machine-readable code used by external tools for branching
- `message`: Human-readable English message
- `details`: Raw error information or supplemental details

External tools should depend on `code`, not on the contents of `message` or `details`.

### Parent Codes

The following parent codes are currently defined as part of the common specification:

```text
INVALID_REQUEST
NOT_FOUND
INTERNAL_ERROR
```

Child codes use the parent code as a prefix.

```text
INVALID_REQUEST.FORMAT_VERSION
NOT_FOUND.WORKSPACE
NOT_FOUND.TARGET
NOT_FOUND.LOG
```

Current error codes:

| Code | Exit code | Meaning |
|---|---:|---|
| `INVALID_REQUEST` | 2 | The arguments or JSON request body are invalid |
| `INVALID_REQUEST.FORMAT_VERSION` | 2 | The requested format version is invalid or unsupported |
| `NOT_FOUND.WORKSPACE` | 1 | No active workspace is available |
| `NOT_FOUND.TARGET` | 1 | The primary or named target does not exist |
| `NOT_FOUND.LOG` | 1 | The command log does not exist |
| `INTERNAL_ERROR` | 1 | An unexpected database, filesystem, or implementation failure occurred |

User-correctable input and missing-resource errors are not returned as `INTERNAL_ERROR`.

When an unknown child code is returned, the portion before the first `.` can be treated as the parent code.

```text
NOT_FOUND.WORKSPACE
→ NOT_FOUND
```

## Output Rules

When `--format json` is specified, standard output contains JSON only.

- JSON: standard output
- Warnings, diagnostics, and debug information: standard error

Whenever a JSON response can be generated, errors—including unexpected errors—are returned through the common response format with `success: false`.

After `--format json` has been specified, invalid command arguments also use the common response envelope. Usage information is not emitted outside the JSON response in this case.

## Missing Values

All fields defined by the selected format version are always returned, even when no value is available.

```text
missing scalar value    null
empty list              []
no supplemental details {}
```

Example:

```json
{
  "username": "admin",
  "password": null
}
```

External tools may assume that every field defined by the selected format version is always present.

## Exit Codes

```text
0  success
1  execution error
2  invalid arguments
```

Use `error.code`, not the exit code, for detailed error handling.

## Available JSON Outputs

Initial supported outputs:

```text
formats
prompt
credential
log
service
```

## `formats`

Returns the available JSON output names and the format versions supported by each output.

```bash
ctx formats --format json --format-version 1.0
```

Without `--format json`, `ctx formats` prints a table of the same information:

```text
OUTPUT       VERSIONS
credential   1.0
formats      1.0
log          1.0
prompt       1.0
service      1.0
```

Add-ons can use this output to verify that the required JSON outputs and versions are available.

The `data` object has the following structure:

```json
{
  "formats": {
    "credential": ["1.0"],
    "formats": ["1.0"],
    "log": ["1.0"],
    "prompt": ["1.0"],
    "service": ["1.0"]
  }
}
```

| Field | Type | Description |
|---|---|---|
| `formats` | object | Map from a JSON output name to its supported format versions |
| `formats.<name>` | array of strings | Supported versions in ascending version order |

The order of keys in the `formats` object is not significant. An unknown output name must not be assumed to exist.

## `prompt`

Returns the current execution context, including the workspace, primary target, local IP address, and related information.

```bash
ctx prompt --format json --format-version 1.0
```

Example:

```json
{
  "success": true,
  "format_version": "1.0",
  "data": {
    "active": true,
    "workspace_id": "fa874e0a-c4d5-41fa-b6ba-63687d58a737",
    "workspace_name": "aaa",
    "workspace_path": "/workspace/cases/aaa",
    "local_ip": "172.18.0.2",
    "local_interface": "eth0",
    "target_name": "default",
    "target_ip": "1.2.3.4"
  },
  "error": null
}
```

The `data` object has the following fields:

| Field | Type | Description |
|---|---|---|
| `active` | boolean | Whether the current directory belongs to an active workspace |
| `workspace_id` | string \| null | Workspace identifier |
| `workspace_name` | string \| null | Workspace name |
| `workspace_path` | string \| null | Absolute workspace path |
| `local_ip` | string \| null | Local callback IP selected by ctx |
| `local_interface` | string \| null | Interface associated with `local_ip` |
| `target_name` | string \| null | Primary target name |
| `target_ip` | string \| null | Primary target IP address |

When `active` is `false`, all other fields are `null`. Fields whose values are not available in an active workspace are also `null`.

## `credential`

Returns stored credentials.

```bash
ctx credential ls --format json --format-version 1.0
ctx credential ls ssh --format json --format-version 1.0
```

When a `scope` is specified, only credentials matching that scope are returned.

Example `data` object:

```json
{
  "credentials": [
    {"id": 1, "scope": "ssh", "username": "root", "password": "toor"},
    {"id": 2, "scope": "ssh", "username": "testuser", "password": null}
  ]
}
```

| Field | Type | Description |
|---|---|---|
| `credentials` | array of objects | Credentials stored in the current workspace; `[]` when none match |
| `credentials[].id` | integer | Credential record ID |
| `credentials[].scope` | string | Credential scope |
| `credentials[].username` | string | Username |
| `credentials[].password` | string \| null | Password, or `null` when no password is stored |

Ordering:

```text
scope ASC, username ASC, id ASC
```

Passwords are returned in plaintext. External tools must avoid exposing retrieved passwords through logs, standard error, temporary files, or process listings.

## `log`

Add-ons can create and finish command logs without accessing the ctx database directly. Requests are read as JSON from standard input and responses are written as JSON to standard output.

Both operations require an active workspace. Request fields not listed below are not part of the versioned contract.

### Lifecycle

Call `start` immediately before beginning the operation. ctx creates a log with status `running` and returns its ID. Keep that ID in memory and call `finish` once after the operation reaches one of these terminal states:

```text
running -> success
running -> failed
running -> interrupted
```

Use `success` only when the operation completed successfully, `failed` for an ordinary failure, and `interrupted` when execution was cancelled or terminated before completion. If a process starts but the add-on exits without calling `finish`, the log remains `running`; callers should finish it as `interrupted` on cancellation whenever possible.

Start a log:

```bash
printf '%s\n' '{"command":"xssh","expanded_command":"ssh -p 22 testuser@172.18.0.5","started_at":"2026-07-13T00:00:00Z"}' | \
  ctx log start --format json --format-version 1.0
```

The response contains the new log ID:

```json
{
  "success": true,
  "format_version": "1.0",
  "data": {"id": 1},
  "error": null
}
```

Start request fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `command` | string | yes | User-facing command name; must not be blank |
| `expanded_command` | string | no | Expanded command; defaults to `command` when omitted or blank |
| `started_at` | string | no | Start time; defaults to the current UTC time in RFC 3339 format when omitted or blank |

The successful `data.id` is an integer containing the new command log ID.

Caller-supplied time strings are stored unchanged. Use an RFC 3339 UTC timestamp so ctx and other tools can order and display the log consistently.

Finish a log by sending its result as JSON. Do not include passwords or `sshpass` arguments in the command or output fields.

```bash
printf '%s\n' '{"status":"success","exit_code":0,"stdout":"connected\n","stderr":"","ended_at":"2026-07-13T00:05:00Z"}' | \
  ctx log finish 1 --format json --format-version 1.0
```

Finish request fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `status` | string | yes | One of `success`, `failed`, or `interrupted` |
| `exit_code` | integer \| null | no | Process exit code; defaults to `0` when omitted or `null` |
| `stdout` | string | no | Captured standard output; defaults to an empty string |
| `stderr` | string | no | Captured standard error; defaults to an empty string |
| `ended_at` | string | no | End time; defaults to the current UTC time in RFC 3339 format when omitted or blank |

The `<id>` argument must be a positive integer identifying an existing command log. A successful finish returns the same shape as start: `data.id` contains that log ID.

### Secret handling

The command, expanded command, stdout, and stderr are persisted in the workspace database and can be displayed by `ctx log`. Remove passwords, tokens, cookies, authentication headers, and other secrets before sending the request. In particular, do not record password-bearing command arguments such as `sshpass` invocations.

## `service`

Returns stored service information.

```bash
ctx service ls --format json --format-version 1.0
ctx service ls --target web --format json --format-version 1.0
```

Without `--target`, services for the primary target are returned. With `--target <name>`, services for that named target are returned.

Example `data` object:

```json
{
  "services": [
    {
      "id": 1,
      "port": 22,
      "protocol": "tcp",
      "state": "open",
      "reason": null,
      "service_name": "ssh",
      "product": "OpenSSH",
      "version": null,
      "extrainfo": null,
      "tunnel": null,
      "cpe": null,
      "last_seen": "2026-07-13T00:00:00Z"
    }
  ]
}
```

| Field | Type | Description |
|---|---|---|
| `services` | array of objects | Services stored for the selected target; `[]` when none exist |
| `services[].id` | integer | Service record ID |
| `services[].port` | integer | Port number |
| `services[].protocol` | string | Transport protocol |
| `services[].state` | string \| null | Detected port state |
| `services[].reason` | string \| null | Detection reason |
| `services[].service_name` | string \| null | Detected service name |
| `services[].product` | string \| null | Detected product |
| `services[].version` | string \| null | Detected product version |
| `services[].extrainfo` | string \| null | Additional service information |
| `services[].tunnel` | string \| null | Tunnel type, such as `ssl` |
| `services[].cpe` | string \| null | Detected CPE value |
| `services[].last_seen` | string \| null | Time the service was last observed |

Ordering:

```text
protocol ASC, port ASC, id ASC
```

## Compatibility

Format versions manage JSON structure only.

### MAJOR

Incremented for breaking changes to the base structure.

Examples:

- Removing a base field
- Renaming a field
- Changing a type
- Changing meaning
- Changing nullability
- Changing nesting
- Changing between arrays and objects

### MINOR

Incremented for extensions that preserve the base structure of the same MAJOR version.

Optional fields introduced in an earlier MINOR version within the same MAJOR version may be removed or restructured in a later MINOR version.

### Same Version

The JSON structure remains fixed within the same `MAJOR.MINOR` version.

The following changes do not require a format version update:

- Fixing value retrieval bugs
- Changing internal implementation
- Correcting returned values
- Correcting ordering that violated the specification
- Fixing JSON escaping bugs

## Version Retention

Published format versions are retained by default.

However, backward compatibility may be dropped when maintaining compatibility becomes excessively burdensome. Permanent retention is not guaranteed.

When support is dropped, the affected JSON output and format version will be identified explicitly.
