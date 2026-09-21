package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/ocix/ocixtest"
)

func TestDirReadManifestReturnsRawBytes(t *testing.T) {
	root := writeDirCatalog(t, "alpha")
	want, _ := os.ReadFile(filepath.Join(root, "pkgs", "alpha", "pkg.yaml"))

	got, err := NewDir(root).ReadManifest(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("manifest bytes differ:\n%s\nwant\n%s", got, want)
	}
	var nf *PackageNotFoundError
	if _, err := NewDir(root).ReadManifest(context.Background(), "missing"); !errors.As(err, &nf) {
		t.Fatalf("missing package: got %v, want PackageNotFoundError", err)
	}
}

func TestFileReadManifestChecksDeclaredName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.yaml")
	want := strings.ReplaceAll(dirPkgYAML, "%NAME%", "alpha")
	os.WriteFile(path, []byte(want), 0o644)

	got, err := NewFile(path).ReadManifest(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if string(got) != want {
		t.Fatalf("manifest bytes differ:\n%s\nwant\n%s", got, want)
	}
	var nf *PackageNotFoundError
	if _, err := NewFile(path).ReadManifest(context.Background(), "beta"); !errors.As(err, &nf) {
		t.Fatalf("undeclared name: got %v, want PackageNotFoundError", err)
	}
}

func TestOCIReadManifestReturnsStoredBytes(t *testing.T) {
	store := newPulledStore(t, []ocixtest.FakeEntry{
		{Name: "tool", Description: "d", Latest: "1.0.0", YAML: []byte(pulledPkgYAML)},
	})
	src := NewOCI("remote", store)

	got, err := src.ReadManifest(context.Background(), "tool")
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if string(got) != pulledPkgYAML {
		t.Fatalf("manifest bytes differ:\n%s\nwant\n%s", got, pulledPkgYAML)
	}
	var nf *PackageNotFoundError
	if _, err := src.ReadManifest(context.Background(), "missing"); !errors.As(err, &nf) {
		t.Fatalf("missing package: got %v, want PackageNotFoundError", err)
	}
}
