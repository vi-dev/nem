---
title: nem
layout: hextra-home
---

<div>
{{< hextra/hero-container class="nem-hero"
  image="images/nem-mark.svg" imageTitle="nem" imageWidth="120" imageHeight="120" >}}
    <div>
      <h1 class="hx:font-bold">nem.</h1>
    </div>
{{< /hextra/hero-container >}}
</div>

{{< hextra/hero-badge class="hx:mt-6" >}}
  <div class="hx:w-2 hx:h-2 hx:rounded-full hx:bg-primary-400"></div>
  <span>Free, open source</span>
{{< /hextra/hero-badge >}}

<div class="hx:mt-6">
{{< hextra/hero-headline >}}
  Reproducible dev environments.
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mt-2">
{{< hextra/hero-subtitle >}}
  For you, your teams, and your agents.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mt-12">
{{< hextra/hero-button text="Get Started" link="docs/getting-started/" >}}
</div>

<div class="hx:mt-12">
{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Use nem"
    link="docs/using/"
    subtitle="For developers. Each project gets its own package versions, switched automatically as you change directories. Commit `nem.toml` and `nem.lock` and teammates, pipelines, and coding agents install exactly the same packages, same versions, same digests." >}}

  {{< hextra/feature-card
    title="Manage a catalog"
    link="docs/managing-catalogs/"
    subtitle="For operators. Mirror the official catalog or publish your own into a registry you control, air-gapped networks included. Installs are checksum-verified and never need root, fit for regulated environments." >}}

  {{< hextra/feature-card
    title="Write packages"
    link="docs/writing-packages/"
    subtitle="For maintainers. One manifest per package: where versions and artifacts come from, how they install, how they are built from source and tested." >}}
{{< /hextra/feature-grid >}}
</div>
