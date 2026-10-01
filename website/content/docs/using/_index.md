---
title: Using nem
weight: 2
sidebar:
  open: true
---

Everything a developer needs to consume packages with nem: what the two
project files mean, how packages and environment variables are declared,
how the shell picks them up, and how the same environment reaches CI and
coding agents.

{{< cards >}}
  {{< card link="how-it-works/" title="How nem works" subtitle="Manifest, lockfile, composed environment, catalogs, NEM_HOME." >}}
  {{< card link="packages/" title="Packages" subtitle="Declare, update, share, and inspect a project's packages." >}}
  {{< card link="environment-variables/" title="Environment variables" subtitle="The [env] table, expansion, precedence, and reserved names." >}}
  {{< card link="shell-integration/" title="Shell integration" subtitle="The hook for interactive shells, exec and env for scripts." >}}
  {{< card link="catalogs/" title="Catalogs" subtitle="Add, order, pin, and authenticate to catalogs." >}}
  {{< card link="ci-and-containers/" title="CI and containers" subtitle="Images, the sync-then-exec pattern, and caching." >}}
  {{< card link="agents/" title="Coding agents" subtitle="Give an agent the same environment as a teammate." >}}
  {{< card link="maintenance/" title="Maintenance" subtitle="Reclaim disk space, update nem, uninstall." >}}
{{< /cards >}}
