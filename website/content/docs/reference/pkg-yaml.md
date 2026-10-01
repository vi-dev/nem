---
title: pkg.yaml
weight: 5
---

The package manifest: one `pkgs/<name>/pkg.yaml` per package in a catalog,
schema 2. It says where a package's artifacts come from, how they are laid
out on disk, what the package exports, and which versions exist with their
checksums. Parsing is strict: an unknown key anywhere is an error, except
inside `sha256`, where keys for other platforms are ignored.
`nem catalog lint` validates a manifest and `nem catalog fmt` rewrites it in
canonical form.

## Example: a prebuilt binary

```yaml
schema: 2
name: kubectl
description: The Kubernetes command-line tool
homepage: https://kubernetes.io/docs/reference/kubectl/
license: Apache-2.0

versionDiscovery:
  github:
    repo: kubernetes/kubernetes
    filter: '^v\d+\.\d+\.\d+$'
    prefix: "v"

artifact:
  url: "https://dl.k8s.io/release/v{{.Version}}/bin/{{.OS}}/{{.Arch}}/kubectl"

install:
  - copy: {src: "{{.Artifact}}", dst: "bin/kubectl", mode: 0o755}

bins: ["bin"]

versions:
  - version: 1.37.0
    sha256:
      darwin/arm64: "583beedaebe422e71d3f1a96acef8b1fef86ea2f09a45ad01aa6c9ce287c1380"
      darwin/amd64: "d5276c0f4fde77fc446070290f345944a7f1fda153df6b960e5fde93b7a9bccd"
      linux/arm64: "922df28df248cc00a9e025f947704f1d1482de64ece54cfe57e61f19eaf1eef3"
      linux/amd64: "6129359f4e1f3848a5572ccb0b26cf28b8ca08cef38c95a765b2f64a2c961a2f"
```

## Example: built from source

```yaml
schema: 2
name: aardvark-dns
description: Authoritative DNS server for container networks
homepage: https://github.com/containers/aardvark-dns
license: Apache-2.0

platforms: [linux/arm64, linux/amd64]

versionDiscovery:
  github:
    repo: containers/aardvark-dns
    filter: '^v\d+\.\d+\.\d+$'
    prefix: "v"

artifact:
  oci: ":{{.Version}}"

install:
  - extract: {strip: 0}

versions:
  - version: 2.1.0
    sourceSha256: "daf871488603e659b0501224cf0731ac317809b1d1701fc061cb4f6ae39a894f"

build:
  deps:
    - rust
  source:
    url: "https://github.com/containers/aardvark-dns/archive/refs/tags/v{{.Version}}.tar.gz"
  output: out
  steps:
    - run: |
        set -e
        cargo build --release --locked
        mkdir -p "$NEM_OUTPUT/bin"
        cp target/release/aardvark-dns "$NEM_OUTPUT/bin/aardvark-dns"

test:
  - run: aardvark-dns --version | grep -q "$NEM_VERSION"
```

## Top-level keys

| Key | Required | Meaning |
|---|---|---|
| `schema` | yes | Must be `2`. |
| `name` | yes | Package name, `^[a-z0-9][a-z0-9._-]*$`. Also the directory name under `pkgs/`. |
| `description` | no | One line, shown by `nem search` and `nem info`. |
| `homepage` | no | URL shown by `nem info`. |
| `license` | no | SPDX identifier, shown by `nem info`. |
| `platforms` | no | Platforms the package supports; default all four. See [`platforms`](#platforms). |
| `deps` | no | Packages this one needs at run time. See [`deps`](#deps). |
| `versionDiscovery` | no | Where `nem catalog outdated` and `bump` find new versions. See [`versionDiscovery`](#versiondiscovery). |
| `artifact` | yes | Where each version's file comes from. See [`artifact`](#artifact). |
| `install` | yes | How the artifact is laid out in the install directory. See [`install`](#install). |
| `bins` | no | Directories that join `PATH`; default `[bin]`. See [`bins` and `libs`](#bins-and-libs). |
| `libs` | no | Directories that join the loader path. |
| `env` | no | Environment variables the package exports. See [`env`](#env). |
| `build` | no | How to build the package from source. See [`build`](#build). |
| `test` | no | Commands `nem catalog test` runs against an installed version. See [`test`](#test). |
| `versions` | yes | The versions that exist, newest first, with checksums. See [`versions`](#versions). |

## `platforms`

A list drawn from `darwin/arm64`, `darwin/amd64`, `linux/arm64`, and
`linux/amd64`; a bare `darwin` or `linux` means every architecture of that
OS, and anything else is rejected. Omit it to support all four.
`deps`, `install` actions, `env` exports, `build.steps`, and `test` steps each
accept their own `platforms` list to apply only on a subset.

## `deps`

Run-time dependencies. Each entry is either a string, `name` or
`name@version`, or a mapping:

```yaml
deps:
  - jq                      # any version, resolved like a nem use without one
  - name: libgpg-error      # a library the binaries load at run time
    kind: link
    compat: "1"
  - name: docker            # only where the package needs it
    platforms: [linux/amd64]
```

| Key | Meaning |
|---|---|
| `name` | Package name, `^[a-z0-9][a-z0-9._-]*$`. |
| `version` | Exact version to require. Without it the dependency floats to the first entry of its `versions` list. |
| `platforms` | Platforms on which the dependency applies. |
| `kind` | `run` (default): the dependency's `bins` join `PATH` beside the package. `link`: a library the package's binaries load; its `libs` join the loader path and nem links it into the package's `.nem-link-dependencies` directory. |
| `compat` | Only with `kind: link`. A release line such as `"1"` or `"3.2"`, `^\d+(\.\d+)*$`. A version matches when its leading components equal the line; the resolver picks the highest match. Two packages that need incompatible lines of the same library fail to resolve with a compat conflict. |

## `versionDiscovery`

Exactly one source. `nem catalog outdated` compares what it finds with the
newest entry in `versions`; `nem catalog bump` adds the missing ones.

```yaml
versionDiscovery:
  github:                       # tags of a GitHub repository
    repo: kubernetes/kubernetes
    filter: '^v\d+\.\d+\.\d+$'
    prefix: "v"
```

| Source | Keys | How versions are found |
|---|---|---|
| `github` | `repo` (required), `filter`, `prefix`, `suffix` | Tags of `github.com/<repo>`, read over git's smart HTTP transport. |
| `gitlab` | `repo` (required), `filter`, `prefix`, `suffix` | Tags of `gitlab.com/<repo>`, the same way. |
| `git` | `url` (required, `http(s)`), `filter`, `prefix`, `suffix` | Tags of any repository reachable over HTTP. |
| `http` | `url` (required, `http(s)`), `filter` (required) | The body at `url`, scanned with `filter`. |
| `oci` | a repository reference | The repository's tags are the versions. |

For the three tag sources, `filter` is a regular expression matched
anywhere in the tag, so anchor it with `^` and `$` to mean the whole tag;
without a `filter`, every tag is kept.
When it contains a named group `(?P<version>…)`, that group is the version
and every other named group becomes a `meta` value on the discovered entry.
Without a `version` group, tags that match are kept and `prefix` and
`suffix` are trimmed off to produce the version; no `meta` is collected. For
`http`, the version is the `version` group, else the first group, else the
whole match; other named groups become `meta` only alongside a `version`
group. Characters that cannot appear in an OCI tag are replaced with `_` so
the result is usable as a version; a tag longer than 128 characters still
fails validation.

## `artifact`

Exactly one of `url`, `github`, or `oci`.

```yaml
artifact:
  url: "https://example.com/tool-{{.Version}}-{{.OS}}-{{replace .Arch \"amd64\" \"x86_64\"}}.tar.gz"
```

| Key | Meaning |
|---|---|
| `url` | A Go template rendered per version and platform with `.Version`, `.OS`, `.Arch`, and `.Meta.<key>`, and the functions `trimPrefix`, `trimSuffix`, `replace`, `versionMajor`, `versionMajorMinor`. |
| `github` | `repo` and `asset`; `asset` is a template like `url`. Resolves to `https://github.com/<repo>/releases/download/<version>/<asset>`. Lint does not check that either is set; a missing one fails at download time. |
| `oci` | An OCI reference, templated with `.Version`, `.OS`, `.Arch` and `trimPrefix`, `trimSuffix`, `replace`. A relative form, `":<tag>"` or `"@<digest>"`, names the package's own archive in the catalog's archive store: the registry for an `oci` catalog, `archives/<name>/` inside the directory for a `dir` catalog. That is how built-from-source packages are consumed. |

`url` and `github` artifacts need a `sha256` for every supported platform in
every version. `oci` artifacts are verified by the registry digest, never
against `sha256`; leave the map out, because if it is present at all lint
requires an entry for every supported platform.

When a catalog has published an archive for a version, nem pulls that first
and falls back to the upstream URL only when none exists. `nem catalog fill`
is what publishes archives for `url` and `github` artifacts.

## `install`

A list of actions run in order inside a staging directory that becomes
the install directory. Each action has exactly one action key and an
optional `platforms` list, and at least one action must apply to the
platform being installed, or the install fails. Lint does not check that,
unlike the same mistake in a `test` step.

```yaml
install:                                                      # an archive
  - extract: {strip: 1}
  - move: {src: "server/tool", dst: "bin/tool"}
```

```yaml
install:                                                      # a single binary
  - copy: {src: "{{.Artifact}}", dst: "bin/tool", mode: 0o755}
```

| Action | Keys | Effect |
|---|---|---|
| `extract` | `strip` | Unpack a tar or zip artifact, compressed or not, into the install directory, dropping `strip` leading path components. A compressed single file that is not an archive is written as one executable named after the artifact minus its `.gz`, `.bz2`, `.xz`, or `.zst` suffix, and `strip` is ignored. An uncompressed single file is not extractable; use `copy`. |
| `copy` | `src`, `dst`, `mode` | Copy a file. `src` is a path inside the install directory or `{{.Artifact}}` for the downloaded file itself; `mode` is octal, default `0644`. |
| `move` | `src`, `dst` | Rename a path inside the install directory. |
| `mkdir` | a path | Create a directory. |

Paths are templates over `.Version`, `.OS`, and `.Arch` with the same
helper functions as `artifact.url`, and must stay inside the install
directory. `{{.Artifact}}` stands for the downloaded file only when it is
the entire `src` of a `copy`; anywhere else it is literal text.

## `bins` and `libs`

Both are lists of directories relative to the install root.

`bins` (default `[bin]`, also when the list is empty) are the directories
that join `PATH` whenever the package is on it, whether declared directly or
as a `run` dependency; they are not checked to exist.
`libs` are the directories that join the loader path, `DYLD_LIBRARY_PATH` on
macOS and `LD_LIBRARY_PATH` on Linux, when the package is declared directly
or used as a `link` dependency. A library package must declare `libs` to be
usable as a `link` dependency at all; without it nothing joins the loader
path.

## `env`

Environment variables the package exports into every environment that
includes it.

```yaml
env:
  - name: JAVA_HOME
    value: "{{.InstallDir}}"
    platforms: [linux/arm64, linux/amd64]
```

| Key | Meaning |
|---|---|
| `name` | `^[A-Za-z_][A-Za-z0-9_]*$`. A name on nem's [reserved list](../../using/environment-variables/#reserved-names) is rejected by `nem catalog lint`. |
| `value` | A template over `.InstallDir` and `.Version` with `trimPrefix`, `trimSuffix`, `replace`. |
| `platforms` | Platforms on which the export applies. |

Exports sit below a project's own `[env]` in
[precedence](../../using/environment-variables/#what-wins).

## `build`

Present when the package is built from source rather than downloaded.
`nem catalog build` runs it on the host platform and, with `--push`,
publishes the result as the package's archive, which is why such packages
declare a relative `oci` artifact.

| Key | Required | Meaning |
|---|---|---|
| `source` | yes | `url`: where the source archive comes from, templated like `artifact.url`. |
| `steps` | yes | A list of `run` scripts, each with optional `platforms`, executed with `sh -c` inside the unpacked source. |
| `output` | yes | The directory, relative to the unpacked source, that the steps fill. It becomes the installed package. |
| `deps` | no | Build-time dependencies, in the same shape as `deps`. |
| `normalize` | no | Default `true`. After the steps, nem drops libtool `.la` files, rewrites `pkg-config` files to relocatable paths, on macOS fixes Mach-O install names and rpaths, and plants a `.nem-link-dependencies` link in every subdirectory holding a binary that links a dependency. The verification step that follows requires those links, so with `normalize: false` the build fails unless the steps plant them themselves. |

Steps see `NEM_VERSION`, `NEM_OS`, `NEM_ARCH`, `NEM_PREFIX` (the directory
the package installs into), `NEM_STAGING_DIR`, `NEM_OUTPUT` (the absolute
`output` directory), and `NEM_DEP_<NAME>_PREFIX` for each dependency, with
the name upper-cased and other characters replaced by `_`. `PATH`,
`CPPFLAGS`, `CFLAGS`, `LDFLAGS`, `CGO_CFLAGS`, `CGO_LDFLAGS`, and
`PKG_CONFIG_PATH` are prepared so the dependencies are found. The full list
is under [Configuration variables](../environment/#variables-in-build-and-test-steps).

A version of a built package carries `sourceSha256`, the checksum of the
downloaded source archive, instead of per-platform `sha256` values. It is
optional; a build without it only reports the checksum for you to record.

## `test`

Commands that prove an installed version works. Each entry has `run` and an
optional `platforms` list.

```yaml
test:
  - run: tool --version | grep -q "$NEM_VERSION"
  - run: tool --help | grep -q usage
    platforms: [linux/arm64, linux/amd64]
```

`nem catalog test` installs the version into a throwaway directory, then
runs each script with `sh -c` in a scratch directory with the package's and
its dependencies' `bins` on `PATH`, link dependencies on the loader path,
its `env` exports applied, and the same `NEM_*` and compiler variables as a
build step except `NEM_STAGING_DIR` and `NEM_OUTPUT`; see
[Configuration variables](../environment/#variables-in-build-and-test-steps).
`NEM_PREFIX` names the throwaway install, which is deleted after the test.
A package with no `test` entries is install-verified only.

## `versions`

The versions that exist, newest first. An entry is either a bare string or a
mapping:

```yaml
versions:
  - version: 1.37.0
    meta: {date: "20260814"}
    sha256:
      darwin/arm64: "…"
      linux/amd64: "…"
  - 1.36.4                      # shorthand for {version: 1.36.4}; usable only with oci artifacts
```

| Key | Meaning |
|---|---|
| `version` | Must be a valid OCI tag, `^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`, because it names the package's archive tag. |
| `meta` | Extra values per version; keys `^[A-Za-z][A-Za-z0-9_]*$`, values non-empty. Reachable as `.Meta.<key>` in `artifact.url`, `artifact.github.asset`, and `build.source.url`, and filled from discovery's named groups. |
| `sha256` | A map from platform to a 64-character lowercase hex digest. Required for `url` and `github` artifacts, and it must cover every supported platform; lint checks presence per platform, not the value, which a wrong digest fails at install time. |
| `sourceSha256` | The digest of the source archive for a built package. |

A `nem use` without a version takes the first entry. `nem catalog lint`
rejects a list that is not strictly newest first, duplicates included, and
`nem catalog bump` inserts new versions in order. The resolver steps further down the list only when
another package requires an older or `compat`-constrained version; it never
picks anything above the first entry.

## What lint enforces

Beyond the rules above, `nem catalog lint` reports:

- `name` that does not match the directory the manifest lives in;
- an `artifact.url` or `artifact.github.asset` template that fails to
  expand for any version and platform;
- an `env` export whose name is reserved;
- `versions` that are not newest first;
- a `test` step whose `platforms` overlap none of the package's.

Over a whole catalog it also reports a manifest it cannot read, a missing
`pkgs/` directory, and a `pkgs/` directory with no packages, and it prints
every validation error above verbatim.

## Related

- [nem catalog lint](../cli/nem-catalog-lint/), [nem catalog fmt](../cli/nem-catalog-fmt/), [nem catalog bump](../cli/nem-catalog-bump/), [nem catalog build](../cli/nem-catalog-build/), [nem catalog test](../cli/nem-catalog-test/)
- [Writing nem packages](../../writing-packages/)
- [NEM_HOME](../nem-home/)
- [Environment variables](../../using/environment-variables/)
