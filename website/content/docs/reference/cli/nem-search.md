---
title: nem search
weight: 32
---

Search catalogs for packages.

Search catalogs for packages by name or description.
Without a query, list every available package.

## Usage

```shell
nem search [query]
```

Aliases: `find`

## Examples

```shell
nem search                   # every package in every enabled catalog
nem search kube              # names or descriptions containing "kube"
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
- Guide: [Catalogs]({{< relref "/docs/using/catalogs" >}})
