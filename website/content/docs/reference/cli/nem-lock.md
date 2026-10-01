---
title: nem lock
weight: 31
---

Regenerate the lockfile from nem.toml and install.

Resolve every package nem.toml declares, rewrite nem.lock with the exact closure, and install what is missing. Run it after editing nem.toml by hand. Every declared version must exist in a catalog exactly as written; there are no ranges and no latest keyword.

## Usage

```shell
nem lock [flags]
```

## Examples

```shell
nem lock                     # after editing nem.toml
nem lock -g                  # the global manifest
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
