package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/fill"
	"github.com/vi-dev/nem/internal/mirror"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/publish"
	"github.com/vi-dev/nem/internal/spec"

	"github.com/vi-dev/nem/internal/testx"
)

type airArchiveFixtures struct {
	mu     sync.Mutex
	stores map[string]oras.Target
}

func newAirArchiveFixtures() *airArchiveFixtures {
	return &airArchiveFixtures{stores: map[string]oras.Target{}}
}

func (f *airArchiveFixtures) open(name string) oras.Target {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.stores[name]; ok {
		return s
	}
	s := memory.New()
	f.stores[name] = s
	return s
}

func writeCatalogPkg(t *testing.T, dir, name, yaml string) {
	t.Helper()
	pkgDir := filepath.Join(dir, "pkgs", name)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", pkgDir, err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "pkg.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write pkg.yaml for %s: %v", name, err)
	}
}

func TestAirGappedCatalogRoundTrip(t *testing.T) {
	toolArchive := makeTarGz(t, map[string]string{"bin/tool": "tool binary bytes"})
	toolSHA := sha256Hex(t, string(toolArchive))
	curlSHA := sha256Hex(t, "curl payload, never fetched")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tool/v1.0.0" {
			w.Write(toolArchive)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	catalogDir := t.TempDir()
	writeCatalogPkg(t, catalogDir, "tool", urlFillPkgYAML("tool", "v1.0.0", up.URL+"/tool/{{.Version}}", toolSHA))
	writeCatalogPkg(t, catalogDir, "curl", urlFillPkgYAML("curl", "1.0.0", up.URL+"/curl/{{.Version}}", curlSHA))

	srcCatalog := memory.New()
	srcArchives := newAirArchiveFixtures()

	connectedNemHome := t.TempDir()

	restorePublishTarget := publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) {
		return srcCatalog, nil
	})
	_, errb, err := runNem(t, connectedNemHome, "catalog", "publish", "example.com/cat", catalogDir, "--tag", "v2")
	restorePublishTarget()
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, errb)
	}

	t.Cleanup(fill.SetCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) {
		return srcCatalog, "v2", nil
	}))
	t.Cleanup(fill.SetArchivesOpener(func(_, name string) (oras.Target, error) {
		return srcArchives.open(name), nil
	}))
	t.Cleanup(fill.SetHTTPClient(up.Client()))

	_, errb, err = runNem(t, connectedNemHome, "catalog", "fill", "example.com/cat:v2", "--pkg", "tool")
	if err != nil {
		t.Fatalf("fill: %v\n%s", err, errb)
	}
	wantFillSummary := fmt.Sprintf("Filled 1 packages, %d fill(s), 0 heal(s), 0 present, 0 package(s) not fillable", len(spec.SupportedPlatforms))
	if !strings.Contains(errb, wantFillSummary) {
		t.Fatalf("fill summary = %q, want to contain %q", errb, wantFillSummary)
	}

	up.Close()

	dstCatalog := memory.New()
	dstArchives := newAirArchiveFixtures()

	t.Cleanup(mirror.SetSrcCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) {
		return srcCatalog, "v2", nil
	}))
	t.Cleanup(mirror.SetDstCatalogOpener(func(string) (oras.Target, string, error) {
		return dstCatalog, "v2", nil
	}))
	t.Cleanup(mirror.SetSrcArchivesOpener(func(_, name string) (oras.ReadOnlyTarget, error) {
		return srcArchives.open(name), nil
	}))
	t.Cleanup(mirror.SetDstArchivesOpener(func(_, name string) (oras.Target, error) {
		return dstArchives.open(name), nil
	}))

	_, errb, err = runNem(t, connectedNemHome, "catalog", "mirror", "example.com/cat:v2", "internal.example.com/cat:v2")
	if err != nil {
		t.Fatalf("mirror: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "Mirrored 2 packages, 1 tag(s)") {
		t.Fatalf("mirror summary:\n%s", errb)
	}

	ctx := context.Background()
	srcCatDesc, err := srcCatalog.Resolve(ctx, "v2")
	if err != nil {
		t.Fatalf("resolve src catalog: %v", err)
	}
	dstCatDesc, err := dstCatalog.Resolve(ctx, "v2")
	if err != nil {
		t.Fatalf("resolve dst catalog: %v", err)
	}
	if srcCatDesc.Digest != dstCatDesc.Digest {
		t.Fatalf("dst catalog digest %s != src digest %s", dstCatDesc.Digest, srcCatDesc.Digest)
	}
	srcArchDesc, err := srcArchives.open("tool").Resolve(ctx, "v1.0.0")
	if err != nil {
		t.Fatalf("resolve src tool archive: %v", err)
	}
	dstArchDesc, err := dstArchives.open("tool").Resolve(ctx, "v1.0.0")
	if err != nil {
		t.Fatalf("resolve dst tool archive: %v", err)
	}
	if srcArchDesc.Digest != dstArchDesc.Digest {
		t.Fatalf("dst tool archive digest %s != src digest %s", dstArchDesc.Digest, srcArchDesc.Digest)
	}

	consumerNemHome := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	if _, errb, err := runNem(t, consumerNemHome, "catalog", "add", "air", "internal.example.com/cat:v2"); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}

	testx.Swap(t, &syncCatalog, func(ctx context.Context, ref, storePath string, progress ocix.ProgressFunc) (string, error) {
		store, err := ocix.SyncLocalCatalog(ctx, dstCatalog, "v2", storePath, progress)
		if err != nil {
			return "", err
		}
		return store.Digest(), nil
	})

	if _, errb, err := runNem(t, consumerNemHome, "catalog", "update"); err != nil {
		t.Fatalf("catalog update: %v\n%s", err, errb)
	}

	t.Cleanup(fetch.SetPullArchive(func(ctx context.Context, catalogRef, name, tag string, plat spec.Platform, dir string) (string, error) {
		return ocix.PullArchiveFrom(ctx, dstArchives.open(name), tag, plat, dir)
	}))

	out, errb, err := runNem(t, consumerNemHome, "use", "air:tool@v1.0.0")
	if err != nil {
		t.Fatalf("use: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Installed tool v1.0.0") {
		t.Fatalf("narration missing install success line: %q", errb)
	}

	start := time.Now()
	_, errb, err = runNem(t, consumerNemHome, "use", "air:curl@1.0.0")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("use of a package absent from the mirror must fail")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("use took %s to fail; want a fast network-class error, not a hang", elapsed)
	}
	if !strings.Contains(errb, "curl") {
		t.Fatalf("stderr should name the failing package: %q", errb)
	}
	if !strings.Contains(errb, "connection refused") || !strings.Contains(errb, "network") {
		t.Fatalf("stderr should carry an ordinary network-class error and hint: %q", errb)
	}
}
