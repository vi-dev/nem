---
title: nem catalog add
weight: 4
---

Add a catalog.

Append a catalog to config.yaml under &lt;name&gt;, after the ones already configured. &lt;ref&gt; is an OCI reference or a local directory; the type is detected from it unless --type says otherwise. nem use, lock, sync, and update pull a new oci catalog the first time they need it; nem catalog update pulls it right away.

## Usage

```shell
nem catalog add <name> <ref> [flags]
```

## Examples

```shell
nem catalog add corp registry.example/nem/catalog:v4             # an OCI catalog
nem catalog add corp registry.example/nem/catalog@sha256:0123...  # pinned to a digest
nem catalog add local ./my-catalog                                # a directory
```

## Flags

| Flag | Description |
|------|-------------|
| `--type <string>` | catalog type: oci or dir (default: auto-detect) |

## Global flags

`--color`, `--narration`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Catalogs]({{< relref "/docs/using/catalogs" >}})
