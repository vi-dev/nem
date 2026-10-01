---
title: nem catalog mirror
weight: 14
---

Replicate a catalog and its archives to another registry.

Copy a catalog's index and every archive it references from &lt;src&gt; to &lt;dst&gt;, byte for byte, so that &lt;dst&gt; can be consumed as a catalog on its own. Re-running skips what is already present. Archives the source never published are not created; nem catalog fill adds them.

## Usage

```shell
nem catalog mirror <src> <dst> [flags]
```

## Examples

```shell
nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2             # copy
nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2 --dry-run   # the plan only
```

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | report the mirror plan without writing anything |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Mirroring and filling]({{< relref "/docs/managing-catalogs/mirror-and-fill" >}})
