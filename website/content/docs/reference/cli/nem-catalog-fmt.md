---
title: nem catalog fmt
weight: 11
---

Rewrite package manifests to canonical form.

Rewrite package manifests in canonical form: the field order, quoting, and layout nem catalog bump itself writes. Only a local directory or a single pkg.yaml can be formatted.

## Usage

```shell
nem catalog fmt [catalog] [flags]
```

## Examples

```shell
nem catalog fmt                        # every manifest in the current directory
nem catalog fmt . --package kubectl    # one package
```

## Flags

| Flag | Description |
|------|-------------|
| `--package <strings>` | format this package (repeatable; default: every package) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Authoring workflow]({{< relref "/docs/writing-packages/workflow" >}})
