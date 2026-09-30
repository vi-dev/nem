---
title: nem catalog update
weight: 20
---

Sync oci catalogs from their remote.

Pull the current index of each oci catalog from its registry into NEM_HOME so that resolution sees new packages and versions. Without a name, every enabled oci catalog is synced. dir catalogs are read live and need no sync.

## Usage

```shell
nem catalog update [name]
```

Aliases: `up`

## Examples

```shell
nem catalog update           # every enabled oci catalog
nem catalog update official  # one catalog
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
