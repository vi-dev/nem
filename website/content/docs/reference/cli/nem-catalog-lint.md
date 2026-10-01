---
title: nem catalog lint
weight: 12
---

Validate package manifests in a catalog.

Parse and validate every package manifest in the catalog and print one warning per finding; the exit status is 1 when there are findings. The catalog is the current directory by default. A directory, a single pkg.yaml, or an OCI reference all work.

## Usage

```shell
nem catalog lint [catalog] [flags]
```

## Examples

```shell
nem catalog lint                                 # the current directory
nem catalog lint . --package kubectl             # one package
nem catalog lint ghcr.io/vi-dev/nem-catalog:v2   # a published catalog
```

## Flags

| Flag | Description |
|------|-------------|
| `--package <strings>` | lint this package (repeatable; default: every package) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Authoring workflow]({{< relref "/docs/writing-packages/workflow" >}})
