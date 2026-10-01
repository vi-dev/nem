---
title: Dependencies and exports
weight: 6
---

`deps` pulls other packages in alongside yours; `env` puts values into the
environment of every project that uses it. A `link` dependency is the
special case: a library the binaries load at run time.

## Run dependencies

A `run` dependency is another package that must be installed alongside
this one; its `bins` join `PATH` too.

```yaml
deps:
  - lima                                  # any version
  - name: docker
    version: 28.0.0                       # an exact version
  - name: qemu
    platforms: [linux/arm64, linux/amd64] # only where it is needed
```

Without a version the dependency floats: the resolver takes the first
entry of its `versions` list and steps down only when something else in
the environment needs an older one. A pinned version is exact.

## Link dependencies

A `link` dependency is a library the package's binaries load at run time.
Its `libs` directories join the loader path, and nem plants a
`.nem-link-dependencies/<dep>` link inside the package so binaries built
against it find it from any install location.

```yaml
deps:
  - name: libgpg-error
    kind: link
    compat: "1"                           # any 1.x release
```

`compat` names the release line the package was built against. The
resolver picks the highest version on that line, and two packages that
need incompatible lines of the same library cannot share an environment.
A library package must declare `libs`, or nothing joins the loader path:

```yaml
bins: [bin]
libs: [lib]
```

## When resolution fails

The resolver never silently overrides a pin. A dependency that needs a
different version than one you pinned directly stops with a pin conflict
and the hint `Re-pin with nem use <pkg>@<version> or unuse the package
requiring it`; two link dependencies on incompatible lines stop with
`Unuse one of the conflicting packages, or re-pin <pkg> to a version they
all accept`.

## Exports

`env` puts variables into the environment of every project that uses the
package. The value is a template over `.InstallDir` and `.Version`:

```yaml
env:
  - name: JAVA_HOME
    value: "{{.InstallDir}}"
    platforms: [linux/arm64, linux/amd64]
```

Names must match `^[A-Za-z_][A-Za-z0-9_]*$`, and a
[reserved name](../../using/environment-variables/#reserved-names) fails
lint. A project's own `[env]` entries win over package exports; the full
[precedence](../../using/environment-variables/#what-wins) is on the
Environment variables page.

## Related

- [pkg.yaml: deps](../../reference/pkg-yaml/#deps), [pkg.yaml: env](../../reference/pkg-yaml/#env), [pkg.yaml: bins and libs](../../reference/pkg-yaml/#bins-and-libs)
- [Environment variables](../../using/environment-variables/)
- [NEM_HOME](../../reference/nem-home/)
