---
title: Keeping a mirror current
weight: 7
---

A mirror is kept current by running `mirror` and `fill` again. Both skip
what is present, so the loop is cheap; the work is knowing what changed and
when developers see it.

## The loop

```shell
nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2   # new manifests and archives
nem catalog fill registry.corp.example/nem/catalog:v2                                   # archives for the new versions
```

Both commands skip what is already present, so a scheduled run with
nothing new upstream costs a pull of the source catalog plus one probe per
archive and writes nothing. Run them in that order, from a job with push access
to your registry.

## Seeing what a run would bring

`nem catalog diff` compares two catalogs; with the source first and your
mirror second, the rows are what the source has that the mirror lacks:

```shell
nem catalog diff ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2                 # new and updated packages
nem catalog diff ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2 --output json   # for automation
```

```
OK 0 changed (0 new, 0 updated, 0 removed), 829 unchanged
```

`new` is a package the mirror does not have yet and `updated` one whose
manifest changed, typically a new version; `removed` runs the other way,
a package your mirror still has that the source dropped. The base, here
the source, is linted before the comparison and a lint finding stops the
run, so a scheduled job inherits the source's lint state. The JSON form
lists the versions each row adds, which is enough to drive a scoped
`fill`.

## After a refresh

Developers see the new versions after `nem catalog update`. A store that
was never synced is synced by the next `nem use`, but a stale one is not;
`nem update` warns when a catalog was last synced more than a week ago.
Consumers who pinned the catalog by digest see nothing until someone
gives them the new digest, which is what pinning is for.

## Scoped fills

A package that developers ask for can be filled on its own without
walking the whole catalog:

```shell
nem catalog fill registry.corp.example/nem/catalog:v2 --package terraform   # one package, every version
```

## Retention

Every `publish` writes a release tag and every fill or build adds archive
tags, and nothing removes them. Prune with your registry's own tooling when
storage matters, with two exceptions. Never delete an `archives/<name>` tag
for a package built from source: it has no upstream to fall back to, so
that version becomes uninstallable. And never delete an index that a
consumer added by digest, or their `nem catalog update` stops working. A
lockfile pins package manifests by digest, not the index, so index tags
themselves are safe to prune once nothing is pinned to them.

## Related

- [nem catalog mirror](../../reference/cli/nem-catalog-mirror/), [nem catalog fill](../../reference/cli/nem-catalog-fill/), [nem catalog diff](../../reference/cli/nem-catalog-diff/), [nem catalog update](../../reference/cli/nem-catalog-update/)
- [Mirroring and filling](../mirror-and-fill/), [Packages](../../using/packages/)
