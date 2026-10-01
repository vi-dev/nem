---
title: nem catalog reorder
weight: 18
---

Reorder catalog precedence.

Rewrite the precedence list. Name every configured catalog exactly once, first to last; a lookup without a &lt;catalog&gt;: prefix stops at the first catalog that has the package.

## Usage

```shell
nem catalog reorder <name>...
```

## Examples

```shell
nem catalog reorder corp official   # corp wins over official
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Catalogs]({{< relref "/docs/using/catalogs" >}})
