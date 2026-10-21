---
title: nem deactivate
weight: 29
---

Deactivate nem for the current shell.

Remove the hook block that nem activate installed from the shell's startup file. The current shell keeps its environment until it restarts.

## Usage

```shell
nem deactivate [zsh|bash]
```

## Examples

```shell
nem deactivate               # the current $SHELL
nem deactivate bash          # a specific shell
```

## Global flags

`--color`, `--narration`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
- Guide: [Shell integration]({{< relref "/docs/using/shell-integration" >}})
