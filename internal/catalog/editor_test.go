package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pkgYAML(name string) []byte {
	return []byte(strings.ReplaceAll(dirPkgYAML, "%NAME%", name))
}

func TestDirCreateManifest(t *testing.T) {
	d := NewDir(t.TempDir())
	var _ Editor = d

	if err := d.CreateManifest("newpkg", pkgYAML("newpkg")); err != nil {
		t.Fatalf("create: %v", err)
	}
	pkg, _, err := d.Package(context.Background(), "newpkg")
	if err != nil || pkg.Name != "newpkg" {
		t.Fatalf("read back: %v, %v", pkg, err)
	}

	var ex *ManifestExistsError
	if err := d.CreateManifest("newpkg", pkgYAML("newpkg")); !errors.As(err, &ex) {
		t.Fatalf("want ManifestExistsError, got %v", err)
	}
	if err := d.CreateManifest("Bad Name", pkgYAML("Bad Name")); err == nil {
		t.Fatal("invalid name must be rejected")
	}
}

func TestDirUpdateManifest(t *testing.T) {
	root := writeDirCatalog(t, "tool")
	d := NewDir(root)

	data, err := d.ReadManifest("tool")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := os.Chmod(d.manifestPath("tool"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.UpdateManifest("tool", data); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(d.manifestPath("tool"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode not preserved: %v, %v", info.Mode(), err)
	}

	var nf *PackageNotFoundError
	if err := d.UpdateManifest("ghost", pkgYAML("ghost")); !errors.As(err, &nf) {
		t.Fatalf("want PackageNotFoundError, got %v", err)
	}
	if _, err := d.ReadManifest("ghost"); !errors.As(err, &nf) {
		t.Fatalf("read missing: %v", err)
	}
}

func TestDirUpdateManifestWritesUnvalidatedContent(t *testing.T) {
	root := writeDirCatalog(t, "tool")
	d := NewDir(root)
	if err := d.UpdateManifest("tool", []byte("not: [valid")); err != nil {
		t.Fatalf("UpdateManifest must not validate content: %v", err)
	}
	data, err := d.ReadManifest("tool")
	if err != nil || string(data) != "not: [valid" {
		t.Fatalf("want raw bytes written, got %q, %v", data, err)
	}
}

func TestDirUpdateManifestRejectsInvalidName(t *testing.T) {
	d := NewDir(t.TempDir())
	if err := d.UpdateManifest("../escape", []byte("x")); err == nil {
		t.Fatal("invalid names must be rejected (path safety)")
	}
}

func TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.yaml")
	f := NewFile(path)
	var _ Editor = f
	var _ Catalog = f
	ctx := context.Background()

	var nf *PackageNotFoundError
	if _, err := f.ReadManifest("tool"); !errors.As(err, &nf) {
		t.Fatalf("read missing: %v", err)
	}
	if _, _, err := f.Package(ctx, "tool"); !errors.As(err, &nf) {
		t.Fatalf("package on missing file: %v", err)
	}
	if err := f.UpdateManifest("tool", pkgYAML("tool")); !errors.As(err, &nf) {
		t.Fatalf("update missing: %v", err)
	}
	if err := f.CreateManifest("tool", pkgYAML("tool")); err != nil {
		t.Fatalf("create: %v", err)
	}
	var ex *ManifestExistsError
	if err := f.CreateManifest("tool", pkgYAML("tool")); !errors.As(err, &ex) {
		t.Fatalf("re-create: %v", err)
	}

	pkg, dig, err := f.Package(ctx, "tool")
	if err != nil || dig != "" || pkg.Name != "tool" {
		t.Fatalf("package: %+v, %q, %v", pkg, dig, err)
	}
	if _, _, err := f.Package(ctx, "other"); !errors.As(err, &nf) {
		t.Fatalf("package name mismatch: %v", err)
	}
	names, err := f.PackageNames(ctx)
	if err != nil || len(names) != 1 || names[0] != "tool" {
		t.Fatalf("names: %v, %v", names, err)
	}
	sums, err := f.Summaries(ctx)
	if err != nil || len(sums) != 1 || sums[0].Name != "tool" || sums[0].Latest != "v2.0.0" {
		t.Fatalf("summaries: %v, %v", sums, err)
	}
	versions, err := f.Versions(ctx, "tool")
	if err != nil || len(versions) != 2 || versions[0] != "v2.0.0" {
		t.Fatalf("versions: %v, %v", versions, err)
	}

	edited := []byte(strings.ReplaceAll(string(pkgYAML("tool")), "The tool tool", "Edited"))
	if err := f.UpdateManifest("tool", edited); err != nil {
		t.Fatalf("update: %v", err)
	}
	pkg, _, err = f.Package(ctx, "tool")
	if err != nil || pkg.Description != "Edited" {
		t.Fatalf("stale read after write: %q, %v", pkg.Description, err)
	}
}

func TestDirCreateManifestStatError(t *testing.T) {
	root := t.TempDir()
	d := NewDir(root)
	if err := os.MkdirAll(filepath.Join(root, "pkgs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkgs", "tool"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := d.CreateManifest("tool", pkgYAML("tool"))
	if err == nil {
		t.Fatal("stat failure must propagate, not fall through to create")
	}
	var ex *ManifestExistsError
	if errors.As(err, &ex) {
		t.Fatalf("want the underlying stat error, got ManifestExistsError")
	}
}

func TestDirReadManifestRejectsInvalidName(t *testing.T) {
	d := NewDir(t.TempDir())
	if _, err := d.ReadManifest("../escape"); err == nil {
		t.Fatal("path-shaped name must be rejected")
	}
}
