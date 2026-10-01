---
title: Version discovery
weight: 3
---

`versionDiscovery` tells nem where upstream publishes versions, so
`nem catalog outdated` can see what you lack and `nem catalog bump` can add
it with checksums. Without it, new versions are typed by hand.

## The sources

Optional; when present, exactly one source:

```yaml
versionDiscovery:
  github:                                  # tags of a GitHub repository
    repo: jqlang/jq
    filter: '^jq-\d+\.\d+(\.\d+)?$'
    prefix: "jq-"
```

```yaml
versionDiscovery:
  gitlab:                                  # tags of a GitLab repository
    repo: group/project
    filter: '^v\d+\.\d+\.\d+$'
    prefix: "v"
```

```yaml
versionDiscovery:
  git:                                     # tags of any repository over HTTP
    url: https://git.example.com/tool.git
    filter: '^\d+\.\d+\.\d+$'
```

```yaml
versionDiscovery:
  http:                                    # a page or file scanned with a regex
    url: https://curl.se/download/
    filter: 'curl-(\d+\.\d+\.\d+)\.tar\.gz'
```

```yaml
versionDiscovery:
  oci: ghcr.io/example/tool                # a repository whose tags are the versions
```

The three tag sources read tags over the git protocol, so no API token is
involved and the API rate limit does not apply.

## Filters, prefixes, and suffixes

`filter` is a regular expression matched anywhere in a tag. Anchor it with
`^` and `$` when you mean the whole tag; `v\d+\.\d+\.\d+` unanchored also
accepts `v1.2.3-rc1`. Tags that match are kept, `prefix` and `suffix` are
trimmed off, and the rest is the version. For `http`, the version is the
first capture group.

Characters that cannot appear in an OCI tag are replaced with `_`, because
a version also names the package's archive tag.

## Named groups and meta

When the filter has a named group `(?P<version>…)`, that group is the
version and every other named group is stored as `meta` on the version
entry, where artifact templates can read it as `.Meta.<name>`. The
python-build-standalone releases need a date that is not part of the
version:

```yaml
versionDiscovery:
  http:
    url: https://api.github.com/repos/astral-sh/python-build-standalone/releases/latest
    filter: '"name": "cpython-(?P<version>\d+\.\d+\.\d+)\+(?P<date>\d+)-'

artifact:
  url: "https://github.com/astral-sh/python-build-standalone/releases/download/{{.Meta.date}}/cpython-{{.Version}}+{{.Meta.date}}-…"
```

`meta` is collected only when a `version` group is present.

## Seeing and adding versions

```shell
nem catalog outdated                     # PACKAGE  CURRENT  LATEST for packages that lag
nem catalog outdated . --output json     # the same rows for scripts
nem catalog bump                         # add every missing newer version
nem catalog bump . --package jq          # one package
nem catalog bump . --package jq@1.8.1    # a specific version, even an older one
nem catalog bump . --backfill 3          # make sure the newest three exist
nem catalog bump . --dry-run             # discover and report, write nothing
```

`outdated` compares the newest entry in `versions` with the newest version
the source offers and never writes. `bump` downloads every platform's
artifact to compute its checksum, or the source archive for a built
package, and inserts the entry in version order so the list stays newest
first. It refuses to write a manifest whose versions would be out of order.

## Related

- [pkg.yaml: versionDiscovery](../../reference/pkg-yaml/#versiondiscovery), [pkg.yaml: versions](../../reference/pkg-yaml/#versions)
- [nem catalog outdated](../../reference/cli/nem-catalog-outdated/), [nem catalog bump](../../reference/cli/nem-catalog-bump/)
