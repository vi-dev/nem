---
title: nem catalog outdated
weight: 15
---

Report packages whose upstream has a newer version.

Compare each package's newest manifest version with the newest version its upstream offers and list the packages that lag behind. It never writes; nem catalog bump adds the versions.

## Usage

```shell
nem catalog outdated [catalog] [flags]
```

Aliases: `old`

## Examples

```shell
nem catalog outdated                     # every package
nem catalog outdated . --output json     # for scripts
```

## Flags

| Flag | Description |
|------|-------------|
| `--output <string>` | output format: text or json (default `text`) |
| `--package <strings>` | check this package (repeatable; default: every package) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Version discovery]({{< relref "/docs/writing-packages/version-discovery" >}})
