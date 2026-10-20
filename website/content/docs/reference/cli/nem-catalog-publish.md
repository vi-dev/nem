---
title: nem catalog publish
weight: 16
---

Publish a catalog to an OCI registry.

Lint the catalog, then push its package manifests to &lt;ref&gt; as an OCI index and move the given tags to it. Packages are compared with the index the tags point at: unchanged manifests are not pushed again unless --force, and --dry-run lists what would change. --output json prints the ref, tags, index digest and changed packages on stdout. Archives built with nem catalog build --push and nem catalog fill live beside the index and are not touched.

## Usage

```shell
nem catalog publish <ref> [catalog] [flags]
```

Aliases: `pub`

## Examples

```shell
nem catalog publish registry.example/nem/catalog .                        # publish a checkout under tag v3
nem catalog publish registry.example/nem/catalog --tag v3 --tag 2026.09   # several tags
nem catalog publish registry.example/nem/catalog --dry-run                # what would change
```

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | report what would change without writing anything |
| `--force` | push every package manifest even when unchanged |
| `--output <string>` | output format: text or json (default `text`) |
| `--tag <strings>` | tag to move to the published index (repeatable; default v3) |

## Global flags

`--color`, `--narration`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Publishing your own catalog]({{< relref "/docs/managing-catalogs/publish-your-own" >}})
