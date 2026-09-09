package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/publish"
	"github.com/vi-dev/nem/internal/spec"

	"github.com/vi-dev/nem/internal/testx"
)

func TestPublishToUseRoundTrip(t *testing.T) {
	ctx := context.Background()

	shared, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}

	catalogRoot := downloadableDirCatalog(t, "")

	restorePublish := publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) {
		return shared, nil
	})
	defer restorePublish()

	pubNemHome := t.TempDir()
	if _, errb, err := runNem(t, pubNemHome, "catalog", "lint", catalogRoot); err != nil {
		t.Fatalf("lint: %v\n%s", err, errb)
	}

	if _, errb, err := runNem(t, pubNemHome, "catalog", "publish", "example.com/cat", catalogRoot, "--tag", "v2"); err != nil {
		t.Fatalf("publish: %v\n%s", err, errb)
	}
	if _, err := shared.Resolve(ctx, "v2"); err != nil {
		t.Fatalf("resolve published v2: %v", err)
	}

	consumerNemHome := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	if _, errb, err := runNem(t, consumerNemHome, "catalog", "add", "team", "example.com/cat:v2"); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}

	testx.Swap(t, &syncCatalogStore, func(ctx context.Context, ref, storePath string, progress ocix.ProgressFunc) error {
		_, err := ocix.SyncLocalCatalog(ctx, shared, "v2", storePath, progress)
		return err
	})

	restoreFetch := fetch.SetPullArchive(func(context.Context, string, string, string, spec.Platform, string) (string, error) {
		return "", ocix.ErrArchiveNotFound
	})
	defer restoreFetch()

	out, errb, err := runNem(t, consumerNemHome, "use", "team:tool@v1.0.0")
	if err != nil {
		t.Fatalf("use: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Installed tool v1.0.0") {
		t.Fatalf("narration missing install success line: %q", errb)
	}

	installDir, err := testNemHome(consumerNemHome).PackageDir("tool", "v1.0.0")
	if err != nil {
		t.Fatalf("PackageDir: %v", err)
	}
	binPath := filepath.Join(installDir, "bin", "tool")

	whichOut, whichErr, err := runNem(t, consumerNemHome, "which", "tool")
	if err != nil {
		t.Fatalf("which: %v\n%s", err, whichErr)
	}
	resolved := strings.TrimSpace(whichOut)
	if !strings.HasPrefix(resolved, consumerNemHome) {
		t.Fatalf("which tool = %q, want a path under NEM_HOME %q", resolved, consumerNemHome)
	}
	if resolved != binPath {
		t.Fatalf("which tool = %q, want installed bin path %q", resolved, binPath)
	}
}
