---
title: nem exec
weight: 29
---

Run a command in the composed environment.

Run one command as a child process with the composed PATH and environment variables, and exit with its exact status. Nothing is installed; run nem sync first. Put -- before the command so its own flags are not parsed by nem.

## Usage

```shell
nem exec [-- <cmd> [args...]]
```

Aliases: `x`

## Examples

```shell
nem exec -- kubectl version --client   # one command
nem exec -- go test ./...              # a CI step
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
