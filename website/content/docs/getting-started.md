---
title: Getting started
weight: 1
---

`nem` makes your development environment appear the moment you enter a project
directory. Declare the packages and environment variables a project needs in its
`nem.toml` file, and `nem` handles the rest.

{{< callout type="info" >}}
`nem` is in active development. Expect breaking changes and rough edges —
feedback and contributions are very welcome.
{{< /callout >}}

## Installation

Install `nem` using the provided installation script:

```shell
curl -fsSL https://raw.githubusercontent.com/vi-dev/nem/main/install.sh | bash
```

By default, `nem` is installed to `~/.local/bin`. Add this directory to your
`PATH` if it isn't already:

```shell
export PATH="$HOME/.local/bin:$PATH"
```

It's possible to customize the installation using environment variables:

| Variable          | Default        | Meaning                                       |
|-------------------|----------------|-----------------------------------------------|
| `NEM_VERSION`     | latest release | a release tag such as `v0.1.0`, or `unstable` |
| `NEM_INSTALL_DIR` | `~/.local/bin` | install destination                           |
| `GITHUB_TOKEN`    | unset          | optional; raises the GitHub API rate limit    |

These and every other variable nem reads are listed under
[Configuration variables](../reference/environment/).

Examples:

To install `unstable` version:

```shell
curl -fsSL https://raw.githubusercontent.com/vi-dev/nem/main/install.sh | NEM_VERSION=unstable bash
```

Or install a specific version:

```shell
curl -fsSL https://raw.githubusercontent.com/vi-dev/nem/main/install.sh | NEM_VERSION=v0.1.0 bash
```

Prebuilt binaries for Linux and macOS are also attached to every
[GitHub release](https://github.com/vi-dev/nem/releases).

### Update

`nem` is able to update itself:

```shell
nem self update                   # update to the latest stable release
nem self update --check           # check for updates without installing
nem self update --version v0.1.0  # update to a specific version
```

## Quickstart

Try `nem` in a project directory:

```shell
nem activate      # hook nem into your shell (zsh, bash)
exec $SHELL

cd ~/code/my-project
nem use kubectl   # records the resolved version in nem.toml and installs it
kubectl version --client
```

`kubectl` is now on your `PATH` whenever you are in this directory.

### Step by step

{{% steps %}}

### Hook nem into your shell

```shell
nem activate
exec $SHELL
```

`nem activate` installs a hook block into your shell's startup file (zsh and
bash are supported), and `exec $SHELL` restarts the shell so the hook takes
effect. The hook applies each project's environment automatically as you move
between directories. Use `nem activate --print` to inspect the block instead
of installing it. [Shell integration](../using/shell-integration/) explains
what the block does.

### Declare your first packages

```shell
nem use kubectl
nem use go@1.27.0
```

`nem` installs the latest version of `kubectl`, the requested version of `go`, and records
the details in two files in the project directory:

- `nem.toml` — the manifest: what the project wants, e.g. `kubectl = '1.36.3'`
- `nem.lock` — machine-written: the exact resolved packages with SHA-256 digests and other details.

Both of these files should be committed to version control.

{{< callout type="info" >}}
On first run, `nem` configures the official package catalog,
[`ghcr.io/vi-dev/nem-catalog`](https://github.com/vi-dev/nem-catalog).
Turn it off with `nem catalog disable official`, or add your own catalogs
with `nem catalog add`.
{{< /callout >}}

### Use the packages

```shell
go version
kubectl version --client
```

The packages are on your `PATH` only while you are in this directory (or a
subdirectory). Leave and it disappears; come back and it returns.

### Share the environment

As long as `nem.toml` and `nem.lock` are committed, teammates, CI pipelines, and agents 
can reproduce the same environment by running:

```shell
nem sync              # install what nem.lock pins
```

`nem sync` installs everything the lockfile pins that is missing on their
machine — same versions, same digests. A catalog store that was never
synced is synced first; a stale one is not, which is what `nem catalog
update` is for.

{{% /steps %}}

## Everyday usage

The commands you will use most. For every command, see the
[command reference](../reference/cli/).

| Command | Purpose |
|---------|---------|
| [`nem use`](../reference/cli/nem-use/) | Declare and install packages |
| [`nem sync`](../reference/cli/nem-sync/) | Install what `nem.lock` pins |
| [`nem status`](../reference/cli/nem-status/) | Show declared packages and composed environment variables |
| [`nem search`](../reference/cli/nem-search/) | Search catalogs for packages |
| [`nem which`](../reference/cli/nem-which/) | Show where a command resolves in the composed environment |
| [`nem exec`](../reference/cli/nem-exec/) | Run a command in the composed environment |

## Next steps

[How nem works](../using/how-it-works/) explains the model behind the
commands you just ran. From there, pick the section for what you do:

{{< cards >}}
  {{< card link="../using/" title="Using nem" subtitle="Packages, environment variables, the shell hook, CI, and coding agents." >}}
  {{< card link="../managing-catalogs/" title="Managing catalogs" subtitle="Mirror or publish a catalog in a registry you control." >}}
  {{< card link="../writing-packages/" title="Writing nem packages" subtitle="Write, build, and test packages for your own catalog." >}}
  {{< card link="../reference/" title="Reference" subtitle="Commands, file formats, and the on-disk layout." >}}
{{< /cards >}}
