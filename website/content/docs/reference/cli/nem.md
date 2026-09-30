---
title: nem
weight: 1
---

Reproducible dev environments. For you, your teams, and your agents.

nem gives each project its own packages and environment variables, declared in nem.toml and pinned with digests in nem.lock. Packages install under NEM_HOME and join PATH only while the shell is inside the project, so teammates, CI, and agents reproduce the same environment from the two files.

## Usage

```shell
nem [flags]
```

## Examples

```shell
nem activate                 # hook nem into the current shell
nem use kubectl go@1.27.0    # declare packages in this project
nem sync                     # install what nem.lock pins
```

## Flags

| Flag | Description |
|------|-------------|
| `--color <string>` | colorize output: auto, always, or never (default `auto`) |
| `-q, --quiet` | suppress narration output |
| `--verbose` | show debug output |
| `-v, --version` | version for nem |

## Commands

### Environment

| Command | Description |
|---------|-------------|
| [exec](../nem-exec/) | Run a command in the composed environment |
| [lock](../nem-lock/) | Regenerate the lockfile from nem.toml and install |
| [status](../nem-status/) | Show declared packages and composed environment variables |
| [sync](../nem-sync/) | Install locked packages missing on this machine |
| [unuse](../nem-unuse/) | Remove declared packages |
| [update](../nem-update/) | Update declared packages to their latest versions |
| [use](../nem-use/) | Declare and install packages |
| [which](../nem-which/) | Show where a command resolves in the composed environment |

### Discovery

| Command | Description |
|---------|-------------|
| [info](../nem-info/) | Show a package's details and available versions |
| [search](../nem-search/) | Search catalogs for packages |

### Catalogs

| Command | Description |
|---------|-------------|
| [catalog](../nem-catalog/) | Manage catalogs |

### Shell integration

| Command | Description |
|---------|-------------|
| [activate](../nem-activate/) | Activate nem for the current shell |
| [deactivate](../nem-deactivate/) | Deactivate nem for the current shell |
| [env](../nem-env/) | Print the shell script that applies the composed environment |

### Maintenance

| Command | Description |
|---------|-------------|
| [clean](../nem-clean/) | Reclaim disk space in NEM_HOME |
| [self](../nem-self/) | Manage this nem installation |

### Other commands

| Command | Description |
|---------|-------------|
| [completion](../nem-completion/) | Generate the autocompletion script for the specified shell |
| [version](../nem-version/) | Print the version of nem |
