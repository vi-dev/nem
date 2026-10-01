---
title: CI and containers
weight: 6
aliases:
  - /docs/guides/containers-and-ci/
---

A pipeline or container has no shell hook, so the whole pattern is two
commands: `nem sync` to install what the lockfile pins, then `nem exec` to
run each step in the composed environment. The images and the caching
advice below exist to make those two fast.

In a container or a pipeline there is no shell hook. `nem sync` installs
what the lockfile pins and `nem exec` runs each step in the composed
environment; that is the whole pattern.

## Images

`nem`'s multi-arch images are published to
[`ghcr.io/vi-dev/nem`](https://github.com/vi-dev/nem/pkgs/container/nem)
on every release, signed with cosign and with SBOMs attached:

```shell
docker run --rm ghcr.io/vi-dev/nem:latest --help
```

Stable releases are tagged `vX.Y.Z`, `vX.Y`, `vX`, and `latest`; the
`unstable` tag tracks `main`. The entrypoint is `nem` and `NEM_HOME` is
`/root/.nem`.

The standard image carries a build toolchain, so packages can be
[built from source](../../writing-packages/build-from-source/) inside it
with `nem catalog build`.

## Rootless variants

Every tag has a `-rootless` companion (`vX.Y.Z-rootless`, …,
`rootless`, `unstable-rootless`).

This is a slim image that runs as user `nem` (uid 1000) with `NEM_HOME` at
`/home/nem/.nem`, and carries no build toolchain. It is meant for consuming
packages — as a CI job's base image, or as the base of a
[dev container](../agents/#dev-containers).

## The CI pattern

There's no shell hook to install in a pipeline. Check out the project,
run `nem sync` to install whatever the lockfile pins that's missing on the
runner, then run each step under `nem exec` so it sees the composed
environment:

```shell
nem sync                               # install what nem.lock pins
nem exec -- kubectl apply -f manifests/   # each step in the composed environment
nem exec -- go test ./...
```

The catalog store starts empty on a fresh runner; `nem sync` syncs it before
looking anything up. A restored but stale store is not re-synced, so a
pipeline that caches the store runs `nem catalog update` when it needs
newer manifests.

## Caching

`$NEM_HOME/packages` (default `~/.nem/packages`) holds every package nem has
installed. Cache it between runs, keyed on the lockfile, so a pipeline only
downloads when `nem.lock` changes:

```yaml
- uses: actions/cache@v4
  with:
    path: ~/.nem/packages
    key: nem-${{ runner.os }}-${{ hashFiles('nem.lock') }}
    restore-keys: nem-${{ runner.os }}-
- run: curl -fsSL https://raw.githubusercontent.com/vi-dev/nem/main/install.sh | bash
- run: echo "$HOME/.local/bin" >> "$GITHUB_PATH"
- run: nem sync
- run: nem exec -- go test ./...
```

The catalog store under `$NEM_HOME/catalogs` is small and `nem sync` fills
it in seconds, so cache only `packages`. See the
[NEM_HOME reference](../../reference/nem-home/) for everything that lives
under it.

## Related

- [nem sync](../../reference/cli/nem-sync/), [nem exec](../../reference/cli/nem-exec/)
- [Coding agents](../agents/)
- [NEM_HOME](../../reference/nem-home/)
