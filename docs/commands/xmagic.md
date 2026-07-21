# xmagic Online Help

[日本語](./xmagic.ja.md)

`xmagic` creates a copy whose leading magic number identifies it as another file type. It never modifies the source and chooses a nonconflicting output name automatically. Use it only for authorized file-validation testing.

## Synopsis

```text
xmagic [ls]
xmagic set <type> <file>
```

No arguments is equivalent to `xmagic ls`.

## Supported Types

| type | signature | alias |
| --- | --- | --- |
| `gif` | `GIF89a` | `gif89a` |
| `jpg` | `FF D8 FF` | `jpeg` |
| `png` | `89 50 4E 47 0D 0A 1A 0A` | none |
| `pdf` | `%PDF-` | none |
| `zip` | `50 4B 03 04` | none |

GIF87a is recognized as a source signature but cannot be selected as a target.

## Replace versus Prepend

If the source begins with a known signature, xmagic removes that signature's bytes and writes the target signature in their place. For example, PNG-to-JPG removes the eight-byte PNG signature and adds the three-byte JPEG signature.

If the source signature is unknown, no source bytes are removed and the target is prepended. Thus `xmagic set gif shell.php` creates `GIF89a` followed by the complete PHP source.

This does not convert the full file structure. MIME detectors and parsers may reject or classify the output differently. Setting a type already present at the beginning is an error.

## Output Naming and Publication

The target type is inserted between the original stem and extension:

```text
image.png -> image.jpg.png
shell.php -> shell.gif.php
```

Existing paths are never overwritten; `.2`, `.3`, and so on are added. Data is written to a temporary file and published as a new path only after completion. Permission bits are copied from the source.

On success xmagic prints the operation, absolute output path, detected content type, source/output SHA-256 values, and confirmation that the original is unchanged. Internal operation state includes paths, added/removed bytes, hashes, and time. The current release does not expose a restore command; retain the source as the authoritative original.

## Safety and Troubleshooting

Only regular files are accepted. A valid leading signature does not make the rest of the file structurally valid. Do not create or distribute malicious files; limit use to systems where upload-validation testing is authorized.

- Already has the target magic: no change is needed.
- An upload is still rejected: the application may validate extension, MIME, decoding, or re-encode images.
- Detection is unexpected: inspect the bytes and full parser output; magic is only one signal.
