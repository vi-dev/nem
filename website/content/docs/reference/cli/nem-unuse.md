---
title: nem unuse
weight: 37
---

Remove declared packages.

Remove packages from nem.toml, re-resolve nem.lock, and install what the new resolution needs so the remaining packages link against the versions it pins. Installed files stay under NEM_HOME because other projects may use them; nem clean reclaims them.

## Usage

```shell
nem unuse <pkg>... [flags]
```

## Examples

```shell
nem unuse kubectl            # drop it from this project
nem unuse -g jq              # drop it from the global manifest
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
