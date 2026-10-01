---
title: Catalogs
weight: 5
aliases:
  - /docs/guides/catalogs/
---

Catalogs are searched in configured order and the first one that has a
package wins. Adding, reordering, pinning, and authenticating all come down
to editing that list, which lives in `config.yaml`.

## What a catalog is

A catalog is an ordered, named source of package manifests. There are two
types:

- `oci` — an OCI registry reference, synced locally. This is how the
  official catalog is distributed.
- `dir` — a local directory laid out as `pkgs/<name>/`[`pkg.yaml`](../../reference/pkg-yaml/), mainly for
  authoring a catalog before publishing it. It is what you use while writing
  your own packages; see the [Writing nem packages quickstart](../../writing-packages/quickstart/).

On first run, `nem` configures the `official` catalog for you. It's a
plain entry like any other — removable, reorderable, disable-able — with
no special reserved status.

{{< callout type="info" >}}
`nem`'s official catalog is hosted at
[`ghcr.io/vi-dev/nem-catalog`](https://github.com/vi-dev/nem-catalog).
{{< /callout >}}

## Lookup order

Catalogs are searched in configuration order, and the first one that has
a package wins. A `[<catalog>:]` prefix on a package in `nem.toml` (for
example `nem use mycatalog:kubectl`) pins that package to one specific
catalog, skipping the rest.

A disabled catalog keeps its slot in the order but is skipped by lookups
— disabling one and re-enabling it later doesn't change where it sits
relative to the others.

## Adding and ordering catalogs

```shell
nem catalog add <name> <ref>      # --type oci|dir; default: auto-detect
nem catalog list
nem catalog remove <name>
nem catalog reorder <name>...     # every configured catalog, exactly once
nem catalog update [name]         # sync oci catalogs from their remote
nem catalog enable <name>...
nem catalog disable <name>...
```

`nem catalog add` auto-detects the type from the reference unless you
pass `--type`, and appends the new catalog after the ones already
configured — a private catalog that should beat `official` needs a
`reorder` afterwards. `reorder` rewrites the whole precedence list at once — it
expects every configured catalog named exactly once. `update` re-syncs
`oci` catalogs against their remote; `dir` catalogs are read live from
disk and need no updating.

All of this is stored in `$NEM_HOME/config.yaml`, and hand-editing the file
works just as well as the commands above. See the
[config.yaml reference](../../reference/config-yaml/) for the full file
format.

## Pinning to a digest

A tag such as `:v2` moves with every release of the catalog. To freeze a
catalog for a reproducible setup, point its entry at a digest instead.
Edit the `ref` in `$NEM_HOME/config.yaml` in place so the catalog keeps its
position in the order:

```yaml
catalogs:
  - name: official
    type: oci
    ref: ghcr.io/vi-dev/nem-catalog@sha256:…
```

Then run `nem catalog update official` once. A digest never moves, so later
updates have nothing new to pull, and every machine that pins the same
digest resolves the same packages.

## Authentication

OCI registries use standard Docker credentials — `nem` has no
authentication store of its own. Run `docker login <registry>` to set up
access, and `nem` picks up the same credentials. A registry that answers
with 401 makes `nem` print exactly that hint, with the host filled in.

## Related

- [nem catalog](../../reference/cli/nem-catalog/)
- [config.yaml](../../reference/config-yaml/)
