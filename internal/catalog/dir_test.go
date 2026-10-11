package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/spec"
)

const dirPkgYAML = `
schema: 2
name: %NAME%
description: The %NAME% tool
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - v2.0.0
  - v1.0.0
`

func writeDirCatalog(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		dir := filepath.Join(root, "pkgs", n)
		os.MkdirAll(dir, 0o755)
		y := []byte(strings.ReplaceAll(dirPkgYAML, "%NAME%", n))
		os.WriteFile(filepath.Join(dir, "pkg.yaml"), y, 0o644)
	}
	return root
}

func TestDirLoadAndVersions(t *testing.T) {
	root := writeDirCatalog(t, "go", "kubectl")
	d := NewDir(root)
	ctx := context.Background()

	pkg, dig, err := d.Package(ctx, "go")
	if err != nil || pkg.Name != "go" || dig != "" {
		t.Fatalf("Load: %+v, %q, %v", pkg, dig, err)
	}
	vs, err := d.Versions(ctx, "go")
	if err != nil || len(vs) != 2 || vs[0] != "v2.0.0" {
		t.Fatalf("Versions: %v, %v", vs, err)
	}

	var nf *PackageNotFoundError
	if _, _, err := d.Package(ctx, "absent"); !errors.As(err, &nf) {
		t.Fatalf("want PackageNotFoundError, got %v", err)
	}
}

func TestDirSummaries(t *testing.T) {
	root := writeDirCatalog(t, "zeta", "alpha")
	sums, err := NewDir(root).Summaries(context.Background())
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(sums) != 2 || sums[0].Name != "alpha" || sums[0].Latest != "v2.0.0" ||
		sums[1].Name != "zeta" || sums[0].Description == "" {
		t.Fatalf("summaries: %+v", sums)
	}
}

func TestDirNameMismatchErrors(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "pkgs", "alias")
	os.MkdirAll(dir, 0o755)
	y := []byte(strings.ReplaceAll(dirPkgYAML, "%NAME%", "other"))
	os.WriteFile(filepath.Join(dir, "pkg.yaml"), y, 0o644)

	if _, _, err := NewDir(root).Package(context.Background(), "alias"); err == nil {
		t.Fatal("manifest name mismatch must error")
	}
}

func TestDirInvalidPkgErrors(t *testing.T) {
	root := writeDirCatalog(t, "ok")
	bad := filepath.Join(root, "pkgs", "bad")
	os.MkdirAll(bad, 0o755)
	os.WriteFile(filepath.Join(bad, "pkg.yaml"), []byte("schema: 1\nname: bad\n"), 0o644)
	if _, _, err := NewDir(root).Package(context.Background(), "bad"); err == nil {
		t.Fatal("invalid pkg.yaml must error")
	}

	sums, err := NewDir(root).Summaries(context.Background())
	if err != nil || len(sums) != 1 || sums[0].Name != "ok" {
		t.Fatalf("summaries: %+v, %v", sums, err)
	}
}

func seedScannerJunk(t *testing.T, root string) {
	t.Helper()
	pkgs := filepath.Join(root, "pkgs")
	if err := os.WriteFile(filepath.Join(pkgs, "README.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgs, "Bad-Name"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgs, "no-manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(t.TempDir(), "linked")
	if err := os.MkdirAll(linkedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	y := []byte(strings.ReplaceAll(dirPkgYAML, "%NAME%", "linked"))
	if err := os.WriteFile(filepath.Join(linkedDir, "pkg.yaml"), y, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkedDir, filepath.Join(pkgs, "linked")); err != nil {
		t.Fatal(err)
	}
}

func TestDirPackageNames(t *testing.T) {
	root := writeDirCatalog(t, "go", "node")
	seedScannerJunk(t, root)

	names, err := NewDir(root).PackageNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"go", "node"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestDirPackageNamesMissingDirIsEmpty(t *testing.T) {
	names, err := NewDir(t.TempDir()).PackageNames(context.Background())
	if err != nil || names != nil {
		t.Fatalf("want nil, nil; got %v, %v", names, err)
	}
}

func TestDirSummariesSkipsNonPackages(t *testing.T) {
	root := writeDirCatalog(t, "go")
	seedScannerJunk(t, root)

	sums, err := NewDir(root).Summaries(context.Background())
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(sums) != 1 || sums[0].Name != "go" {
		t.Fatalf("summaries: %+v", sums)
	}
}

func TestDirScannersSkipUnreadablePackages(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	root := writeDirCatalog(t, "go", "hidden")
	locked := filepath.Join(root, "pkgs", "hidden")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	sums, err := NewDir(root).Summaries(context.Background())
	if err != nil || len(sums) != 1 || sums[0].Name != "go" {
		t.Fatalf("summaries: %+v, %v", sums, err)
	}
	names, err := NewDir(root).PackageNames(context.Background())
	if err != nil || !reflect.DeepEqual(names, []string{"go"}) {
		t.Fatalf("names: %v, %v", names, err)
	}
}

const dirRecipeYAML = `artifact:
  oci: ":{{.Version}}"
install:
  - extract: {strip: 1}
`

func TestDirAssemblesFrozenRecipes(t *testing.T) {
	root := writeDirCatalog(t, "go")
	os.WriteFile(filepath.Join(root, "pkgs", "go", "recipe@1.yaml"), []byte(dirRecipeYAML), 0o644)
	d := NewDir(root)
	ctx := context.Background()
	pkg, _, err := d.Package(ctx, "go")
	if err != nil {
		t.Fatalf("Package: %v", err)
	}
	r, err := pkg.Recipe("v1.0.0")
	if err != nil || r.Prefix != "1" || r.Install[0].Extract.Strip != 1 {
		t.Fatalf("Recipe(v1.0.0) = %+v, %v", r, err)
	}
	files, err := d.ReadFiles(ctx, "go")
	if err != nil || string(files.Recipes["recipe@1.yaml"]) != dirRecipeYAML {
		t.Fatalf("ReadFiles: %+v, %v", files, err)
	}
}

func TestDirRejectsBadRecipeFileName(t *testing.T) {
	root := writeDirCatalog(t, "go")
	os.WriteFile(filepath.Join(root, "pkgs", "go", "recipe@v1.yaml"), []byte(dirRecipeYAML), 0o644)
	_, _, err := NewDir(root).Package(context.Background(), "go")
	if err == nil || !strings.Contains(err.Error(), "recipe@<prefix>.yaml") {
		t.Fatalf("want filename load error, got %v", err)
	}
}

func TestDirWriteFilesRoundTrip(t *testing.T) {
	root := writeDirCatalog(t, "go")
	d := NewDir(root)
	ctx := context.Background()
	files, _ := d.ReadFiles(ctx, "go")
	files.Recipes = map[string][]byte{"recipe@1.yaml": []byte(dirRecipeYAML)}
	if err := d.WriteFiles(ctx, "go", files); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	again, err := d.ReadFiles(ctx, "go")
	if err != nil || !reflect.DeepEqual(again, files) {
		t.Fatalf("round trip: %+v vs %+v (%v)", again, files, err)
	}
	if err := d.WriteFiles(ctx, "go", spec.Files{Pkg: files.Pkg}); err != nil {
		t.Fatalf("WriteFiles pkg only: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "pkgs", "go", "recipe@1.yaml")); err != nil {
		t.Fatal("WriteFiles must never delete a recipe file")
	}
	if err := d.WriteFiles(ctx, "go", spec.Files{Recipes: map[string][]byte{"recipe@x.yaml": nil}}); err == nil {
		t.Fatal("WriteFiles must reject a bad recipe filename")
	}
	var nf *PackageNotFoundError
	if err := d.WriteFiles(ctx, "absent", files); !errors.As(err, &nf) {
		t.Fatalf("absent package: %v", err)
	}
}
