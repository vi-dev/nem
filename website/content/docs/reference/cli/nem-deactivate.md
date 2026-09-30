---
title: nem deactivate
weight: 27
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

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
