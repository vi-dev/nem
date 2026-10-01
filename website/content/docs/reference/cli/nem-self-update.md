---
title: nem self update
weight: 34
---

Update nem to the latest build on its channel.

Download a nem release build, verify its checksum, and replace the running binary with it. Without --version, updates within the current channel: stable builds to the latest release, unstable builds to the current rolling build from main.

## Usage

```shell
nem self update [flags]
```

Aliases: `up`

## Examples

```shell
nem self update                     # latest build on the current channel
nem self update --check             # report only
nem self update --version v0.3.0    # a specific release
```

## Flags

| Flag | Description |
|------|-------------|
| `--check` | only report whether an update is available |
| `--version <string>` | release tag to install, like v1.2.3, "stable" for the latest release, or "unstable" |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem self](../nem-self/)
- Guide: [Maintenance]({{< relref "/docs/using/maintenance" >}})
