---
title: Mirroring and filling
weight: 3
---

`mirror` copies what a catalog already has; `fill` creates what it lacks.
Both skip whatever is already in place, which is what makes them safe to
schedule.

## Mirror

```shell
nem catalog mirror <src> <dst>             # both need a tag or digest
nem catalog mirror <src> <dst> --dry-run   # report the plan without writing
```

`mirror` pulls the source index and every package manifest, pushes them
unchanged to the destination tag, then walks the packages and copies every
archive the source has, version by version. Packages whose `oci` artifact
points at some other registry are left alone. A prebuilt version the
source has no archive for is skipped, not created; that is `fill`'s job.
A built-from-source version with no source archive is a failure, because
the archive is the only form that package exists in; the cure is
`nem catalog build --push` against the mirror, not `fill`. On a second run
every archive that is already present is skipped, so a re-run after no
upstream change is quick.

The summary is `Mirrored N packages, M tags`, tags being archive versions
copied. Any failure is listed as a warning, counted in the summary, and
makes the command exit 1, so a scheduled run that half-worked is visible.

## Fill

```shell
nem catalog fill <ref>                       # a tag or digest, with push access
nem catalog fill <ref> --package jq          # one package (repeatable)
nem catalog fill <ref> --dry-run             # report the plan
```

For every version and platform of every package whose artifact is a
`url` or a GitHub asset, `fill` downloads the upstream file, verifies it
against the checksum in the manifest, and publishes it as an archive in
the catalog's archive repository. Packages with `oci` artifacts are
already archives and count as `not fillable`.

Each item ends as one of:

| Outcome | Meaning |
|---|---|
| filled | No archive existed; one was published. |
| present | An archive with the right digest already existed; nothing done. |
| healed | An archive existed but did not match the manifest's checksum; it was replaced. |
| failed | The download or the push failed; the run exits 1. |

`fill` works against your own copy rather than the source because
publishing needs push access to the catalog's archive repositories.

## Order and repetition

Mirror first, fill second: mirror brings the manifests and whatever
archives exist, fill adds the missing ones. Both are idempotent, so the
maintenance loop is simply to run them again; see
[Keeping a mirror current](../keeping-current/).

## Where things end up

Both write into `<registry>/<repository>/archives/<name>` beside the
destination reference, one tag per version. Consumers find them there
without configuration, because the location derives from the catalog
reference they added. See [How catalogs work](../how-catalogs-work/).

## Scale

A fill of the full official catalog downloads every artifact of every
version once. `--package` limits a run to the packages your developers
actually use; a scheduled run can widen from there.

## Related

- [nem catalog mirror](../../reference/cli/nem-catalog-mirror/), [nem catalog fill](../../reference/cli/nem-catalog-fill/)
- [Air-gapped networks](../air-gapped/), [Keeping a mirror current](../keeping-current/)
