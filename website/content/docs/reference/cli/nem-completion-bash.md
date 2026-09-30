---
title: nem completion bash
weight: 23
---

Generate the autocompletion script for bash.

```text
Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(nem completion bash)

To load completions for every new session, execute once:

#### Linux:

	nem completion bash > /etc/bash_completion.d/nem

#### macOS:

	nem completion bash > $(brew --prefix)/etc/bash_completion.d/nem

You will need to start a new shell for this setup to take effect.
```

## Usage

```shell
nem completion bash
```

## Flags

| Flag | Description |
|------|-------------|
| `--no-descriptions` | disable completion descriptions |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem completion](../nem-completion/)
