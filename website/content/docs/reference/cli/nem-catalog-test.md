---
title: nem catalog test
weight: 19
---

Install packages and run their declared test steps.

Install the selected packages from the catalog into NEM_HOME and run each one's declared test steps, continuing past failures and printing a summary at the end. A package without test steps is install-verified only. The exit status is 1 when any package fails.

## Usage

```shell
nem catalog test [catalog] [flags]
```

## Examples

```shell
nem catalog test                                # every package at its latest version
nem catalog test . --package jq@1.8.3           # one version
nem catalog test . --package jq --package yq    # several packages
```

## Flags

| Flag | Description |
|------|-------------|
| `--package <strings>` | test name@version package (repeatable; omitted version means latest; default: every package) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
- Guide: [Testing packages]({{< relref "/docs/writing-packages/testing" >}})
