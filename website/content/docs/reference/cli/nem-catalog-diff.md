---
title: nem catalog diff
weight: 7
---

Compare a base catalog's package manifests against a target catalog.

Lint &lt;base&gt;, then compare its package manifests against &lt;target&gt; and print one row per package with its status relative to target: new, updated, or removed, plus a count of unchanged packages. Base is the catalog being worked on and target the reference, typically the published one. --output json emits only the changed rows together with the versions base adds.

## Usage

```shell
nem catalog diff <base> <target> [flags]
```

## Examples

```shell
nem catalog diff . ghcr.io/vi-dev/nem-catalog:v2                 # a checkout against the published catalog
nem catalog diff . ghcr.io/vi-dev/nem-catalog:v2 --output json   # for CI
nem catalog diff . ghcr.io/vi-dev/nem-catalog:v2 --package jq    # one package
```

## Flags

| Flag | Description |
|------|-------------|
| `--output <string>` | output format: text or json (default `text`) |
| `--package <strings>` | compare this package (repeatable; default: every package) |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
