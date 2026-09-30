---
title: nem update
weight: 38
---

Update declared packages to their latest versions.

Re-resolve declared packages to the newest version their catalog offers, as if nem use had been run again for each, then install and rewrite nem.toml and nem.lock. Without names, every declared package is updated. A pick below a declared version aborts the whole update, and a catalog that has not been synced earns a warning to run nem catalog update first.

## Usage

```shell
nem update [<pkg>...] [flags]
```

Aliases: `up`

## Examples

```shell
nem update                   # every declared package
nem update kubectl           # one package
nem update --dry-run         # show the plan without writing anything
```

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | report the update plan without writing anything |
| `-g, --global` | target the global manifest |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
