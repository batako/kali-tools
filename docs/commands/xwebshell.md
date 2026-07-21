# xwebshell Online Help

[日本語](./xwebshell.ja.md)

`xwebshell` catalogs web-shell templates installed below Kali's `/usr/share/webshells`, shows their contents, and exports configured copies. Use these templates only in explicitly authorized lab environments.

## Synopsis

```text
xwebshell [ls]
xwebshell show <ID>
xwebshell export <ID>
```

No arguments is equivalent to `ls`.

## Catalog and Status

`ls` compares the built-in catalog with the live filesystem and prints ID, status, category, and name:

```text
[+] known and available
[!] known but missing from this environment
[?] discovered on disk but not registered in the catalog
```

Available/New/Missing totals follow the table. IDs describe the current inventory and may change when packages change. The scan is recursive; cataloged directory groups cover their child files, and available SecLists-derived web shells are included.

## Show and Export

`show <ID>` prints metadata, absolute paths, and file contents. A group entry lists its included files. It never modifies the package source.

`export <ID>` accepts only available `[+]` entries, prompts for callback host/port or other template fields when defined, and copies the configured result into the current directory. Groups preserve their directory structure. `/usr/share/webshells` is never modified, and an existing destination is never overwritten.

Missing `[!]` entries cannot be exported. Unknown `[?]` files lack catalog metadata/configuration rules and should be inspected directly at their discovered path.

## Packages, Completion, and Safety

The Kali `webshells` package provides the primary root. Its absence is an error. Optional providers such as SecLists are inventoried only when present. Shell completion installed by `ctx init-shell` offers IDs with category/name/description. `__complete` is internal and should not be called manually.

Web shells provide remote command execution. Never place exported files in a public server, Git repository, or artifact unintentionally. Verify callback settings and authentication, and upload/execute only on an authorized target.

## Troubleshooting

- Package not found: install Kali's `webshells` package in the Kali service.
- Many `[!]` entries: optional packages may be absent or Kali's package layout may have changed.
- A `[?]` entry appears: it is a newly discovered, uncataloged package file.
- Export destination exists: xwebshell protects it; choose another directory or resolve it manually.
