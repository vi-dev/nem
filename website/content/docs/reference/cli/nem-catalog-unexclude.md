---
title: nem catalog unexclude
weight: 21
---

Lift the exclusion of package versions.

Remove the exclusion from the versions named by --package in a local catalog. The package keeps schema 4; lower it by hand once no version is excluded.

## Usage

```shell
nem catalog unexclude [catalog] [flags]
```

## Examples

```shell
nem catalog unexclude . --package asciinema@3.2.0   # offer the version again
```

## Flags

| Flag | Description |
|------|-------------|
| `--package <strings>` | lift the exclusion of name@version (repeatable; required) |

## Global flags

`--color`, `--narration`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Authoring workflow]({{< relref "/docs/writing-packages/workflow" >}})
