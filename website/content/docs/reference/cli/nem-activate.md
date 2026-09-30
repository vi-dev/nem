---
title: nem activate
weight: 2
---

Activate nem for the current shell.

Install nem's hook block into the shell's startup file, .zshrc or .bashrc, between # &gt;&gt;&gt; nem &gt;&gt;&gt; and # &lt;&lt;&lt; nem &lt;&lt;&lt; markers. The hook re-applies the composed environment on every directory change and right after nem use, unuse, lock, and sync, and sources nem's completions. Without a shell name, $SHELL decides. Restart the shell afterwards.

## Usage

```shell
nem activate [zsh|bash] [flags]
```

## Examples

```shell
nem activate                 # the current $SHELL
nem activate zsh             # a specific shell
nem activate --print         # print the block instead of installing it
```

## Flags

| Flag | Description |
|------|-------------|
| `--print` | print the hook block to stdout instead of installing it into the rc file |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
