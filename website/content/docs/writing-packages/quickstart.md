---
title: Quickstart
weight: 1
---

You end with a catalog directory, one manifest for a prebuilt binary, and
a project that installs it from your catalog.

{{% steps %}}

### Create the catalog directory

A catalog is a directory with one manifest per package:

```shell
mkdir -p my-catalog/pkgs/jq      # pkgs/<name>/pkg.yaml, one directory per package
```

Built-from-source packages later add an `archives/` directory beside
`pkgs/`; a catalog of prebuilt binaries never needs it.

### Write the manifest

`my-catalog/pkgs/jq/pkg.yaml`, for a package that ships one static binary
per platform:

```yaml
schema: 2                                   # always 2
name: jq                                    # must match the directory name
description: Command-line JSON processor
homepage: https://jqlang.org
license: MIT

artifact:                                   # where each version's file comes from
  url: "https://github.com/jqlang/jq/releases/download/jq-{{.Version}}/jq-{{replace .OS \"darwin\" \"macos\"}}-{{.Arch}}"

install:                                    # how it lands in the install directory
  - copy: {src: "{{.Artifact}}", dst: "bin/jq", mode: 0o755}

versions:                                   # newest first, one checksum per platform
  - version: 1.8.2
    sha256:
      darwin/arm64: "2d75340ba57a4b4b4c8708a21c2dc8e958a48aaa8bba13b27f77f6e4c0eca07e"
      darwin/amd64: "e94b266e3c26690550006abe63152b782280f4e14374accdf04cbde844f00bc0"
      linux/arm64: "8b85c817833814ddca00a144c33705546355afccf0cf39b188f3cdb48b852309"
      linux/amd64: "b1c22172dd303f3be49e935aa56aa48a8b7a46e0bc838b4997d3bb451495870f"
```

The `url` is a template: `.Version`, `.OS`, and `.Arch` are filled in per
platform, and `replace` renames `darwin` to the `macos` that jq's release
files use. `bin/` is on `PATH` by default, so the binary goes there.

### Lint it

```shell
nem catalog lint my-catalog              # OK Catalog is clean: no findings
```

Lint parses the manifest strictly, checks every rule the
[pkg.yaml reference](../../reference/pkg-yaml/#what-lint-enforces) lists,
and prints one warning per finding.

### Add the catalog

```shell
nem catalog add local ./my-catalog       # OK Added catalog local
nem catalog list                         # local, type dir, after official
```

A path makes it a `dir` catalog. It is read straight from disk, so every
edit shows up at once; there is nothing to sync.

### Use the package

```shell
mkdir demo && cd demo
nem use local:jq                         # OK Installed jq 1.8.2
nem which jq                             # …/packages/jq/1.8.2/bin/jq
```

The `local:` prefix pins the package to your catalog, and `nem.toml`
records it that way. On a fresh machine the first `nem use` also syncs the
official catalog, which takes a moment.

### Test it

```shell
nem catalog test ../my-catalog --package jq   # … OK Installed jq 1.8.2 (declares no tests)
```

Without `test` steps, `nem catalog test` only proves the version installs.
Add one and run it again:

```yaml
test:
  - run: jq --version | grep -q "$NEM_VERSION"
```

```shell
nem catalog test ../my-catalog --package jq   # OK Tested jq 1.8.2 (1 step)
```

{{% /steps %}}

## Where next

- [Anatomy of a package](../anatomy/) explains every key you just wrote.
- [Authoring workflow](../workflow/) is the loop for adding packages and
  keeping them current.
- [Publishing your own catalog](../../managing-catalogs/publish-your-own/)
  turns the directory into an OCI catalog other machines can add.

## Related

- [pkg.yaml](../../reference/pkg-yaml/)
- [nem catalog lint](../../reference/cli/nem-catalog-lint/), [nem catalog test](../../reference/cli/nem-catalog-test/)
- [Catalogs](../../using/catalogs/)
