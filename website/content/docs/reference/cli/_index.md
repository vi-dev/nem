---
title: Commands
weight: 1
---

Every `nem` command, grouped as in `nem --help`. Every command also accepts `--color`, `--quiet`, and `--verbose`; see [nem](nem/).

### Environment

| Command | Description |
|---------|-------------|
| [nem exec](nem-exec/) | Run a command in the composed environment |
| [nem lock](nem-lock/) | Regenerate the lockfile from nem.toml and install |
| [nem status](nem-status/) | Show declared packages and composed environment variables |
| [nem sync](nem-sync/) | Install locked packages missing on this machine |
| [nem unuse](nem-unuse/) | Remove declared packages |
| [nem update](nem-update/) | Update declared packages to their latest versions |
| [nem use](nem-use/) | Declare and install packages |
| [nem which](nem-which/) | Show where a command resolves in the composed environment |

### Discovery

| Command | Description |
|---------|-------------|
| [nem info](nem-info/) | Show a package's details and available versions |
| [nem search](nem-search/) | Search catalogs for packages |

### Catalogs

| Command | Description |
|---------|-------------|
| [nem catalog](nem-catalog/) | Manage catalogs |

### Shell integration

| Command | Description |
|---------|-------------|
| [nem activate](nem-activate/) | Activate nem for the current shell |
| [nem deactivate](nem-deactivate/) | Deactivate nem for the current shell |
| [nem env](nem-env/) | Print the shell script that applies the composed environment |

### Maintenance

| Command | Description |
|---------|-------------|
| [nem clean](nem-clean/) | Reclaim disk space in NEM_HOME |
| [nem self](nem-self/) | Manage this nem installation |

### Catalog consumption

| Command | Description |
|---------|-------------|
| [nem catalog add](nem-catalog-add/) | Add a catalog |
| [nem catalog disable](nem-catalog-disable/) | Disable configured catalogs |
| [nem catalog enable](nem-catalog-enable/) | Enable configured catalogs |
| [nem catalog list](nem-catalog-list/) | List configured catalogs |
| [nem catalog remove](nem-catalog-remove/) | Remove a catalog |
| [nem catalog reorder](nem-catalog-reorder/) | Reorder catalog precedence |
| [nem catalog update](nem-catalog-update/) | Sync oci catalogs from their remote |

### Catalog maintenance

| Command | Description |
|---------|-------------|
| [nem catalog build](nem-catalog-build/) | Build a catalog's build-from-source packages on the host platform |
| [nem catalog bump](nem-catalog-bump/) | Add newer upstream versions to package manifests |
| [nem catalog diff](nem-catalog-diff/) | Compare a base catalog's package manifests against a target catalog |
| [nem catalog fill](nem-catalog-fill/) | Download a catalog's upstream artifacts and publish them as archives |
| [nem catalog fmt](nem-catalog-fmt/) | Rewrite package manifests to canonical form |
| [nem catalog lint](nem-catalog-lint/) | Validate package manifests in a catalog |
| [nem catalog mirror](nem-catalog-mirror/) | Replicate a catalog and its archives to another registry |
| [nem catalog outdated](nem-catalog-outdated/) | Report packages whose upstream has a newer version |
| [nem catalog publish](nem-catalog-publish/) | Publish a catalog to an OCI registry |
| [nem catalog test](nem-catalog-test/) | Install packages and run their declared test steps |

### Other commands

| Command | Description |
|---------|-------------|
| [nem completion](nem-completion/) | Generate the autocompletion script for the specified shell |
| [nem version](nem-version/) | Print the version of nem |
