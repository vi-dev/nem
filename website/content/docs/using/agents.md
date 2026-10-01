---
title: Coding agents
weight: 7
---

A coding agent needs nothing from a repository that a teammate does not:
`nem.toml` and `nem.lock`. Because agents run in non-interactive shells,
they use the same `sync` then `exec` pattern as CI.

## What the repository needs

Nothing agent-specific. `nem.toml` says what the project wants and
`nem.lock` pins the exact versions and digests; both are committed. An
agent that clones the repository has everything it needs to reproduce the
environment.

## What the agent runs

Agents usually run commands in a non-interactive shell with no hook
installed, so the pattern is the [CI one](../ci-and-containers/): install
once, then run every step in the composed environment.

```shell
nem catalog update                # once: fill the catalog store on a fresh machine
nem sync                          # install what nem.lock pins
nem exec -- go test ./...         # run a step in the composed environment
nem exec -- kubectl get pods      # any command, same environment
nem which kubectl                 # which binary a command resolves to
```

`nem exec` exits with the command's own status, so an agent reads success
and failure exactly as it would without `nem`.

## Dev containers

The rootless image is built to be a base for dev containers: it runs as
user `nem` (uid 1000) with `NEM_HOME` at `/home/nem/.nem` and carries no
build toolchain, and no `git`, `curl`, or `sudo`; add whatever else your
workflow needs on top. Its entrypoint is `nem`, so a dev container overrides
the command and installs the project's packages once the container is up:

```json
{
  "image": "ghcr.io/vi-dev/nem:rootless",
  "overrideCommand": true,
  "postCreateCommand": "nem catalog update && nem sync"
}
```

## Telling the agent

Most agents read an instructions file at the repository root, such as
`AGENTS.md` or `CLAUDE.md`. One short section is enough:

```markdown
## Tooling

This repository pins its packages with nem. Run `nem catalog update` and
then `nem sync` before anything else, and run every command through
`nem exec -- <command>` so it sees the pinned versions.
```

## Related

- [CI and containers](../ci-and-containers/)
- [nem sync](../../reference/cli/nem-sync/), [nem exec](../../reference/cli/nem-exec/), [nem which](../../reference/cli/nem-which/)
