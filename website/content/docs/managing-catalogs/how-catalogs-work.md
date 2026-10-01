---
title: How catalogs work
weight: 2
---

A catalog in a registry is an OCI index with one entry per package, plus a
sibling repository of archives per package. Everything an operator does,
mirror, fill, publish, or pin, is an operation on one of those two things.

## The index

A catalog reference such as `ghcr.io/vi-dev/nem-catalog:v2` points at an
OCI index with one entry per package. Each entry is a small OCI image
manifest carrying the package name, description, and latest version as
annotations, and that manifest has a single layer: the package's
`pkg.yaml` as written, with media type `application/vnd.nem.pkg.v2+yaml`.
A catalog with hundreds of packages is one index, one such manifest per
package, and one `pkg.yaml` blob each; nothing else. The digest of a
package's manifest is what `nem.lock` records as `digest`, so a lockfile
pins the exact `pkg.yaml` a version resolved from.

## The archives

A package's archives live in a sibling repository named after the
catalog's, `<registry>/<repository>/archives/<name>`: one tag per version,
and under it one entry per platform. For `ghcr.io/vi-dev/nem-catalog`, jq's
archives are at `ghcr.io/vi-dev/nem-catalog/archives/jq`, tag `1.8.2`.

Packages built from source always have archives, because the archive is
the only form the package exists in. Packages that point at an upstream
download have them only when an operator ran `fill` or, for built
packages, `build --push`.

## What a consumer does

`nem catalog update` pulls the index and every package manifest into
`$NEM_HOME/catalogs/<name>/store`. Resolution reads that store and never
the network. At install time nem looks for an archive of the version in
the catalog's archive repository first; if there is none it downloads from
the upstream URL in the manifest and verifies the checksum. Either way the
bytes are checked against what the manifest pins.

## Tags and digests

A tag such as `v2` moves every time the catalog is published. `nem catalog
publish` also writes a release tag of the form `v2.20260930T092433Z` on
every run, so each published state keeps a permanent name. A digest,
`@sha256:…`, names one exact index: a consumer who adds a catalog by
digest gets that state until they choose another, and `nem catalog
update` has nothing to pull for it.

## The same shape on disk

A `dir` catalog is the registry layout as files: `pkgs/<name>/pkg.yaml`
for the manifests and `archives/<name>/` for archives that `build --push`
wrote. That is why the same commands work on both.

## Why fill exists

A catalog whose packages point at upstream URLs is complete and usable,
but every consumer reaches the internet for every install. `fill` walks the
catalog, downloads each artifact once, verifies it, and publishes it as an
archive beside the index, after which consumers pull from your registry
and upstream can disappear. `mirror` copies only the archives the source
catalog already has, so the order for a self-contained copy is mirror,
then fill.

## Related

- [nem catalog update](../../reference/cli/nem-catalog-update/), [nem catalog mirror](../../reference/cli/nem-catalog-mirror/), [nem catalog fill](../../reference/cli/nem-catalog-fill/), [nem catalog publish](../../reference/cli/nem-catalog-publish/)
- [config.yaml](../../reference/config-yaml/)
- [NEM_HOME](../../reference/nem-home/)
