package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReadsFrozenRecipesBesideIt(t *testing.T) {
	dir := t.TempDir()
	y := strings.ReplaceAll(dirPkgYAML, "%NAME%", "go")
	os.WriteFile(filepath.Join(dir, "pkg.yaml"), []byte(y), 0o644)
	os.WriteFile(filepath.Join(dir, "recipe@1.yaml"), []byte(dirRecipeYAML), 0o644)
	f := NewFile(filepath.Join(dir, "pkg.yaml"))
	pkg, _, err := f.Package(context.Background(), "go")
	if err != nil || len(pkg.Frozen) != 1 {
		t.Fatalf("Package: %+v, %v", pkg, err)
	}
	files, err := f.ReadFiles(context.Background(), "go")
	if err != nil || len(files.Recipes) != 1 {
		t.Fatalf("ReadFiles: %+v, %v", files, err)
	}
}

func TestFileReadsFrozenRecipesUnderGlobMetacharacters(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pkgs [v3]", "go")
	os.MkdirAll(dir, 0o755)
	y := strings.ReplaceAll(dirPkgYAML, "%NAME%", "go")
	os.WriteFile(filepath.Join(dir, "pkg.yaml"), []byte(y), 0o644)
	os.WriteFile(filepath.Join(dir, "recipe@1.yaml"), []byte(dirRecipeYAML), 0o644)
	pkg, _, err := NewFile(filepath.Join(dir, "pkg.yaml")).Package(context.Background(), "go")
	if err != nil || len(pkg.Frozen) != 1 {
		t.Fatalf("Package: %+v, %v", pkg, err)
	}
}
