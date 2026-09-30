---
title: nem catalog disable
weight: 8
---

Disable configured catalogs.

Skip the named catalogs during lookups while keeping their place in the precedence order. nem catalog enable reverses it.

## Usage

```shell
nem catalog disable <name>...
```

## Examples

```shell
nem catalog disable official        # stop resolving from it
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
