---
title: Publishing your own catalog
weight: 5
---

`nem catalog publish` turns a directory of package manifests into an OCI
index that other machines can add. It pushes manifests, not archives; those
come from `build --push` and `fill`.

## What publish does

`nem catalog publish` takes a bare repository reference and a local
catalog, a directory or a single `pkg.yaml`:

```shell
nem catalog publish registry.example/nem/catalog . --dry-run   # the plan, nothing pushed
nem catalog publish registry.example/nem/catalog .             # publish under tag v2
```

It lints the catalog first and refuses on findings. The dry run prints
the packages it would publish and the tags it would write:

```
PACKAGE  VERSION
jq       1.8.2
OK Would publish registry.example/nem/catalog (1 package), tags v2, v2.20260930T092433Z
```

The real run pushes each package manifest, skipping any whose bytes are
already in the registry, assembles the index, and tags it:

```
OK Pushed jq 1.8.2
OK Published registry.example/nem/catalog: 1 pushed, 0 unchanged, tags v2, v2.20260930T092433Z
```

Two things to notice. The reference carries no tag: `--tag` says which
tags to move, `v2` by default, and the reference with a tag is refused
with `must be a bare repository ref`. And every run also writes a release
tag, `v2.` followed by the UTC time, so each published state keeps a name
you can pin or roll back to. A second run with nothing changed pushes
nothing and moves the tags to the same index:

```
OK Published registry.example/nem/catalog: 0 pushed, 1 unchanged, tags v2, v2.20260930T092512Z
```

`--force` pushes every manifest again regardless.

## Archives

Publishing pushes manifests, not archives. Packages built from source get
their archives from `nem catalog build --push` against the published
reference with its tag;
see [Building from source](../../writing-packages/build-from-source/).
Packages that download from upstream work without archives, and
[`nem catalog fill`](../mirror-and-fill/#fill) against the published
reference adds them when you want consumers to stop reaching upstream.

## A safe pipeline

The official catalog publishes to a staging tag, proves it, then
promotes, and the same shape works for any catalog:

```shell
nem catalog publish registry.example/nem/catalog . --tag v2-staging     # 1. stage
nem catalog build registry.example/nem/catalog:v2-staging --missing --push   # 2. archives against staging
nem catalog test registry.example/nem/catalog:v2-staging --package jq   # 3. prove it, on each platform's runner
oras tag registry.example/nem/catalog:v2-staging v2                     # 4. promote by retagging
```

Promotion retags the same index, so what was tested is what ships. The
release tag written in step 1 is the permanent name for that state.

## Telling consumers

```shell
nem catalog add corp registry.example/nem/catalog:v2           # follow the moving tag
nem catalog add corp registry.example/nem/catalog@sha256:…     # or freeze one state
```

[Access and rollout](../access-and-rollout/) covers credentials and getting
the entry onto every machine.

## Related

- [nem catalog publish](../../reference/cli/nem-catalog-publish/), [nem catalog build](../../reference/cli/nem-catalog-build/), [nem catalog fill](../../reference/cli/nem-catalog-fill/)
- [Authoring workflow](../../writing-packages/workflow/)
- [How catalogs work](../how-catalogs-work/)
