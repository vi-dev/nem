package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func TestUseInstallsFromDirCatalogArchives(t *testing.T) {
	catalogDir := t.TempDir()
	pkgYAML := "schema: 2\nname: tl\ndescription: test tool\n" +
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\nversions: [v1.0.0]\n"
	writeFile(t, filepath.Join(catalogDir, "pkgs", "tl", "pkg.yaml"), pkgYAML)

	blob := testx.TarGz(t, map[string]string{"bin/tl": "#!/bin/sh\necho tl\n"}, 0o755)
	w, err := archive.NewDir(catalogDir).OpenRW(context.Background(), "tl")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := archive.Push(context.Background(), w, "v1.0.0", spec.Current(), archive.BytesBlob(blob), false); err != nil {
		t.Fatal(err)
	}

	nemHomeDir := t.TempDir()
	cfg := "catalogs:\n  - name: local\n    type: dir\n    path: " + catalogDir + "\n"
	writeFile(t, filepath.Join(nemHomeDir, "config.yaml"), cfg)

	t.Chdir(t.TempDir())
	if _, errb, err := runNem(t, nemHomeDir, "use", "tl@v1.0.0"); err != nil {
		t.Fatalf("use: %v\n%s", err, errb)
	}
	if _, err := os.Stat(filepath.Join(nemHomeDir, "packages", "tl", "v1.0.0", "bin", "tl")); err != nil {
		t.Fatalf("tl not installed from dir catalog archives: %v", err)
	}
}
