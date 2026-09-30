---
title: nem status
weight: 35
---

Show declared packages and composed environment variables.

Print the packages the current scope declares with their declared version, catalog, and whether each is locked and installed, then the environment variables nem composes for it with the package or manifest each one comes from. Inside a project the result is the project layered over the global scope.

## Usage

```shell
nem status [flags]
```

Aliases: `st`

## Examples

```shell
nem status                   # this project, layered over the global scope
nem status -g                # the global scope alone
```

## Flags

| Flag | Description |
|------|-------------|
| `-g, --global` | show the global scope |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
