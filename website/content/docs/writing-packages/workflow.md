---
title: Authoring workflow
weight: 9
---

A catalog stays healthy with the same few commands in two loops: lint, fmt,
test, and build for a new package; outdated, bump, test, and build for an
existing one. `diff` shows what a checkout changes against what is live.

## A new package

1. Write `pkgs/<name>/pkg.yaml`, starting from the
   [Quickstart](../quickstart/) shape.
2. `nem catalog lint .` until it is clean.
3. `nem catalog fmt .` to settle the layout.
4. `nem catalog test . --package <name>` on each platform you can reach.
5. For a built package, `nem catalog build . --package <name> --push` on
   each platform.

```shell
nem catalog lint .                             # no findings
nem catalog fmt .                              # stable layout, comments kept
nem catalog test . --package tool              # installs and runs test steps
nem catalog build . --package tool --push      # built packages only
```

## Keeping packages current

```shell
nem catalog outdated                           # which packages lag upstream
nem catalog bump . --package tool              # add the newer versions with checksums
nem catalog test . --package tool              # the new version installs and works
nem catalog build . --missing --push           # built packages: archives for the new versions
```

`bump --dry-run` reports what would be added without downloading or
writing, which is the right first step in automation.

## Before publishing

`nem catalog diff` compares the catalog you are working on with the one
that is live:

```shell
nem catalog diff . registry.example/nem/catalog:v2                  # what this checkout changes
nem catalog diff . registry.example/nem/catalog:v2 --output json    # only changed rows, for scripts
```

The base, your checkout, is linted first. Each row is `new`, `updated`,
or `removed` relative to the target, and the JSON form carries `name`,
`status`, `build`, `base`, `target`, and `diff`, the versions the base
adds. Publishing the result is covered under
[Publishing your own catalog](../../managing-catalogs/publish-your-own/).

## A catalog repository's CI

The shape that works: lint and fmt on every change; `diff --output json`
against the published catalog to find the packages that changed; test and
build only those, one runner per platform; and publish from the default
branch once the checks pass. Caching `$NEM_HOME/packages` between runs
saves re-downloading dependencies, if the runs are long enough to notice.

## Related

- [nem catalog lint](../../reference/cli/nem-catalog-lint/), [fmt](../../reference/cli/nem-catalog-fmt/), [outdated](../../reference/cli/nem-catalog-outdated/), [bump](../../reference/cli/nem-catalog-bump/), [test](../../reference/cli/nem-catalog-test/), [build](../../reference/cli/nem-catalog-build/), [diff](../../reference/cli/nem-catalog-diff/)
- [pkg.yaml](../../reference/pkg-yaml/)
