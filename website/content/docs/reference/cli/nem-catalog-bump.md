---
title: nem catalog bump
weight: 6
---

Add newer upstream versions to package manifests.

Discover newer upstream versions for the selected packages, download each new artifact to compute its checksums, and add the resulting entries to the package manifests. With --backfill &lt;n&gt;, the newest n discovered versions are given entries even when they are older than the current latest. --dry-run only discovers and reports.

## Usage

```shell
nem catalog bump [catalog] [flags]
```

## Examples

```shell
nem catalog bump                         # every package
nem catalog bump . --package kubectl     # one package
nem catalog bump . --backfill 3          # ensure the newest three versions exist
nem catalog bump . --dry-run             # report without writing
```

## Flags

| Flag | Description |
|------|-------------|
| `--backfill <int>` | also ensure the newest &lt;n&gt; discovered versions have entries |
| `--dry-run` | report the versions that would be added without downloading or writing anything |
| `--output <string>` | output format: text or json (default `text`) |
| `--package <strings>` | bump name[@version] (repeatable; omitted version means every newer version; default: every package) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Version discovery]({{< relref "/docs/writing-packages/version-discovery" >}})
