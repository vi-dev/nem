---
title: nem env
weight: 28
---

Print the shell script that applies the composed environment.

Print the shell script that applies the composed environment to the current shell, together with the saved originals that let the same script restore them later. The hook runs it on every directory change; eval it yourself when you need the environment without the hook.

## Usage

```shell
nem env [flags]
```

## Examples

```shell
eval "$(nem env)"            # apply to the current shell
nem env --shell zsh          # render for a specific shell
```

## Flags

| Flag | Description |
|------|-------------|
| `--shell <string>` | shell dialect to render for: bash, zsh, or fish (default: $SHELL) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
