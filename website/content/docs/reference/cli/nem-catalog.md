---
title: nem catalog
weight: 3
---

Manage catalogs.

Catalogs are the ordered sources nem resolves packages from. The consumption commands edit the list in config.yaml; the maintenance commands work on a catalog's contents, whether a local directory of package manifests or a published OCI reference.

## Usage

```shell
nem catalog
```

Aliases: `cat`

## Examples

```shell
nem catalog list                                        # what is configured
nem catalog add corp registry.example/nem/catalog:v2    # add one
nem catalog lint .                                      # validate a checkout
```

## Commands

### Catalog consumption

| Command | Description |
|---------|-------------|
| [add](../nem-catalog-add/) | Add a catalog |
| [disable](../nem-catalog-disable/) | Disable configured catalogs |
| [enable](../nem-catalog-enable/) | Enable configured catalogs |
| [list](../nem-catalog-list/) | List configured catalogs |
| [remove](../nem-catalog-remove/) | Remove a catalog |
| [reorder](../nem-catalog-reorder/) | Reorder catalog precedence |
| [update](../nem-catalog-update/) | Sync oci catalogs from their remote |

### Catalog maintenance

| Command | Description |
|---------|-------------|
| [build](../nem-catalog-build/) | Build a catalog's build-from-source packages on the host platform |
| [bump](../nem-catalog-bump/) | Add newer upstream versions to package manifests |
| [diff](../nem-catalog-diff/) | Compare a base catalog's package manifests against a target catalog |
| [fill](../nem-catalog-fill/) | Download a catalog's upstream artifacts and publish them as archives |
| [fmt](../nem-catalog-fmt/) | Rewrite package manifests to canonical form |
| [lint](../nem-catalog-lint/) | Validate package manifests in a catalog |
| [mirror](../nem-catalog-mirror/) | Replicate a catalog and its archives to another registry |
| [outdated](../nem-catalog-outdated/) | Report packages whose upstream has a newer version |
| [publish](../nem-catalog-publish/) | Publish a catalog to an OCI registry |
| [test](../nem-catalog-test/) | Install packages and run their declared test steps |

## Global flags

`--color`, `--quiet`, and `--verbose` apply to every command; see [nem](../nem/).

## See also

- [nem](../nem/)
