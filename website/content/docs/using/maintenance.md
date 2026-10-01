---
title: Maintenance
weight: 8
---

Everything nem installs lives under `NEM_HOME`, so keeping a machine tidy is
three commands: `nem clean` to reclaim space, `nem self update` to update
nem, and `nem deactivate` plus one `rm` to remove it.

## Reclaiming disk space

Every package nem installs stays under `NEM_HOME` until you remove it.
A bare `nem clean` removes only provable garbage — leaked build staging,
leaked downloads, partial installs, leftover test installs — and never
prompts, so it is safe to run unattended:

```shell
nem clean                    # provable garbage only, no prompt
nem clean --dry-run          # print the plan without deleting anything
nem clean --unused 30d       # also versions nem has not resolved in 30 days
nem clean --all              # every installed package version (asks first)
```

`--unused` measures the last time nem itself used a version — `nem env`
on a directory change, `nem exec`, an install, or `nem catalog build` — not
the last time a shell ran the binary. A `nem sync` that installs nothing
refreshes nothing. A shell that has not changed
directories in a while can still have a version on its `PATH` that
`--unused` would evict. Whatever `nem clean` removes, a project gets back
with `nem sync`.

`--grace` (default `1h`) leaves recently touched reclaimable paths alone,
and `--yes` skips the confirmation that `--unused` and `--all` ask for.
The [NEM_HOME reference](../../reference/nem-home/#reclaiming-space)
lists what counts as garbage tier by tier.

## Updating nem

nem updates itself in place:

```shell
nem self update                     # latest build on the current channel
nem self update --check             # report whether an update exists
nem self update --version v0.3.0    # a specific release
nem self update --version unstable  # the rolling build from main
```

A stable build updates to the latest release; an unstable build follows
`main`. `--version stable` switches an unstable build back to releases.
The download is verified against its checksum before the running binary
is replaced. The [install script](../../getting-started/#installation)
with `NEM_VERSION` is the alternative when a build did not come from a
release.

## Uninstalling

Everything nem installs lives under `NEM_HOME` (default `~/.nem`). Outside
it, nem writes only the hook block in your shell's startup file, the binary
itself on `nem self update`, and a project's `nem.toml` and `nem.lock`.

```shell
nem deactivate               # remove the hook block from .zshrc or .bashrc
exec $SHELL                  # restart the shell without the hook
rm -rf ~/.nem                # every package, catalog, and the global manifest
rm ~/.local/bin/nem          # the binary (or wherever NEM_INSTALL_DIR put it)
```

Projects keep their `nem.toml` and `nem.lock`; they are plain files in the
repository and harmless without nem. `NEM_INSTALL_DIR` and the other
install-time variables are listed under
[Configuration variables](../../reference/environment/#install-script-variables).

## Related

- [nem clean](../../reference/cli/nem-clean/)
- [nem self update](../../reference/cli/nem-self-update/)
- [nem deactivate](../../reference/cli/nem-deactivate/)
- [NEM_HOME](../../reference/nem-home/)
