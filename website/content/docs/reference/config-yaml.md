---
title: config.yaml
weight: 4
---

`nem`'s global configuration, at `$NEM_HOME/config.yaml`, `~/.nem/config.yaml`
by default; see [NEM_HOME](../nem-home/).

## Example

```yaml
catalogs:
  - name: official
    type: oci
    ref: ghcr.io/vi-dev/nem-catalog:v2
  - name: dev
    type: dir
    path: /home/me/src/my-catalog

hosts:
  - host: registry.corp.example
    ca: /etc/nem/corp-ca.pem
  - host: dev-registry.corp.example:5000
    plainHTTP: true
```

## `catalogs:`

A list of configured catalogs, in lookup-precedence order — the first
catalog in the list that has a package wins. `nem catalog` commands
(`add`, `remove`, `reorder`, `enable`/`disable`) rewrite this list;
hand-editing it works too, but the next `nem catalog` command rewrites the
whole file from nem's own structure, so comments and formatting do not
survive it.

| Field | Meaning |
|---|---|
| `name` | Required, unique, `^[a-z0-9][a-z0-9._-]*$` — the handle `nem catalog` commands use, and the directory name under `$NEM_HOME/catalogs/` |
| `type` | `oci` or `dir` |
| `ref` | `oci` only, required — a registry reference: a moving tag, a frozen release tag, or a digest. Setting it on a `dir` catalog is an error |
| `path` | `dir` only, required — an absolute path to a local catalog directory. Setting it on an `oci` catalog is an error |
| `disabled` | Optional, default `false` — a disabled catalog keeps its precedence slot but is skipped by lookups |

## `hosts:`

Per-host connection settings, applied when nem talks to an OCI registry:
catalog syncs, archives, mirror, fill, and publish. They do not apply to
artifact downloads from upstream URLs, which use nem's default client, so
a `ca` entry does not cover the hosts in `artifact.url`.

| Field | Meaning |
|---|---|
| `host` | Required — exact match, including any port |
| `ca` | Absolute path to a private CA bundle (PEM) |
| `plainHTTP` | Use HTTP instead of HTTPS |
| `insecure` | Use HTTPS but skip certificate verification |

Exactly one of `ca`, `plainHTTP`, or `insecure` is required per entry. The
same host may appear more than once without error; the last entry wins. Loopback
registries (`localhost`, `127.0.0.0/8`, `::1`) default to plain HTTP unless
an entry names them explicitly.

## Loading

A missing `config.yaml` is the normal state before the first run — the
first command that needs the full config writes the built-in default (the
`official` catalog, no `hosts:`), and it never overwrites a file that's
already there. Shell completion reads the file without creating it.

`hosts:` is read leniently: a missing file yields no settings, an
unparsable file yields no settings plus a warning, and an individual entry
that fails validation is dropped with its own warning while the rest of
the list still applies. None of this fails the command. Commands that read
`catalogs:`, by contrast, fail on an invalid entry anywhere in the list;
unknown keys are ignored rather than reported.
