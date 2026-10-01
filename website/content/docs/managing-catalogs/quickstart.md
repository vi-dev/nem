---
title: Quickstart
weight: 1
---

At the end of this page your registry holds a copy of the official catalog
with archives for the packages you chose, and `nem use` installs from it.

{{% steps %}}

### Log in to your registry

nem reads Docker's credentials, so logging Docker in logs nem in:

```shell
docker login registry.corp.example      # nem reads ~/.docker/config.json
```

A registry on `localhost` needs no TLS setup; anything else with a private
CA or plain HTTP needs a `hosts:` entry first, see
[Access and rollout](../access-and-rollout/#private-registries).

### Mirror the catalog

```shell
nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2 --dry-run   # what would be copied
nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2             # copy it
```

Both references need a tag or digest. The run copies the index and every
package manifest, then every archive the source has, and ends with a
summary:

```
OK Mirrored 829 packages, 147 tags
```

829 packages but only 147 archive tags: the official catalog carries
archives for packages built from source and for little else. Everything
else still points at upstream downloads, which is what the next step
fixes.

### Fill the archives

```shell
nem catalog fill registry.corp.example/nem/catalog:v2 --package jq --dry-run   # start with one package
nem catalog fill registry.corp.example/nem/catalog:v2 --package jq             # download, verify, publish
nem catalog fill registry.corp.example/nem/catalog:v2                          # everything, when ready
```

`fill` downloads each version's artifact for every platform, verifies it
against the manifest's checksum, and publishes it as an archive beside
your copy of the index. It runs against your mirror because publishing
needs push access. The first run for jq publishes four archives, one per
platform:

```
OK Filled jq (4 fills, 0 heals) (2s)
OK Filled 1 package, 4 fills, 0 heals, 0 present, 0 packages not fillable
```

A second run finds everything present:

```
OK Filled 1 package, 0 fills, 0 heals, 4 present, 0 packages not fillable
```

The full catalog is a lot of downloads; `--package` for the packages your
developers use is a good starting scope.

### Add it and use it

```shell
nem catalog add corp registry.corp.example/nem/catalog:v2   # appended after official
nem catalog reorder corp official                           # make it win over official
```

Or, if the mirror should be the only source, `nem catalog disable
official` instead of the `reorder`. Then, in any project:

```shell
nem use jq                                                  # resolves from corp, installs from your registry
```

The first `nem use` syncs the new catalog. Archives resolve from your
registry automatically, because their location is derived from the
catalog reference.

### Tell developers

Each developer runs the same `nem catalog add`, or gets a ready
`config.yaml`; [Access and rollout](../access-and-rollout/) covers both,
and [Keeping a mirror current](../keeping-current/) covers the refresh
loop.

{{% /steps %}}

## Related

- [nem catalog mirror](../../reference/cli/nem-catalog-mirror/), [nem catalog fill](../../reference/cli/nem-catalog-fill/)
- [How catalogs work](../how-catalogs-work/), [Mirroring and filling](../mirror-and-fill/)
