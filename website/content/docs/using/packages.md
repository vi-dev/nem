---
title: Packages
weight: 2
aliases:
  - /docs/guides/managing-environments/
---

`nem use` writes two files: the version you asked for into `nem.toml` and
the exact resolved closure into `nem.lock`. Every other package command,
`unuse`, `update`, `lock`, and `sync`, is a different way of moving those
two files and the installs they describe.

## Declaring packages

```shell
nem use [<catalog>:]<pkg>[@<version>]...
```

`nem use` resolves each package, installs it, and records the result in two
files: the version in `nem.toml`, the exact resolved closure — with
SHA-256 digests — and transitive dependencies in `nem.lock`.

If `@<version>` is not specified, `nem` takes the first entry of the
package's `versions` list, which catalogs keep newest first, and steps down
the list only when another package requires an older or compat-constrained
version.
If `<catalog>:` is not specified, `nem` searches for the package in all catalogs, in order, until it finds a match.

`nem unuse <pkg>...` removes declarations from `nem.toml` and re-resolves
`nem.lock`. Dropping a pin can move a shared dependency, so it then installs
what the new lock needs and relinks the packages that stay. It never deletes
installed packages — other projects on the same machine may still be using
them.

## Updating packages

```shell
nem update [<pkg>...]
```

`nem update` (alias `up`) re-resolves declared packages — all, or just the
ones named — to their catalog's latest, exactly as if you re-ran `nem use`
for each. `-g` targets the global manifest; `--dry-run` reports the plan
without writing anything. A catalog that was never synced is synced first;
one last synced more than a week ago earns a warning to run
`nem catalog update`. Each
package settles on the first entry its dependents allow; a pick below a
declared version aborts the whole update, and conflicting declarations fail
it with the clashing requirements.

## Project and global scope

By default `nem` works at project scope, where declared packages and environment are scoped to the project directory tree.
`nem` finds a project's manifest by walking up from the current directory to the nearest `nem.toml`. 

`nem` also supports a global scope, where declared packages and environment are available everywhere.
The global `nem.toml` lives at `$NEM_HOME/nem.toml`, and every command that touches a manifest 
addresses it by adding `--global` / `-g` flag.

```shell
nem use -g go@1.27.0   # declare a package globally, not just for this project
```

When both scopes declare the same package or environment variable, the project's declaration wins — see
[the full precedence order](../environment-variables/#what-wins).

## Hand-editing nem.toml

`nem.toml` is meant to be edited by hand too, not only through `nem use`.
After editing it, run `nem lock` to regenerate `nem.lock` and install:

```shell
nem lock
```

Every package must be pinned to the exact version string as it appears in the
catalog — there's no `"latest"` keyword and no version ranges in
`nem.toml`; a version `nem` can't find in the catalog is a lock error. If a
dependency needs a different version than a package you pinned directly,
that's a pin-conflict error, not a silent override. `nem` never installs a
version that contradicts what the manifest declares.

## Sharing

Commit both `nem.toml` and `nem.lock`. Teammates and CI run:

```shell
nem sync              # install what nem.lock pins
```

`nem sync` installs exactly what the lockfile pins and warns if `nem.toml`
has drifted from `nem.lock`, for example a declared package the lock doesn't
cover yet. It reads package manifests from the local catalog store, syncing
a store that was never synced first; a stale store needs `nem catalog update`.

```
WARN nem.toml declares tree@2.3.2, which nem.lock does not cover — run `nem lock`
```

## Inspecting

| Command | Shows |
|---------|-------|
| [`nem status`](../../reference/cli/nem-status/) (`-g` for the global scope) | Declared packages and composed environment variables |
| [`nem which <command>...`](../../reference/cli/nem-which/) | Where a command resolves in the composed environment |
| [`nem info [<catalog>:]<pkg>`](../../reference/cli/nem-info/) | A package's details and available versions |
| [`nem search [query]`](../../reference/cli/nem-search/) | Catalog packages matching a query, omit `query` to list every package |

## Related

- [nem use](../../reference/cli/nem-use/), [nem update](../../reference/cli/nem-update/), [nem lock](../../reference/cli/nem-lock/), [nem sync](../../reference/cli/nem-sync/)
- [nem.toml](../../reference/nem-toml/)
- [nem.lock](../../reference/nem-lock/)
