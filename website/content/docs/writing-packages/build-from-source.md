---
title: Building from source
weight: 7
---

A package built from source has no upstream download. Its artifact is its
own archive in the catalog, and `nem catalog build` is what produces that
archive, one platform at a time.

## The shape

A built package downloads nothing from upstream at install time. Its artifact is its
own archive in the catalog, the install action unpacks it, each version
carries the checksum of the source archive, and `build` says how to turn
that source into the archive:

```yaml
artifact:
  oci: ":{{.Version}}"

install:
  - extract: {strip: 0}

versions:
  - version: 2.1.0
    sourceSha256: "daf871488603e659b0501224cf0731ac317809b1d1701fc061cb4f6ae39a894f"

build:
  deps:
    - rust                                # build-time only
  source:
    url: "https://github.com/containers/aardvark-dns/archive/refs/tags/v{{.Version}}.tar.gz"
  output: out                             # what the steps fill; it becomes the package
  steps:
    - run: |
        set -e
        export CARGO_HOME="$PWD/.cargo"   # keep the toolchain's cache inside the build
        cargo build --release --locked
        mkdir -p "$NEM_OUTPUT/bin"
        cp target/release/aardvark-dns "$NEM_OUTPUT/bin/aardvark-dns"
```

`nem catalog bump` fills `sourceSha256` for built packages the same way it
fills `sha256` for downloaded ones.

## What a build does

`nem catalog build` runs one task per package: download and verify the
source, unpack it, install the build dependencies, run each step that
applies to the platform, normalize the output, verify it, run the
package's `test` steps against it, and stage the archive.

Steps run with `sh -c` inside the unpacked source with `NEM_OUTPUT`,
`NEM_VERSION`, `NEM_PREFIX`, `NEM_DEP_<NAME>_PREFIX` for each dependency,
and `PATH`, `CFLAGS`, `LDFLAGS`, `PKG_CONFIG_PATH`, and friends prepared so
the dependencies are found; the full list is under
[Configuration variables](../../reference/environment/#variables-in-build-and-test-steps).
A configure-and-make project installs into `$NEM_OUTPUT` through
`DESTDIR`; this is the AWS CLI's recipe, whose second step exists because
archive extraction rejects symlinks to absolute paths:

```yaml
build:
  deps:
    - python
  source:
    url: "https://github.com/aws/aws-cli/archive/refs/tags/{{.Version}}.tar.gz"
  output: out
  steps:
    - run: ./configure --prefix=/ --with-download-deps --with-install-type=portable-exe && make && make install DESTDIR="$NEM_OUTPUT"
    - run: cd "$NEM_OUTPUT/bin" && ln -sf ../lib/aws-cli/aws aws && ln -sf ../lib/aws-cli/aws_completer aws_completer
```

Start every multi-line step with `set -e` so a failing command fails the
build.

## Where the archive goes

Without `--push` the archive is staged in a temporary store and removed
when the batch ends; the run proves the recipe works and nothing more.
`--push` is what keeps it:

```shell
nem catalog build . --package aardvark-dns@2.1.0 --push   # into ./archives/aardvark-dns/
```

Against a `dir` catalog the archive lands in `archives/<name>/` beside
`pkgs/`, and `nem use local:aardvark-dns` installs from there. Against an
OCI reference it is pushed to the registry beside the catalog's index,
which is how a published catalog serves built packages.

## Choosing what to build

```shell
nem catalog build . --dry-run                     # the plan, in dependency waves
nem catalog build . --package foo@1.2.3           # one version
nem catalog build . --package foo --with-deps     # plus its missing dependencies
nem catalog build . --missing --push              # only versions whose archive is absent
```

`--missing` is what keeps re-runs cheap: it selects every version and
platform of the selected packages that has no archive yet and skips the
rest.

## One platform at a time

A build produces an archive for the machine it runs on. To build for
Linux from a Mac, run nem inside the standard image, which carries a
toolchain:

```shell
docker run --rm -v "$PWD:/work" -v nem-home:/root/.nem ghcr.io/vi-dev/nem:latest \
  catalog build . --package aardvark-dns@2.1.0 --push   # linux, the container's architecture
```

The container runs as root, so the `archives/` it writes into the bind
mount are root-owned on the host.

To cover all four platforms, run the same command on one runner per
platform. The official catalog's own workflow is the model: Linux jobs run
in the `ghcr.io/vi-dev/nem` container on `amd64` and `arm64` runners, and a
macOS job runs on a macOS runner. Each pushes its own platform's archive
into the same store, and pushes into one archive index must not overlap:
build in parallel if you like, but run the `--push` jobs one at a time.

## Normalize

After the steps, nem rewrites the output so it works from any install
directory: libtool `.la` files are dropped, `pkg-config` files are made
relocatable, Mach-O install names and rpaths are fixed, and the
`.nem-link-dependencies` links that let binaries find their link
dependencies are planted. `normalize: false` turns all of that off; only do
it when the steps arrange relocation themselves, or the built binaries
will not find their dependencies.

## Related

- [pkg.yaml: build](../../reference/pkg-yaml/#build)
- [nem catalog build](../../reference/cli/nem-catalog-build/)
- [Configuration variables](../../reference/environment/)
- [Testing packages](../testing/)
