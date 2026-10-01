---
title: Configuration variables
weight: 7
---

Every environment variable nem reads, and every one it sets.

## Variables nem reads

| Variable | Default | Effect |
|---|---|---|
| `NEM_HOME` | `~/.nem` | Where everything nem installs and records lives, used as given: no `~` expansion, a relative value resolves against each command's working directory, and an empty value counts as unset. See [NEM_HOME](../nem-home/). |
| `NO_COLOR` | unset | Any non-empty value disables colour when `--color` is `auto`. |
| `GITHUB_TOKEN` | unset | Raises the GitHub API rate limit for `nem self update`. |
| `DOCKER_CONFIG` | `~/.docker` | Directory holding the `config.json` that registry credentials are read from. |
| `SHELL` | `bash` when unset | Picks the dialect for `nem activate`, `nem deactivate`, and `nem env` when no shell is named. |
| `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` | unset | Standard proxy settings, in upper or lower case with upper case winning, honoured for every download and registry request. `ALL_PROXY` is not read. |

Beyond these, nem is configured only through its files:
[config.yaml](../config-yaml/) for catalogs and hosts, and a project's
[nem.toml](../nem-toml/). `TMPDIR` is never read; all staging goes under
`$NEM_HOME/tmp`.

## Install script variables

The install script at
`https://raw.githubusercontent.com/vi-dev/nem/main/install.sh` reads:

| Variable | Default | Effect |
|---|---|---|
| `NEM_VERSION` | latest release | A release tag such as `v0.1.0`, or `unstable` for the rolling build from `main`. |
| `NEM_INSTALL_DIR` | `~/.local/bin` | Where the binary is placed. |
| `GITHUB_TOKEN` | unset | Raises the GitHub API rate limit while the script looks up releases. |

## Variables the shell hook sets

The block that `nem activate` installs, and the script `nem env` prints, keep
their bookkeeping in your shell's environment:

| Variable | Set by | Meaning |
|---|---|---|
| `PATH` | `nem env` | Package directories ahead of the original `PATH`. |
| `NEM_ORIGINAL_PATH` | the hook block and `nem env`, once | `PATH` before nem first changed it. |
| `NEM_MANAGED_KEYS` | `nem env` | Space-separated names of the variables nem currently manages. |
| `NEM_SAVED__<NAME>` | `nem env` | The value `<NAME>` had before nem set it. |
| `NEM_SAVED__<NAME>_SET` | `nem env` | Whether `<NAME>` existed before nem set it, so leaving a project can unset it again. |
| `DYLD_LIBRARY_PATH` on macOS, `LD_LIBRARY_PATH` on Linux | `nem env` | Composed only when a library package is in the environment. |

Because of this bookkeeping, the prefixes `NEM_`, `LD_`, `DYLD_` and the
suffix `_SET` are on the [reserved list](../../using/environment-variables/#reserved-names),
together with names such as `PATH` and `PS1`, matched case-insensitively.
A `[env]` entry using one is dropped with a warning; a package export using
one fails `nem catalog lint`, and one that reached a catalog anyway is
dropped with the same warning when the environment is composed.

`nem exec` sets the same `PATH`, loader variable, and `[env]` values in its
child process, with two differences: it keeps none of the `NEM_SAVED__*`,
`NEM_MANAGED_KEYS`, or `NEM_ORIGINAL_PATH` bookkeeping, and it prepends to
the `PATH` it inherited rather than rebuilding from `NEM_ORIGINAL_PATH`.

## Variables in build and test steps

`nem catalog build` and `nem catalog test` run a package's `build.steps` and
`test` scripts under `sh -c`. The steps inherit nem's own environment, and
nem sets these on top of it:

| Variable | Build | Test | Meaning |
|---|---|---|---|
| `NEM_VERSION` | yes | yes | The version being built or tested. |
| `NEM_OS`, `NEM_ARCH` | yes | yes | The platform, such as `darwin` and `arm64`. |
| `NEM_PREFIX` | yes | yes | In a build, the directory the package will install into. In a test, the throwaway install under `packages/<name>-NEMTEST-*/`, deleted when the test finishes; never record it. |
| `NEM_STAGING_DIR` | yes | no | The staging root; the steps run in the unpacked source below it. |
| `NEM_OUTPUT` | yes | no | The absolute path of `build.output`; steps fill it. |
| `PWD` | yes | yes | The directory the step runs in: the unpacked source in a build, a scratch directory in a test. |
| `NEM_DEP_<NAME>_PREFIX` | yes | yes | The install directory of each resolved dependency, transitive ones included, with the name upper-cased and other characters replaced by `_`. A build resolves `build.deps`; a test resolves the run-time `deps`, plus one entry for the package itself under its throwaway name. |
| `PATH` | yes | yes | The `bins` of dependencies on the path or loader path, prepended; in a test the package's own `bins` come first. |
| `CPPFLAGS`, `CFLAGS`, `CGO_CFLAGS` | yes | yes | `-I` flags for each link dependency's include directory, appended after any inherited value. |
| `LDFLAGS`, `CGO_LDFLAGS` | yes | yes | `-L` and rpath flags for each link dependency, appended; on macOS always `-Wl,-headerpad_max_install_names` as well. |
| `PKG_CONFIG_PATH` | yes | yes | Each link dependency's `pkgconfig` directories, joined with the path-list separator and appended. |
| `DYLD_LIBRARY_PATH` on macOS, `LD_LIBRARY_PATH` on Linux | no | yes | The `libs` of the package and its link dependencies; any inherited value is dropped first. |

## Related

- [NEM_HOME](../nem-home/)
- [Environment variables](../../using/environment-variables/)
- [Shell integration](../../using/shell-integration/)
- [pkg.yaml](../pkg-yaml/#build)
