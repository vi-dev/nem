---
title: nem catalog remove
weight: 17
---

Remove a catalog.

Delete a catalog from config.yaml. Packages already installed from it stay under NEM_HOME, but projects whose lockfile pins packages from it cannot sync them until the catalog is added back.

## Usage

```shell
nem catalog remove <name>
```

Aliases: `rm`

## Examples

```shell
nem catalog remove corp      # forget it
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Catalogs]({{< relref "/docs/using/catalogs" >}})
