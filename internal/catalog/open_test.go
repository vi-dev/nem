package catalog

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/testx"
)

func TestOpenBuildsEntriesInOrder(t *testing.T) {
	h := testx.Home(t)
	cfg := &config.Config{Catalogs: []config.CatalogEntry{
		{Name: "dev", Type: "dir", Path: t.TempDir()},
		{Name: "official", Type: "oci", Ref: config.OfficialRef},
	}}
	set, err := Open(cfg, h)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entries := set.Entries()
	if len(entries) != 2 || entries[0].Name != "dev" || entries[1].Name != "official" {
		t.Fatalf("Open: %+v", entries)
	}
	if entries[0].Ref != "" {
		t.Fatalf("dir entry must have no ref, got %q", entries[0].Ref)
	}
	if entries[1].Ref != config.OfficialRef {
		t.Fatalf("oci entry ref: %q", entries[1].Ref)
	}
	if _, ok := entries[0].Catalog.(*Dir); !ok {
		t.Fatal("dev should be a Dir source")
	}
	src, ok := entries[1].Catalog.(*OCI)
	if !ok {
		t.Fatalf("official should be an OCI source, got %T", entries[1].Catalog)
	}
	if _, err := src.PackageNames(context.Background()); !errors.Is(err, ocix.ErrNotSynced) {
		t.Fatalf("want ErrNotSynced from the unopened store, got %v", err)
	}
}

func TestOpenSkipsDisabled(t *testing.T) {
	cfg := &config.Config{Catalogs: []config.CatalogEntry{
		{Name: "a", Type: "dir", Path: "/tmp/a"},
		{Name: "b", Type: "dir", Path: "/tmp/b", Disabled: true},
		{Name: "c", Type: "dir", Path: "/tmp/c"},
	}}
	set, err := Open(cfg, testx.Home(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var got []string
	for _, e := range set.Entries() {
		got = append(got, e.Name)
	}
	if !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("Open should skip disabled and preserve order, got %v", got)
	}
}
