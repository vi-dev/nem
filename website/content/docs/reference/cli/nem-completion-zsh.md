---
title: nem completion zsh
weight: 26
---

Generate the autocompletion script for zsh.

```text
Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(nem completion zsh)

To load completions for every new session, execute once:

#### Linux:

	nem completion zsh > "${fpath[1]}/_nem"

#### macOS:

	nem completion zsh > $(brew --prefix)/share/zsh/site-functions/_nem

You will need to start a new shell for this setup to take effect.
```

## Usage

```shell
nem completion zsh [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--no-descriptions` | disable completion descriptions |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem completion](../nem-completion/)
