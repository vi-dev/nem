---
title: Air-gapped networks
weight: 4
aliases:
  - /docs/guides/air-gapped/
---

Everything nem needs is OCI content, so a registry that both sides can reach
is the whole bridge: the connected side mirrors and fills it, the inside
consumes it.

## The idea

The content is the catalog index, the package manifests, and the archives,
all OCI. The registry can be one both sides reach, or one you carry across
the gap with your registry's own export and import tooling. The official
catalog is published at
`ghcr.io/vi-dev/nem-catalog`, on
[GitHub](https://github.com/vi-dev/nem-catalog), and it is consumed inside
exactly like any other catalog.

## Connected side

```shell
nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2   # index, manifests, existing archives
nem catalog fill registry.corp.example/nem/catalog:v2                                   # every remaining archive
```

Inside there is no upstream to fall back to, so every package that
developers will use must be filled, not only the ones the source already
had archives for. [Mirroring and filling](../mirror-and-fill/) explains
what each command copies and skips.

## Inside

```shell
nem catalog add corp registry.corp.example/nem/catalog:v2   # the mirrored catalog
nem catalog disable official                                # nothing tries to reach ghcr.io
nem catalog update                                          # sync the store
nem use kubectl                                             # from your registry
nem sync                                                    # on every other machine
```

A registry with a private CA or without TLS needs a `hosts:` entry in
[config.yaml](../../reference/config-yaml/) before the `add`. A fresh
machine does not need the `update`: `nem sync` syncs a store that was never
synced before installing, and `nem use` does the same; `update` is what
refreshes a store that already exists. See
[Packages](../../using/packages/#sharing).

## Keeping it current

New upstream versions reach the inside the same way they arrived: re-run
`mirror` and then `fill` on the connected side. Both skip what is already
there. See [Keeping a mirror current](../keeping-current/).

## Related

- [Mirroring and filling](../mirror-and-fill/), [Access and rollout](../access-and-rollout/)
- [config.yaml](../../reference/config-yaml/), [Catalogs](../../using/catalogs/)
