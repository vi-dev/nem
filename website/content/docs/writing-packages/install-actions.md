---
title: Install actions
weight: 5
---

Each version installs into its own directory, and the install actions say
how the downloaded artifact ends up laid out inside it. There are four:
`extract`, `copy`, `move`, and `mkdir`.

## The install directory

Each version installs into its own directory under `NEM_HOME`, and the
actions run inside it in order. When they finish, the directories in `bins`
(`bin` by default) join `PATH` and the ones in `libs` join the loader
path. Nothing outside that directory is ever touched.

## `extract`

For archives. Unpacks the artifact into the install directory; `strip`
drops leading path components, which most release tarballs need because
they wrap everything in a versioned top directory:

```yaml
install:
  - extract: {strip: 1}                   # tool-1.2.3/bin/tool becomes bin/tool
```

A compressed single file extracts to one executable in the install root,
named after the URL; follow it with a `copy` into `bin/` under the command's
name, as shown under [Single files](../artifacts-and-platforms/#single-files).

## `copy`

For a single binary, or for duplicating a file inside the install
directory. `src` is either `{{.Artifact}}`, the downloaded file itself, or a
path inside the install directory; `mode` is octal and defaults to `0644`,
so executables need it set:

```yaml
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/tool", mode: 0o755}
```

## `move`

For rearranging what an extract produced. rust-analyzer's archive puts the
binary under `server/`:

```yaml
install:
  - extract: {strip: 1}
  - mkdir: bin
  - move: {src: "server/rust-analyzer", dst: "bin/rust-analyzer"}
```

## `mkdir`

Creates a directory, usually a `bin` that the archive lacks, as above.

## Platform-specific actions

Any action takes a `platforms` list and runs only there. At least one
action must apply to each platform the package supports, or the install
fails with "no install action applies":

```yaml
install:
  - extract: {strip: 1}
    platforms: [linux/arm64, linux/amd64]
  - copy: {src: "{{.Artifact}}", dst: "bin/tool", mode: 0o755}
    platforms: [darwin/arm64, darwin/amd64]
```

## Templates and containment

`src`, `dst`, and `mkdir` paths are templates over `.Version`, `.OS`,
`.Arch`, and `.Artifact`. Every path must stay inside the install
directory; `..` that would leave it is refused.

## Two complete examples

An archive with a nested top directory:

```yaml
install:
  - extract: {strip: 1}
bins: ["bin"]
```

A single binary:

```yaml
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/tool", mode: 0o755}
```

## Related

- [pkg.yaml: install](../../reference/pkg-yaml/#install), [pkg.yaml: bins and libs](../../reference/pkg-yaml/#bins-and-libs)
- [Artifacts and platforms](../artifacts-and-platforms/)
