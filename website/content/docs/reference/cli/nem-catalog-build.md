---
title: nem catalog build
weight: 5
---

Build a catalog's build-from-source packages on the host platform.

Build a catalog's build-from-source packages on this machine's platform and stage the resulting archives; with --push, publish them into the catalog. Without --package, every buildable package is built at its latest version. --missing narrows the selection to versions and platforms whose archive is absent, which keeps re-runs cheap.

## Usage

```shell
nem catalog build [catalog] [flags]
```

## Examples

```shell
nem catalog build . --package openssl@3.6.1    # one version
nem catalog build . --missing --push            # fill gaps in the published archives
nem catalog build . --dry-run                   # show the plan and its waves
```

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | report the plan without building |
| `--force` | with --push, overwrite existing archives |
| `--missing` | select every version and platform of the selected packages whose archive is missing |
| `--package <strings>` | build name[@version] (repeatable; omitted version means latest; default: every buildable package) |
| `--push` | publish built archives into the catalog |
| `--with-deps` | also build every package's dependency whose archive is missing |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem catalog](../nem-catalog/)
