package catalog

import (
	"context"
	"errors"
	"testing"

	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
)

const pulledPkgYAML = `schema: 2
name: xtool
description: Target fixture
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - version: v1.0.0
`

const pulledMismatchYAML = `schema: 2
name: other
description: manifest name does not match the requested alias
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - v1.0.0
`

const pulledInvalidYAML = `schema: 2
name: badpkg
description: parses but fails validation
artifact:
  oci: ":{{.Version}}"
versions:
  - v1.0.0
`

const pulledZebraYAML = `schema: 2
name: zebra
description: Last alphabetically
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - v2.0.0
`

const pulledAppleYAML = `schema: 2
name: apple
description: First alphabetically
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - v1.0.0
`

const pulledUntitledYAML = `schema: 2
name: untitled
description: indexed without a title annotation, must be skipped
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - v9.9.9
`

func newPulledStore(t *testing.T, entries []ocixtest.FakeEntry) *ocix.Store {
	t.Helper()
	mem := memory.New()
	ocixtest.PushFakeCatalog(t, mem, entries, ocix.SchemaVersion)
	store, err := ocix.OpenStoreInMemory(context.Background(), mem, "v2", nil)
	if err != nil {
		t.Fatalf("OpenStoreInMemory: %v", err)
	}
	return store
}

func TestOCIServesPulledCatalog(t *testing.T) {
	ctx := context.Background()
	store := newPulledStore(t, []ocixtest.FakeEntry{
		{Name: "xtool", Description: "Target fixture", Latest: "v1.0.0", YAML: []byte(pulledPkgYAML)},
	})
	src := NewOCI("ghcr.io/org/cat:v2", store)

	names, err := src.PackageNames(ctx)
	if err != nil {
		t.Fatalf("PackageNames: %v", err)
	}
	if len(names) != 1 || names[0] != "xtool" {
		t.Fatalf("PackageNames = %v, want [xtool]", names)
	}

	pkg, dig, err := src.Package(ctx, "xtool")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pkg.Name != "xtool" || dig == "" {
		t.Fatalf("Load = %+v, digest %q", pkg, dig)
	}

	sums, err := src.Summaries(ctx)
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(sums) != 1 || sums[0].Name != "xtool" || sums[0].Latest != "v1.0.0" {
		t.Fatalf("Summaries = %+v", sums)
	}

	vers, err := src.Versions(ctx, "xtool")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(vers) != 1 || vers[0] != "v1.0.0" {
		t.Fatalf("Versions = %v, want [v1.0.0]", vers)
	}

	if _, _, err := src.Package(ctx, "ghost"); err == nil {
		t.Fatal("Load of an absent package succeeded, want a not-found error")
	} else if _, ok := errors.AsType[*PackageNotFoundError](err); !ok {
		t.Fatalf("Load error = %v, want *PackageNotFoundError", err)
	}
}

func TestOCISummariesReadsIndexAnnotations(t *testing.T) {
	ctx := context.Background()
	store := newPulledStore(t, []ocixtest.FakeEntry{
		{Name: "zebra", Description: "Last alphabetically", Latest: "v2.0.0", YAML: []byte(pulledZebraYAML)},
		{Name: "apple", Description: "First alphabetically", Latest: "v1.0.0", YAML: []byte(pulledAppleYAML)},
		{Name: "", Description: "indexed without a title annotation, must be skipped", Latest: "v9.9.9", YAML: []byte(pulledUntitledYAML)},
	})
	src := NewOCI("ghcr.io/org/cat:v2", store)

	sums, err := src.Summaries(ctx)
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(sums) != 2 {
		t.Fatalf("Summaries = %+v, want 2 entries (untitled manifest skipped)", sums)
	}

	if sums[0] != (Summary{Name: "zebra", Description: "Last alphabetically", Latest: "v2.0.0"}) {
		t.Fatalf("sums[0] = %+v", sums[0])
	}
	if sums[1] != (Summary{Name: "apple", Description: "First alphabetically", Latest: "v1.0.0"}) {
		t.Fatalf("sums[1] = %+v", sums[1])
	}
}

func TestOCIRejectsNameMismatch(t *testing.T) {
	store := newPulledStore(t, []ocixtest.FakeEntry{

		{Name: "alias", Description: "Target fixture", Latest: "v1.0.0", YAML: []byte(pulledMismatchYAML)},
	})
	src := NewOCI("ghcr.io/org/cat:v2", store)
	_, _, err := src.Package(context.Background(), "alias")
	if err == nil {
		t.Fatal("Load accepted a manifest whose name disagrees with the index title")
	}
	const want = `catalog ghcr.io/org/cat:v2: package alias: manifest declares name "other", want "alias"`
	if got := err.Error(); got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestOCILoadValidateFailure(t *testing.T) {
	store := newPulledStore(t, []ocixtest.FakeEntry{
		{Name: "badpkg", Description: "invalid", Latest: "v1.0.0", YAML: []byte(pulledInvalidYAML)},
	})
	src := NewOCI("ghcr.io/org/cat:v2", store)
	_, _, err := src.Package(context.Background(), "badpkg")
	if err == nil {
		t.Fatal("Load accepted a manifest that fails Validate")
	}
	const wantPrefix = "catalog ghcr.io/org/cat:v2: package badpkg:"
	if got := err.Error(); len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("error = %q, want it to start with %q", got, wantPrefix)
	}
}
