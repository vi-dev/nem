---
title: nem catalog exclude
weight: 10
---

Exclude package versions, with a reason shown to users.

Mark the versions named by --package as excluded in a local catalog. The resolver never picks an excluded version, and asking for it exactly fails with the reason. --use names a replacement version for the error's hint and needs a single --package; an exclusion that suggested an excluded version stops suggesting it. Excluding an excluded version again replaces its reason and --use. The package is raised to schema 4: once the catalog is published, nem releases older than schema 4 refuse to update the whole catalog, not only this package.

## Usage

```shell
nem catalog exclude [catalog] [flags]
```

## Examples

```shell
nem catalog exclude . --package asciinema@3.2.0 --reason "CVE-2026-1234" --use 3.2.1   # suggest a replacement
nem catalog exclude . --package jq@1.7.0 --package jq@1.7.1 --reason "Broken build"     # several versions
```

## Flags

| Flag | Description |
|------|-------------|
| `--package <strings>` | exclude name@version (repeatable; required) |
| `--reason <string>` | one line shown to users who ask for the version (required) |
| `--use <string>` | a listed version to suggest instead |

## Global flags

`--color`, `--narration`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Authoring workflow]({{< relref "/docs/writing-packages/workflow" >}})
