---
title: Access and rollout
weight: 6
---

Getting a catalog onto developers' machines is a `docker login`, possibly a
`hosts:` entry, and one `nem catalog add` per machine, or a `config.yaml`
shipped ready-made.

## Credentials

nem has no credential store of its own. It reads Docker's `config.json`,
from `DOCKER_CONFIG` if set and `~/.docker` otherwise, so whatever logs
Docker in logs nem in:

```shell
docker login registry.example          # once per person
```

In CI, point `DOCKER_CONFIG` at a directory holding a `config.json` with
the registry's credentials, or run `docker login` in the job. A registry
that answers 401 makes nem print `Run docker login <host>` with the host
filled in.

## Private registries

A registry with a private CA, without TLS, or with a certificate you
cannot verify needs a `hosts:` entry in
[config.yaml](../../reference/config-yaml/):

```yaml
hosts:
  - host: registry.corp.example
    ca: /etc/nem/corp-ca.pem            # a private CA bundle
  - host: dev-registry.corp.example:5000
    plainHTTP: true                     # HTTP instead of HTTPS
```

Exactly one of `ca`, `plainHTTP`, or `insecure` per host. Loopback hosts
such as `localhost:5000` default to plain HTTP without an entry.

## What a developer runs

```shell
nem catalog add corp registry.example/nem/catalog:v2   # appended after the catalogs already configured
nem catalog reorder corp official                      # if the corporate catalog should win
nem catalog update                                     # sync it
```

`add` appends, so a catalog that should take precedence over `official`
needs the `reorder`; `nem catalog disable official` instead makes the
mirror the only source. [Catalogs](../../using/catalogs/#adding-and-ordering-catalogs)
under Using nem is the developer's view of the same commands. Everything
these commands write is
`$NEM_HOME/config.yaml`, plain YAML, so shipping a ready file through
whatever distributes dotfiles or machine images works just as well.

## Pinning

Machines that must not move add the catalog by digest instead of tag:

```shell
nem catalog add corp registry.example/nem/catalog@sha256:…   # one exact state
```

The digest of a tag comes from your registry's UI or from
`oras manifest fetch --descriptor registry.example/nem/catalog:v2`. A
pinned consumer never sees new packages until someone changes the digest,
which is the point.

## CI runners

A runner is a developer machine with no memory. Either add the catalog in
the job, before `nem sync`, or bake the
`config.yaml` into the runner image. [CI and containers](../../using/ci-and-containers/)
has the job shape; the rootless image is a good base for a runner image
that carries the configuration.

## Related

- [config.yaml](../../reference/config-yaml/)
- [Catalogs](../../using/catalogs/), [CI and containers](../../using/ci-and-containers/)
- [Configuration variables](../../reference/environment/)
