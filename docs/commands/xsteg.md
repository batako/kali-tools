# xsteg Online Help

[日本語](./xsteg.ja.md)

`xsteg` investigates local files with several steganography backends and extracts candidates under explicit resource limits. `scan` detects without extracting; `extract` acts only on a completed scan.

## Synopsis

```text
xsteg [ls [path]]
xsteg scan <path>
xsteg extract <path> [--auto | --manual] [-w <file>] [--no-crack]
xsteg show <ID> [path]
xsteg doctor
```

No arguments lists reports below the current directory.

## Workflow and Scan Identity

```text
xsteg scan file
xsteg show ID
xsteg extract file
```

Extraction requires a completed scan for the same source path and current SHA-256. Running extract first fails without creating an empty `.xsteg` directory. A modified source requires another scan.

## Scanning

Available backends are selected by file type:

- `file` identifies MIME/type.
- `exiftool` inspects metadata.
- `binwalk -B` identifies signatures at offsets.
- `strings` records printable text.
- `steghide info` checks StegHide metadata on supported files.
- `stegseek --seed` rapidly detects protected StegHide candidates.
- `zsteg -a` checks PNG/BMP LSB candidates.

Missing optional backends are skipped or recorded as warnings. Scan does not run password wordlists or extract payloads.

An offset-zero source signature is not an embedded payload. Normal structures such as a TIFF record inside JPEG EXIF are also ignored. `binwalk: embedded file signatures detected` means additional, non-excluded offset signatures were found; it remains a candidate, not proof. Inspect `binwalk.txt`.

## Extraction and Passphrases

Extract reuses scan findings: Binwalk carving, saved zsteg selectors, StegHide empty/manual/wordlist passphrases, and optional whitespace extraction for text.

If the scan has no protected candidate, xsteg does not ask how to analyze a password. When a protected StegHide candidate exists and no mode was supplied, it prompts:

```text
1) Auto   try ctx password wordlists
2) Manual enter a known passphrase
3) Skip   do not analyze the protected payload
```

- `--auto`: skip the prompt and use `-w` or ctx `password` recommendations in priority order.
- `--manual`: securely prompt for one passphrase. A wrong value is a failure and creates no extracted finding.
- `-w, --wordlist <file>`: explicit automatic-analysis list.
- `--no-crack`: do not run wordlists; this implies automatic noninteractive handling.

`--manual` cannot be combined with `--wordlist` or `--no-crack`.

## Output and Reports

Results live next to the source in `<source>.xsteg`:

```text
file.jpg.xsteg/
  report.json
  file.txt
  exiftool.txt
  binwalk.txt
  files/
    binwalk/
    zsteg/
    steghide/
```

The same source/hash report is reused. An empty orphan directory is reused; `.xsteg.2` is created only for a real conflict.

`report.json` records source, hash, MIME, mode, status, backend results, findings, warnings, and extracted paths. `complete` means processing completed—not that a secret or valid password was found. Confirm an `extracted` finding and path; no results are shown as `Findings: none`.

`xsteg ls [path]` assigns report IDs within that search root. `xsteg show <ID> [path]` displays report details.

## Limits, Logging, and Doctor

Captured backend output is limited to 2 MiB per stream, each extracted file to 100 MiB, and extracted files to 100. Treat all output as untrusted and inspect type/content before execution.

Inside a ctx workspace, xsteg is a parent xlog entry and each backend is a child step. Passphrase arguments are redacted. Automatically discovered passwords may still appear in `report.json`, so reports are sensitive.

`xsteg doctor` reports required/optional backend availability and ctx password-wordlist readiness.

## Troubleshooting

- A clean file scans slowly: proving absence often requires every backend to finish; a positive candidate may return sooner.
- Extract says no completed scan: run scan first and ensure the source hash has not changed.
- A known passphrase fails: inspect doctor output, format support, scan findings, and backend output.
- Binwalk alone reports a candidate: inspect its offset and distinguish ordinary container/metadata structures.
