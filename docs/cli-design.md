# CLI Command Design Rules

These rules apply when adding or extending CLI commands in this repository.

## Terminology

- Root command: the top-level command corresponding to the executable name.
- Subcommand: a command directly below the root command or another subcommand.
- Current level: the command level selected by the command words already entered.
- Default subcommand: a subcommand explicitly defined as omittable.
- Short option: a short alias such as `-h` or `-V` corresponding to a long option.

## Command hierarchy

Commands should use subcommands by default.

```text
<command> <subcommand> [nested-subcommand] [arguments] [options]
```

Separate subcommands by unit of behavior. The root command and every subcommand define the subcommands, arguments, options, and help available at their own level.

## Subcommand omission and default behavior

Each command must explicitly define what happens when a subcommand is omitted.

- With no arguments or options, showing root help is the default behavior.
- A command may instead run a documented default view or default subcommand with no arguments.
- When input or operation options are present, the command may imply one clearly documented default subcommand.
- If no default behavior is defined, do not guess; show help or an error.
- Omitted and explicit subcommand forms must produce the same processing result for the same input.
- Do not make destructive or ambiguous operations implicit defaults.

Document the default behavior and omission conditions in root help and command documentation.

## Positional arguments accepting multiple types

When one positional argument accepts multiple input types, define both explicit type selection and automatic detection.

```text
command [--type-a VALUE | --type-b VALUE | VALUE]
```

- Provide an explicit flag for each input type.
- Treat flagged input as the explicitly selected type.
- Detect the type of an unflagged positional argument using rules defined by the command.
- Document detection precedence and inconclusive-input behavior in help and command documentation.
- Explicit selection and automatic detection must produce the same processing result for the same type and input.
- Even when types can collide, allow the intended type to be selected explicitly.
- As a rule, do not combine an explicit type flag with a positional argument representing the same input.
- A formal flag used only to disambiguate a type may be omitted from completion candidates.
- Formal flags omitted from completion must still be documented in help and command documentation.

Each command defines its actual flag names, input types, and detection rules.

## Help

The root command and every subcommand provide help for their own command level.

```sh
command help
command help subcommand
command --help
command subcommand --help
command subcommand nested-subcommand --help
```

- A command using subcommands must implement a root-level `help` subcommand.
- `command help` shows root help, while `command help subcommand [...]` shows help for the specified level.
- The root and every subcommand must accept both `-h` and `--help`.
- Help describes the current command level.
- If the current level has immediate subcommands, list every one with its description.
- Do not include argument or option details for lower-level subcommands; place them in that subcommand's help.
- Document every positional argument and option available at the current level.
- Do not omit formal flags merely because completion omits them.
- When a short option exists, document both its long and short forms.
- Omitting short options from completion does not permit omitting them from help or command documentation.
- Document subcommand omission behavior in the help for the level where omission is allowed.

Each command must state whether no arguments show help or execute a defined default behavior.

## Root-level common commands and options

A command using subcommands must implement root-level `help` and `version` subcommands as well as their flag forms.

```sh
command help [subcommand...]
command version
command -h
command --help
command --online-help
command -V
command --version
```

- List `help` and `version` in the subcommand section of root help.
- The `help` and `version` subcommands must return the same results as their flag forms.
- Generate the `--online-help` URL from the command name and `Version` through the shared `internal/onlinehelp` package.
- Accept both `-V` and `--version` for version output.
- Keep `version`, `-V`, `--version`, and `--online-help` root-only; do not accept them after an operation subcommand.
- A subcommand-specific equivalent requires a separately documented design.

## Completion

Completion candidates change according to the current command level.

- If the current level has subcommands, include its immediate subcommands.
- Include options available at the current level.
- Do not include options belonging to a lower-level subcommand until that subcommand is selected.
- Do not include default-subcommand operation options until the subcommand is selected.
- After selecting a subcommand, apply the same rules at the selected level.
- Treat `help` and `version` as the canonical completion forms; omit the corresponding `-h` / `--help` and `-V` / `--version` flags from completion.
- Do not include a short option when its long form exists. Manually entered short options must still be accepted.
- A formal flag used only to disambiguate a type may be omitted from completion candidates.
- At an option-value position, provide candidates appropriate to that value, such as files, IDs, or enumerated values.
- When displaying a completion list, show subcommands above options.
- Bash and Zsh must provide the same candidates and value completion at the same command level.

Example:

```text
command <TAB>
  subcommand-a  -- description
  subcommand-b  -- description
  help          -- show help
  version       -- show version
  --online-help  -- show the versioned online help URL

command subcommand-a <TAB>
  --operation-option  -- description
```

## Implementation and verification checklist

- Implement help for the root and every subcommand.
- Implement `help` and `version` subcommands and their corresponding flag forms.
- At each level, document every immediate subcommand, argument, and option.
- Document long and short forms in help and command documentation.
- Test subcommand omission and default behavior.
- Test that explicit input-type selection and automatic detection produce the same result.
- Test that `help` / `--help` and `version` / `--version` return the same results.
- Test that root-only version output and `--online-help` are rejected at operation-subcommand levels.
- Verify completion hierarchy, ordering, omission of duplicate help/version flags and short options, and value completion in Bash and Zsh.
- Do not define the same positional argument more than once in Bash or Zsh completion.
- Update both `docs/commands/<command>.md` and `.ja.md`.
- Update both English and Japanese release notes.
