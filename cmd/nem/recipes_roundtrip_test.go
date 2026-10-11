package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/publish"
	"github.com/vi-dev/nem/internal/testx"
)

func TestFrozenRecipeSurvivesPublishSyncAndUse(t *testing.T) {
	ctx := context.Background()
	shared, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalogRoot := downloadableDirCatalog(t, "  - version: v0.9.0\n    sha256:\n      darwin/arm64: \"SHA\"\n      darwin/amd64: \"SHA\"\n      linux/arm64: \"SHA\"\n      linux/amd64: \"SHA\"\n")
	pkgPath := filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml")
	pkgYAML, err := os.ReadFile(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	sha := shaFromFixture(string(pkgYAML))
	testx.WriteFile(t, pkgPath, strings.ReplaceAll(string(pkgYAML), `"SHA"`, `"`+sha+`"`))
	url := urlFromFixture(string(pkgYAML))
	recipe := "artifact:\n  url: \"" + url + "\"\ninstall:\n  - extract: {}\nenv:\n  - name: TOOL_LEGACY\n    value: \"1\"\n"
	testx.WriteFile(t, filepath.Join(catalogRoot, "pkgs", "tool", "recipe@0.yaml"), recipe)

	defer publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) { return shared, nil })()
	pubHome := t.TempDir()
	if _, errb, err := runNem(t, pubHome, "catalog", "lint", catalogRoot); err != nil {
		t.Fatalf("lint: %v\n%s", err, errb)
	}
	if _, errb, err := runNem(t, pubHome, "catalog", "publish", "example.com/cat", catalogRoot); err != nil {
		t.Fatalf("publish: %v\n%s", err, errb)
	}

	store, err := ocix.OpenStore(ctx, shared, "v3")
	if err != nil {
		t.Fatal(err)
	}
	remote, _, err := store.Files(ctx, "tool")
	if err != nil {
		t.Fatal(err)
	}
	local, err := catalog.NewDir(catalogRoot).ReadFiles(ctx, "tool")
	if err != nil || !reflect.DeepEqual(remote, local) {
		t.Fatalf("published files must equal the dir files byte for byte: %v", err)
	}

	home := t.TempDir()
	chdir(t, t.TempDir())
	if _, errb, err := runNem(t, home, "catalog", "add", "team", "example.com/cat:v3"); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}
	testx.Swap(t, &syncCatalogStore, func(ctx context.Context, ref, storePath string, progress ocix.ProgressFunc) error {
		_, err := ocix.SyncLocalCatalog(ctx, shared, "v3", storePath, progress)
		return err
	})
	defer archive.SetRepoOpener(func(string) (oras.Target, error) { return memory.New(), nil })()

	if _, errb, err := runNem(t, home, "use", "team:tool@v0.9.0"); err != nil {
		t.Fatalf("use: %v\n%s", err, errb)
	}
	meta, err := install.ReadMeta(testNemHome(home), "tool", "v0.9.0")
	if err != nil || len(meta.Env) != 1 || meta.Env[0].Name != "TOOL_LEGACY" {
		t.Fatalf("v0.9.0 must install through recipe@0: %+v, %v", meta, err)
	}
}

func fixtureValue(yaml, key string) string {
	_, rest, ok := strings.Cut(yaml, key+` "`)
	if !ok {
		return ""
	}
	value, _, _ := strings.Cut(rest, `"`)
	return value
}

func shaFromFixture(yaml string) string { return fixtureValue(yaml, "darwin/arm64:") }

func urlFromFixture(yaml string) string { return fixtureValue(yaml, "url:") }
