---
title: Environment variables
weight: 3
---

A project can set environment variables the same way it declares packages:
in `nem.toml`, under `[env]`. nem expands them itself, layers them over the
shell's originals, and restores everything when you leave the directory.

## Declaring

Environment variables live in the `[env]` table, in either the project or
the global `nem.toml`:

```toml
[env]
KUBECONFIG = '$HOME/.kube/config'
AWS_PROFILE = 'dev'
```

Names must match `^[A-Za-z_][A-Za-z0-9_]*$`. A name on the [reserved list](#reserved-names)
is accepted but dropped with a warning when the environment is composed.

Edit `nem.toml` directly to change `[env]` — unlike a `[tools]` change, no
`nem lock` is needed. The shell hook applies the edit on the next
directory change, or run `eval "$(nem env)"` to apply it immediately.

## Expansion

Values support POSIX-style parameter expansion, performed by `nem` itself at
composition time — never by the shell:

- `$VAR` and `${VAR}` expand; a variable that isn't set expands to the
  empty string.
- `$$` produces a literal `$`.
- There's no command substitution and no `~` — use `$HOME` instead.

Because `nem` does the expanding, the scripts it emits carry values that are
already expanded and safely quoted. Manifest content can never inject shell
code, even if a variable's value contains something that looks like one.

## Layering on originals

For a variable `nem` manages, `$VAR` inside an `[env]` value resolves to the
value the shell had *before* nem touched it — not `nem`'s own composed value.
That makes it safe to extend rather than replace:

```toml
[env]
FOO = '$FOO:extra'
```

This layers onto the shell's original `$FOO` every time, instead of
appending to `nem`'s own previous output — so repeated evaluations of the
hook stay idempotent rather than growing the value on every `cd`.

## What wins

When the same variable is set in more than one place, precedence runs
lowest to highest:

1. Global manifest's package exports
2. Project manifest's package exports
3. Global `nem.toml` `[env]`
4. Project `nem.toml` `[env]`

`nem status` shows which package exported a variable, or `nem.toml` for a manifest `[env]` entry.

## PATH and loader path

The composed `PATH` is built deterministically: project packages before global
ones, direct packages before their dependencies, all of that ahead of the
original `PATH` — deduplicated, so nothing appears twice.

When the resolved environment includes a library package, `nem` also composes a
platform loader-path variable the same way — `DYLD_LIBRARY_PATH` on macOS,
`LD_LIBRARY_PATH` on Linux. A project that only uses
command-line packages never gets these variables touched at all.

## Reserved names

Some names are reserved, checked case-insensitively, because `[env]`
values are eval'd straight into your shell. A reserved name is accepted
into `nem.toml`, but `nem` drops that entry at composition time with a
warning, so it never reaches your shell. The denylist, by category:

- **Shell control** the shell itself relies on: `PATH`, `PROMPT_COMMAND`,
  `BASH_ENV`, `ENV`, `IFS`, `PS1`.
- **zsh's tied arrays and specials**, which zsh keeps in sync with other
  variables: `CDPATH`, `FPATH`, `MANPATH`, `MAILPATH`, `MODULE_PATH`,
  `FIGNORE`, `PSVAR`, `WATCH`.
- **libc loader and resolver controls**: `GCONV_PATH`, `GLIBC_TUNABLES`,
  `LOCPATH`, `NLSPATH`, `GETCONF_DIR`, `HOSTALIASES`, `RESOLV_HOST_CONF`.
- **Reserved prefixes**: `NEM_*`, `LD_*`, `DYLD_*` — the one exception is
  `nem`'s own composed loader variable described above, which `nem` writes
  directly rather than through a package or manifest export.
- **Reserved suffix**: `*_SET`, which collides with the hook's own
  save/restore bookkeeping (see
  [Shell integration](../shell-integration/)).

## Related

- [nem.toml](../../reference/nem-toml/)
- [nem status](../../reference/cli/nem-status/)
- [Shell integration](../shell-integration/)
