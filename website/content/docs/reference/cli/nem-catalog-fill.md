---
title: nem catalog fill
weight: 10
---

Download a catalog's upstream artifacts and publish them as archives.

For each package version the catalog's manifests pin by checksum, download the upstream artifact, verify it, and publish it as an archive beside the index at &lt;ref&gt;, so that consumers no longer reach upstream. It needs push access to &lt;ref&gt;; run it against your own mirror. Re-running heals missing or stale archives and skips the rest.

## Usage

```shell
nem catalog fill <ref> [flags]
```

## Examples

```shell
nem catalog fill registry.corp.example/nem/catalog:v2                     # every package
nem catalog fill registry.corp.example/nem/catalog:v2 --package kubectl   # one package
nem catalog fill registry.corp.example/nem/catalog:v2 --dry-run           # the plan only
```

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | report the fill plan without downloading or publishing anything |
| `--package <strings>` | fill this package (repeatable; default: every package) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
