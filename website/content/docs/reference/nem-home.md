---
title: NEM_HOME
weight: 6
---

All of nem's on-disk state lives under one per-user directory: `$NEM_HOME`,
default `~/.nem`. Set `NEM_HOME` to relocate it; the value is used as given,
with no `~` expansion, and an empty value counts as unset. Outside it, nem
writes only a project's own `nem.toml` and `nem.lock`, the managed shell-rc
block that `nem activate` installs, the binary itself when `nem self update`
replaces it, and the manifests and archives of a `dir` catalog that
`nem catalog fmt`, `bump`, `build --push`, or `fill` edits. Nothing anywhere
needs root.

## Layout

```
~/.nem/
├── config.yaml                      # nem's configuration: catalog list, host settings
├── nem.toml                         # global manifest
├── nem.lock                         # global lockfile
├── lock                             # internal process lock — never deleted
├── usage.json                       # last-use stamps; read by `clean --unused`
├── tmp/
│   ├── *.tmp                        # in-flight downloads
│   └── *-build-*/                   # build and test scratch
├── packages/
│   ├── <name>/<version>/            # one immutable tree per installed version
│   │   ├── .nem-meta.yaml           # what was installed, from which catalog
│   │   └── .nem-link-dependencies/  # links to link deps, when the package has any
│   ├── <name>/<version>-*.tmp/      # an install in progress
│   └── <name>-NEMTEST-*/            # a `catalog test` install in progress
└── catalogs/
    └── <name>/store/                # synced copy of an oci catalog
```

- **`config.yaml`** — see [config.yaml](../config-yaml/).
- **`nem.toml` / `nem.lock`** — the global twins of the project files; see
  [nem.toml](../nem-toml/) and [nem.lock](../nem-lock/).
- **`lock`** — a flock held while nem writes manifests, lockfiles, or the
  config, syncs a catalog, installs or relinks packages, runs `nem clean`,
  or runs `nem catalog test`. The file itself is never deleted.
- **`usage.json`** — a stamp per installed version, refreshed when an
  environment is composed (`nem env` on a directory change, `nem exec`),
  when a version is installed, and by `nem catalog build`. A `nem sync`
  that installs nothing refreshes nothing. `clean --unused` evicts versions
  whose stamp is older than the window.
- **`tmp/`** — in-flight downloads as `*.tmp`, and build and test scratch
  directories carrying `-build-` in their name. Cleaned opportunistically.
- **`packages/<name>/<version>/`** — one installed version, addressed by
  name and exact version. `.nem-meta.yaml` inside it records the package,
  version, catalog, `bins`, `libs`, `env` exports, layout version, and
  install time; `nem env`, `status`, and `which` compose the environment
  from these files. `.nem-link-dependencies/<dep>` is present only when the
  package has link dependencies: a relative symlink to the dependency
  version the lock resolved, which `nem sync`, `use`, `update`, and `lock`
  refresh. Subdirectories holding a binary built against a link dependency
  carry their own relative `.nem-link-dependencies` symlink, written at
  build time and shipped in the archive.
- **`packages/<name>/<version>-*.tmp/`** — install staging, renamed into
  place when the install succeeds.
- **`packages/<name>-NEMTEST-*/`** — the throwaway alias that
  `nem catalog test` installs into, removed when the test finishes.
- **`catalogs/<name>/store/`** — the synced copy of an `oci` catalog's
  index and package manifests; `dir` catalogs have nothing here, since
  they're read straight from their configured path.

Every path segment nem creates under `NEM_HOME` must match
`^[A-Za-z0-9][A-Za-z0-9._+-]*$`.

## Install properties

A version counts as installed exactly when `packages/<name>/<version>/`
exists; the `-*.tmp` and `-NEMTEST-` neighbours above never count. Installs
commit by atomic rename, so a half-finished install is never visible as an
installed version — either the whole tree is there, or nothing is.
Interrupted staging left behind by an install that didn't finish is swept
up by later runs.

## Reclaiming space

```shell
nem clean
```

Bare `nem clean` touches only provable garbage and never prompts, so it's
safe to run unattended. Four classes count:

| Garbage | Where |
|---|---|
| leaked staging | `tmp/*-build-*/` |
| leaked download | `tmp/*.tmp` |
| partial install | `packages/<name>/<version>-*.tmp/` |
| leftover test install | `packages/<name>-NEMTEST-*/` |

Anything in those classes younger than `--grace` (default `1h`) is left
alone, which is why `nem clean` straight after a failed build may report
nothing to reclaim. `--unused <age>` and `--all` go further and evict
installed package *versions*; a project gets them back with `nem sync`.

See [Maintenance](../../using/maintenance/#reclaiming-disk-space) for the
caveat on how `--unused` measures "unused", and
[nem clean](../cli/nem-clean/) for every flag.
