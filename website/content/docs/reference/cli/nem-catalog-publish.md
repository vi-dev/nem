---
title: nem catalog publish
weight: 16
---

Publish a catalog to an OCI registry.

Lint the catalog, then push its package manifests to &lt;ref&gt; as an OCI index and move the given tags to it. Manifests whose content is unchanged are not pushed again unless --force. Archives built with nem catalog build --push and nem catalog fill live beside the index and are not touched.

## Usage

```shell
nem catalog publish <ref> [catalog] [flags]
```

Aliases: `pub`

## Examples

```shell
nem catalog publish registry.example/nem/catalog .                        # publish a checkout under tag v2
nem catalog publish registry.example/nem/catalog --tag v2 --tag 2026.09   # several tags
nem catalog publish registry.example/nem/catalog --dry-run                # the plan only
```

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | report the publish plan without writing anything |
| `--force` | push every package manifest even when unchanged |
| `--tag <strings>` | tag to move to the published index (repeatable; default v2) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Publishing your own catalog]({{< relref "/docs/managing-catalogs/publish-your-own" >}})
