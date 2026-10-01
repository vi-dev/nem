---
title: Shell integration
weight: 4
aliases:
  - /docs/guides/shell-integration/
---

The composed environment reaches your shell in one of two ways: a hook that
`nem activate` installs for interactive use, or `nem exec` and `nem env` for
scripts and CI. Neither uses shims; a package is on `PATH` only while it
applies.

There are two ways to bring the composed environment into your work: a
shell hook for interactive, day-to-day use, and `nem exec` / `nem env` for
scripts and CI. Neither uses shims — a package is only on your `PATH` for as
long as it applies.

## Installing the hook

```shell
nem activate      # zsh or bash
exec $SHELL
```

`nem activate [zsh|bash]` appends a managed block to `.zshrc` or `.bashrc`,
between `# >>> nem >>>` and `# <<< nem <<<` markers. `nem deactivate` removes
it. Restart the shell (`exec $SHELL`) so the block takes effect.

Use `nem activate --print` to print the block to stdout instead of
installing it — also the default whenever stdout isn't a terminal, so
`eval "$(nem activate --print)"` works from another program's init script.

## What the hook does

The installed block:

- Saves the shell's original `PATH` once, before nem starts changing it;
- Registers a hook that re-evaluates `eval "$(nem env --shell <shell>)"`:
  zsh's `chpwd` runs it on every `cd`, bash's `PROMPT_COMMAND` before every
  prompt;
- Wraps the `nem` command so `use`, `unuse`, `lock`, and `sync` re-apply the
  environment immediately, without waiting for the next directory change;
- Sources `nem`'s shell completions.

`nem update` is not wrapped: the shell picks up an update on the next
directory change, or run `eval "$(nem env)"` to apply it at once.

## Leaving restores everything

The script that `nem env` prints tracks every variable it manages, together
with the value — or absence — it had before nem touched it. Leaving a
project, or deactivating nem altogether, restores each one exactly:
variables nem changed go back to their saved value, and variables nem set
that didn't exist before are unset. `PATH` entries are deduplicated on
the way in, so re-evaluating the hook repeatedly never grows `PATH`.

## Scripts and CI

Outside an interactive shell, two commands give you the same composed
environment without installing anything:

```shell
nem exec -- <cmd> [args...]   # run one command in the composed environment
nem env --shell bash          # print the script to eval instead
```

[`nem exec`](../../reference/cli/nem-exec/) runs the command as a child process and exits with its exact
status, which makes it the one to reach for in scripts and CI steps. [`nem env`](../../reference/cli/nem-env/) prints the shell script that applies the environment; eval it when you
need the environment in the current shell rather than a subprocess.

{{< callout type="info" >}}
The hook and `nem env` support `zsh` and `bash`. `fish` is not supported yet.
{{< /callout >}}

## Related

- [nem activate](../../reference/cli/nem-activate/), [nem deactivate](../../reference/cli/nem-deactivate/)
- [nem env](../../reference/cli/nem-env/), [nem exec](../../reference/cli/nem-exec/)
- [Environment variables](../environment-variables/)
