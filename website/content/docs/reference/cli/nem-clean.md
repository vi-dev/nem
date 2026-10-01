---
title: nem clean
weight: 21
---

Reclaim disk space in NEM_HOME.

Remove leaked build staging, leaked downloads, partial installs, and leftover test installs. Bare nem clean touches only this provable garbage and never prompts, so it is safe to run unattended.

With --unused or --all, also remove installed package versions; nem sync restores a project's packages. --unused measures the last time nem itself used a version — nem env on a directory change, nem exec, an install, or nem catalog build — not the last time a shell actually used it, so a shell that has not changed directories in a while can still have a version on its PATH that --unused would evict.

## Usage

```shell
nem clean [flags]
```

## Examples

```shell
nem clean                    # provable garbage only, no prompt
nem clean --unused 30d       # also versions not resolved in 30 days
nem clean --all --dry-run    # show what removing every version would free
```

## Flags

| Flag | Description |
|------|-------------|
| `--all` | remove every installed package version |
| `--dry-run` | print the plan without deleting anything |
| `--grace <duration>` | leave recently-touched reclaimable paths alone (default `1h0m0s`) |
| `--unused <days\|hours>` | evict versions nem hasn't resolved in this long, like 30d or 12h |
| `-y, --yes` | skip the confirmation prompt |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
- Guide: [Maintenance]({{< relref "/docs/using/maintenance" >}})
