---
title: nem use
weight: 39
---

Declare and install packages.

Resolve each package against the configured catalogs, install it, and record the result: the version in nem.toml and the exact resolved closure, with digests and dependencies, in nem.lock. Without @&lt;version&gt;, nem takes the first entry of the package's versions list, which catalogs keep newest first, and steps further down the list only when another package requires an older or compat-constrained one. Without &lt;catalog&gt;:, catalogs are searched in configured order and the first match wins.

## Usage

```shell
nem use [<catalog>:]<pkg>[@<version>]... [flags]
```

## Examples

```shell
nem use kubectl              # newest version from the first catalog that has it
nem use go@1.27.0            # an exact version
nem use corp:terraform       # from one specific catalog
nem use -g jq                # in the global manifest instead of the project
```

## Flags

| Flag | Description |
|------|-------------|
| `-g, --global` | target the global manifest |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
- Guide: [Packages]({{< relref "/docs/using/packages" >}})
