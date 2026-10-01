---
title: Artifacts and platforms
weight: 4
---

An artifact is a template that turns a version and a platform into one
download, and nem has four platforms. The template, the checksum per
platform, and whether the catalog carries an archive are what decide how a
version installs.

## The platforms

nem installs on `darwin/arm64`, `darwin/amd64`, `linux/arm64`, and
`linux/amd64`. A manifest supports all four unless it says otherwise:

```yaml
platforms: [linux/arm64, linux/amd64]     # Linux only
```

```yaml
platforms: [darwin]                       # both macOS architectures
```

Every platform the package supports needs a checksum in every version, and
`nem catalog test` and `build` only ever act on the platform they run on.

## URL templates

`artifact.url` is a Go template rendered once per version and platform
with `.Version`, `.OS`, and `.Arch`:

```yaml
artifact:
  url: "https://dl.k8s.io/release/v{{.Version}}/bin/{{.OS}}/{{.Arch}}/kubectl"
```

Upstreams rarely use nem's names, so the functions `replace`,
`trimPrefix`, `trimSuffix`, `versionMajor`, and `versionMajorMinor` are
available:

```yaml
artifact:
  url: "https://example.com/tool-{{.Version}}-{{replace .OS \"darwin\" \"macos\"}}-{{replace (replace .Arch \"arm64\" \"aarch64\") \"amd64\" \"x86_64\"}}.tar.gz"
```

```yaml
artifact:
  url: "https://example.com/{{versionMajorMinor .Version}}/tool-{{.Version}}.tar.gz"
```

Values discovered as `meta` are reachable as `.Meta.<name>`; see
[Version discovery](../version-discovery/#named-groups-and-meta).

## GitHub release assets

For a release asset the repository and asset name are enough:

```yaml
artifact:
  github:
    repo: example/tool
    asset: "tool_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
```

That resolves to `https://github.com/<repo>/releases/download/<version>/<asset>`
and behaves exactly like a `url`.

## Archives in the catalog

```yaml
artifact:
  oci: ":{{.Version}}"                    # this package's archive in the catalog
```

The relative form names the package's own archive store beside the
catalog: the registry for an `oci` catalog, `archives/<name>/` inside the
directory for a `dir` catalog. It is what built-from-source packages use,
because there is no upstream download; see
[Building from source](../build-from-source/). An absolute reference such
as `ghcr.io/example/tool:{{.Version}}` pulls from any registry.

## Checksums

`url` and `github` artifacts carry a `sha256` per platform in every
version. Type them by hand once if you must; `nem catalog bump` downloads
each platform's file and writes them for you. Archives referenced with
`oci` are verified by the registry digest instead and carry none.

## What happens at install time

nem looks for a published archive of the version in the catalog first and
downloads from the upstream URL only when there is none. Publishing
archives for `url` and `github` packages, so that consumers never reach
upstream, is what `nem catalog fill` does; see
[Mirroring and filling](../../managing-catalogs/mirror-and-fill/#fill).

## Single files

A URL that points at a bare binary is placed with `copy`:

```yaml
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/tool", mode: 0o755}
```

A compressed single file, `argo-linux-amd64.gz` for example, is
`extract`ed first: it lands in the install root as one executable named
after the URL's last path segment minus the compression suffix, which is
neither in `bin/` nor named after the command. A `copy` puts it where
`PATH` will find it:

```yaml
install:
  - extract: {strip: 0}
  - copy: {src: "argo-{{.OS}}-{{.Arch}}", dst: "bin/argo", mode: 0o755}
```

## Related

- [pkg.yaml: artifact](../../reference/pkg-yaml/#artifact), [pkg.yaml: platforms](../../reference/pkg-yaml/#platforms)
- [Install actions](../install-actions/)
- [nem catalog bump](../../reference/cli/nem-catalog-bump/)
