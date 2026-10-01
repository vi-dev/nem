---
title: nem.toml
weight: 2
---

The manifest declaring a directory's packages and environment variables. Meant
to be committed. `nem` finds it by walking up from the current directory to
the nearest `nem.toml`, and `nem use` creates one in the current directory
when there is none. The global twin lives at `$NEM_HOME/nem.toml`; inside a
project, its packages and `[env]` apply underneath the project's.

## Example

```toml
[tools]
'dev:kubectl' = '1.36.3'
go = '1.27.0'

[env]
KUBECONFIG = '$HOME/.kube/config'
```

This is the same shape `nem use` writes: keys sorted, bare version strings,
single quotes. `go` has no catalog prefix, so it resolves from the first
catalog (in configuration order) that carries it. `'dev:kubectl'` pins the
package to the catalog named `dev` — TOML requires quoting a prefixed key.

## `[tools]`

- A key is `[<catalog>:]<name>`. Without a prefix, lookup walks the
  configured catalogs first-match-wins, in configuration order, skipping
  disabled ones. With a prefix, lookup uses that catalog only; a prefix
  naming a disabled or unknown catalog is an error.
- A package name appears at most once, prefixed or not — so `nem unuse
  <pkg>` is never ambiguous about which entry to remove.
- Values are the exact version string as it appears in the catalog. There's
  no `"latest"` keyword and no ranges, and no empty value: `nem lock`
  rejects a package declared without a version and `nem sync` warns about
  it.
- Parsing is strict: an unknown table or key — a typo like `[tool]` — is an
  error, not a silent skip.
- Only direct intent lives here. The resolved closure, including
  dependencies, lives in [nem.lock](../nem-lock/).

## `[env]`

Names must match `^[A-Za-z_][A-Za-z0-9_]*$`; one that does not is a parse
error. A name on nem's
[reserved list](../../using/environment-variables/#reserved-names) is
accepted here but dropped with a warning when the environment is composed.
A value supports `$VAR`/`${VAR}` expansion (unset expands to empty) and `$$`
for a literal `$`, performed by nem itself, not the shell. See
[Environment variables](../../using/environment-variables/) for expansion
and precedence.

## How versions resolve

`nem use`, `nem unuse`, `nem update`, and `nem lock` re-resolve the whole
manifest every time — there's no sticky state carried over from the last
resolution. A `nem use` or `nem update` without a version takes the first
entry of the package's catalog `versions` list, which catalogs keep newest
first. Across platforms and dependents, one version per package wins: the
first entry of that list that satisfies every requirement, stepping further
down only when a dependency needs an older or `compat`-constrained one.
Requirements that no single entry satisfies fail to resolve, and a package
you pinned directly is never silently overridden by a dependency that wants
something else — that's a pin-conflict error instead. `nem sync` never
resolves; stability between manifest changes comes from `nem.lock`, not
from the resolver remembering anything.
