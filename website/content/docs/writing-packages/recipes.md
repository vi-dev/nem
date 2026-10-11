---
title: Frozen recipes
weight: 8
---

`pkg.yaml` describes the current line of a package. When upstream changes
how a package is fetched, installed or built, the versions before that
change keep their old recipe in a frozen sibling file.

## The rule

A frozen recipe is a file `recipe@<prefix>.yaml` next to `pkg.yaml`. The
prefix is one or more version components: `recipe@1.yaml` covers every
1.x version, `recipe@1.24.yaml` every 1.24.x version, `recipe@2.1.2.yaml`
exactly that version. A version belongs to the frozen recipe with the
longest matching prefix; a version no frozen recipe matches belongs to
`pkg.yaml`. A prerelease suffix is part of the last component, so
`recipe@2.1.yaml` covers `2.1.3-rc1` but `recipe@2.1.3.yaml` does not.

A frozen recipe is complete. It carries `platforms`, `artifact`,
`install`, `deps`, `bins`, `libs`, `env`, `build` and `test` for the
versions it covers and nothing else: no `schema`, `name`, `versions` or
`versionDiscovery`. A key it leaves out means the same as leaving it out
of `pkg.yaml`.

```text
pkgs/gpgme/
├── pkg.yaml          identity, every version, and the recipe for 2.x
└── recipe@1.yaml     the recipe for every 1.x version
```

## Freezing a recipe

1. Copy the recipe sections of `pkg.yaml` into `recipe@<prefix>.yaml`,
   naming the line you are freezing.
2. Edit `pkg.yaml` for the new line.
3. Run `nem catalog lint`. It checks each recipe against the versions it
   covers, flags a frozen file that no version resolves to, and flags a
   `pkg.yaml` that does not cover the latest version.

`nem catalog bump` adds a new version under whichever recipe covers it:
a 2.3.0 release under `pkg.yaml`, a 1.24.4 backport under
`recipe@1.yaml`. `nem catalog build --package gpgme@1.24.3` builds with
the frozen recipe. `nem catalog diff` attributes a change to the versions
whose recipe changed.

## Retiring a frozen recipe

When the last version a frozen file covers is dropped from `versions`,
delete the file. Lint reports it until you do.

## In a published catalog

A published package is one OCI manifest with a layer for `pkg.yaml` and
one layer per frozen recipe. Editing a frozen recipe changes the package
digest exactly as editing `pkg.yaml` does, so `nem sync` notices it.
