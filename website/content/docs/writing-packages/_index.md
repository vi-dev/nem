---
title: Writing nem packages
weight: 4
sidebar:
  open: true
---

A catalog is a directory of package manifests, one `pkg.yaml` per package.
These guides cover writing a manifest, letting nem find versions and
artifacts for it, building and testing it, and the loop that keeps a
catalog healthy.

{{< cards >}}
  {{< card link="quickstart/" title="Quickstart" subtitle="A catalog directory, one manifest, and a project that uses it." >}}
  {{< card link="anatomy/" title="Anatomy of a package" subtitle="What every part of a manifest does." >}}
  {{< card link="version-discovery/" title="Version discovery" subtitle="Let nem find new upstream versions and add them with checksums." >}}
  {{< card link="artifacts-and-platforms/" title="Artifacts and platforms" subtitle="URL templates, the four platforms, and per-platform checksums." >}}
  {{< card link="install-actions/" title="Install actions" subtitle="Extract, copy, move, and mkdir, and when to use each." >}}
  {{< card link="dependencies-and-exports/" title="Dependencies and exports" subtitle="Run and link dependencies, libraries, and exported variables." >}}
  {{< card link="build-from-source/" title="Building from source" subtitle="Recipes, the build environment, and covering every platform." >}}
  {{< card link="testing/" title="Testing packages" subtitle="Test steps and nem catalog test." >}}
  {{< card link="workflow/" title="Authoring workflow" subtitle="Lint, fmt, test, build, diff, and keeping packages current." >}}
{{< /cards >}}
