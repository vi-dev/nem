---
title: nem info
weight: 30
---

Show a package's details and available versions.

Print a package's description, homepage, license, supported platforms, and executables, the catalog it comes from, and every version that catalog offers. A &lt;catalog&gt;: prefix restricts the lookup to one catalog; without it, the first catalog in configured order that has the package answers.

## Usage

```shell
nem info [<catalog>:]<pkg>
```

## Examples

```shell
nem info kubectl             # from the first catalog that has it
nem info corp:kubectl        # from one catalog
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
- Guide: [Catalogs]({{< relref "/docs/using/catalogs" >}})
