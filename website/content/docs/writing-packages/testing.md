---
title: Testing packages
weight: 8
---

A `test` step is a shell command that must exit 0 against an installed
version. `nem catalog test` installs the version into a throwaway
directory, runs the steps, and reports.

## Test steps

A `test` step is a shell command that must exit 0. It runs with the
package's `bins` on `PATH`, its link dependencies on the loader path, its
`env` exports applied, and `NEM_VERSION`, `NEM_OS`, `NEM_ARCH`, and
`NEM_PREFIX` set:

```yaml
test:
  - run: jq --version | grep -q "$NEM_VERSION"           # the right version is on PATH
  - run: printf '{"a":[1,2,3]}' | jq -e '.a | length == 3' >/dev/null   # and it works
```

Two steps cover most packages: one that checks the version string, one
that exercises the binary. A step can carry `platforms` to run on a
subset; lint rejects a step whose platforms overlap none of the
package's.

## Running them

```shell
nem catalog test                               # every package, latest version
nem catalog test . --package jq                # one package
nem catalog test . --package jq@1.8.2          # one version
nem catalog test . --package jq --package yq   # several
```

`nem catalog test` installs the version into a throwaway directory under
`NEM_HOME/packages`, runs the steps for the platform it is on, and removes
the install afterwards. A package with no `test` steps is install-verified
only. It continues past failures, prints a summary, and exits 1 if any
package failed.

Built packages are tested twice: `nem catalog build` runs the steps
against the fresh output before staging the archive, and `nem catalog
test` runs them against the installed archive like any other package.

## Other platforms

Tests run on the platform they are invoked on, like builds. To cover all
four, run `nem catalog test` on one runner per platform; the official
catalog's workflow uses the `ghcr.io/vi-dev/nem` image for Linux and a
macOS runner for macOS.

## Related

- [pkg.yaml: test](../../reference/pkg-yaml/#test)
- [nem catalog test](../../reference/cli/nem-catalog-test/)
- [Building from source](../build-from-source/)
