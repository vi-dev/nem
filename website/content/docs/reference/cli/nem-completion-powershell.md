---
title: nem completion powershell
weight: 25
---

Generate the autocompletion script for powershell.

```text
Generate the autocompletion script for powershell.

To load completions in your current shell session:

	nem completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.
```

## Usage

```shell
nem completion powershell [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--no-descriptions` | disable completion descriptions |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem completion](../nem-completion/)
