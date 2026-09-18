package catalog

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
