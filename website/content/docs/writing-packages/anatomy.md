---
title: Anatomy of a package
weight: 2
---

A manifest answers five questions: what the package is, where its versions
come from, where its files come from, how they install, and what it needs
and gives. The [Quickstart](../quickstart/) manifest is the running example;
the exact rule for each key is on the
[pkg.yaml reference](../../reference/pkg-yaml/).

## Identity

```yaml
schema: 2
name: jq
description: Command-line JSON processor
homepage: https://jqlang.org
license: MIT
```

`schema` is always `2`. `name` is what people type after `nem use`, and it
must match the directory the manifest lives in. `description` is shown by
`nem search` and `nem info`, `homepage` and `license` by `nem info`, and
nothing else depends on them.

## Where versions come from

```yaml
versionDiscovery:
  github:
    repo: jqlang/jq
    filter: '^jq-\d+\.\d+(\.\d+)?$'
    prefix: "jq-"
```

Optional, but it is what lets `nem catalog outdated` notice a new upstream
release and `nem catalog bump` add it with checksums. Without it, new
versions are typed by hand. See [Version discovery](../version-discovery/).

## Where files come from

```yaml
artifact:
  url: "https://github.com/jqlang/jq/releases/download/jq-{{.Version}}/jq-{{replace .OS \"darwin\" \"macos\"}}-{{.Arch}}"
```

One download per version and platform. A template turns nem's names for
the OS and architecture into whatever the upstream uses. Packages built
from source point at their own archives instead. See
[Artifacts and platforms](../artifacts-and-platforms/).

## What happens on install

```yaml
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/jq", mode: 0o755}
bins: ["bin"]
```

Each version installs into its own directory under `NEM_HOME`, and the
actions say how the downloaded file ends up there. `bins` lists the
directories that join `PATH`; `bin` is the default, so the line above could
be left out. See [Install actions](../install-actions/).

## What the package needs and gives

```yaml
deps:
  - name: libgpg-error
    kind: link
    compat: "1"
env:
  - name: JAVA_HOME
    value: "{{.InstallDir}}"
```

Neither appears in the jq manifest because jq needs nothing and exports
nothing. `deps` pulls other packages in alongside this one; `env` puts
values in the environment of every project that uses it; `libs` marks a
package as a library others can link against. See
[Dependencies and exports](../dependencies-and-exports/).

## The versions

```yaml
versions:
  - version: 1.8.2
    sha256:
      darwin/arm64: "2d75…"
      darwin/amd64: "e94b…"
      linux/arm64: "8b85…"
      linux/amd64: "b1c2…"
```

Every version that can be installed, newest first, with a checksum per
platform. A `nem use` without a version takes the first entry, and lint
rejects a list that is out of order. `nem catalog bump` writes these
entries so you rarely type a checksum yourself.

## When build and test appear

`build` replaces the download with a recipe: where the source is, what to
run, and what directory the result lands in. `test` is a list of shell
commands that prove an installed version works. Both are optional and have
their own guides, [Building from source](../build-from-source/) and
[Testing packages](../testing/).

{{< callout type="info" >}}
Manifests are strict: a key nem does not know fails lint instead of being
ignored. `nem catalog fmt` rewrites a manifest in a stable layout and keeps
your comments, so formatting never becomes a review topic.
{{< /callout >}}

## Related

- [pkg.yaml](../../reference/pkg-yaml/)
- [Quickstart](../quickstart/)
- [Authoring workflow](../workflow/)
