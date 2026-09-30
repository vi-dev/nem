---
title: nem which
weight: 41
---

Show where a command resolves in the composed environment.

Look each name up on the composed PATH and print the path it resolves to, one per line. A name that does not resolve is reported and the exit status is 1.

## Usage

```shell
nem which <command>...
```

## Examples

```shell
nem which kubectl            # the path nem's PATH picks
nem which go gofmt           # several at once
```

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
