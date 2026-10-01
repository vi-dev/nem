---
title: nem sync
weight: 36
---

Install locked packages missing on this machine.

Install exactly what nem.lock pins and nothing else, verifying every download against its digest. It never resolves versions or rewrites files, which makes it the command for teammates, CI, and agents. A nem.toml declaration the lockfile does not cover earns a warning to run nem lock.

## Usage

```shell
nem sync [flags]
```

## Examples

```shell
nem sync                     # in a checked-out project
nem sync -g                  # the global scope
```

## Flags

| Flag | Description |
|------|-------------|
| `-g, --global` | target the global scope |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
- Guide: [Packages]({{< relref "/docs/using/packages" >}})
