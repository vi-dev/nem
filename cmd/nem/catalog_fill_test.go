package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/fill"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/publish/publishtest"
	"github.com/vi-dev/nem/internal/spec"

	"github.com/vi-dev/nem/internal/testx"
)

func urlFillPkgYAML(name, version, urlTemplate, digestHex string) string {
	return string(publishtest.URLPkgYAML(name, version, urlTemplate, publishtest.UniformSha256(digestHex)))
}

func sha256Hex(t *testing.T, s string) string {
	t.Helper()
	return testx.Sha256Hex([]byte(s))
}

func TestCatalogFillCmd(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/go/1.0.0" {
			w.Write([]byte("go-payload"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer up.Close()

	src := memory.New()
	publishtest.PublishCatalog(t, src, "example.com/cat", map[string]string{
		"go": urlFillPkgYAML("go", "1.0.0", up.URL+"/go/{{.Version}}", sha256Hex(t, "go-payload")),
	})
	archives := memory.New()

	t.Cleanup(fill.SetCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil }))
	t.Cleanup(fill.SetArchivesOpener(func(string, string) (oras.Target, error) { return archives, nil }))
	t.Cleanup(fill.SetHTTPClient(up.Client()))

	nemHome := t.TempDir()
	_, errb, err := runNem(t, nemHome, "catalog", "fill", "example.com/cat:v2")
	if err != nil {
		t.Fatalf("fill: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "Filled 1 packages") {
		t.Fatalf("stderr missing summary line:\n%s", errb)
	}

	for _, plat := range spec.SupportedPlatforms {
		got, err := ocix.ArchiveLayerDigest(context.Background(), archives, "1.0.0", plat)
		if err != nil {
			t.Fatalf("resolve published archive for %s: %v", plat, err)
		}
		if got.Encoded() != sha256Hex(t, "go-payload") {
			t.Fatalf("%s digest = %s, want %s", plat, got.Encoded(), sha256Hex(t, "go-payload"))
		}
	}
}

func TestCatalogFillCmdDryRunWritesNothing(t *testing.T) {
	src := memory.New()
	publishtest.PublishCatalog(t, src, "example.com/cat", map[string]string{
		"go": urlFillPkgYAML("go", "1.0.0", "https://example.com/go/{{.Version}}", "deadbeef"),
	})
	archives := memory.New()

	t.Cleanup(fill.SetCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil }))
	t.Cleanup(fill.SetArchivesOpener(func(string, string) (oras.Target, error) { return archives, nil }))

	nemHome := t.TempDir()
	_, errb, err := runNem(t, nemHome, "catalog", "fill", "example.com/cat:v2", "--dry-run")
	if err != nil {
		t.Fatalf("fill --dry-run: %v\n%s", err, errb)
	}

	wantOutcome := fmt.Sprintf("Would fill go (%d fill(s), 0 heal(s))", len(spec.SupportedPlatforms))
	if !strings.Contains(errb, wantOutcome) {
		t.Fatalf("stderr missing package completion line %q:\n%s", wantOutcome, errb)
	}
	if !strings.Contains(errb, "Would fill 1 packages") {
		t.Fatalf("stderr missing dry-run summary line:\n%s", errb)
	}
	if strings.Contains(errb, "would fill") {
		t.Fatalf("stderr must not print per-item progress lines:\n%s", errb)
	}
}

func TestCatalogFillCmdExitsNonzeroOnItemFailure(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer up.Close()

	src := memory.New()
	publishtest.PublishCatalog(t, src, "example.com/cat", map[string]string{
		"go": urlFillPkgYAML("go", "1.0.0", up.URL+"/go/{{.Version}}", "deadbeef"),
	})
	archives := memory.New()

	t.Cleanup(fill.SetCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil }))
	t.Cleanup(fill.SetArchivesOpener(func(string, string) (oras.Target, error) { return archives, nil }))
	t.Cleanup(fill.SetHTTPClient(up.Client()))

	nemHome := t.TempDir()
	_, errb, err := runNem(t, nemHome, "catalog", "fill", "example.com/cat:v2")
	if err == nil {
		t.Fatal("a run with a failed item must exit nonzero")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("err = %v, want *ExitError{Code:1}", err)
	}
	if !strings.Contains(errb, "not found") {
		t.Fatalf("stderr missing the 404 warning:\n%s", errb)
	}
}

func TestCatalogFillCmdUnknownPkgErrors(t *testing.T) {
	src := memory.New()
	publishtest.PublishCatalog(t, src, "example.com/cat", map[string]string{
		"go": urlFillPkgYAML("go", "1.0.0", "https://example.com/go/{{.Version}}", "deadbeef"),
	})
	t.Cleanup(fill.SetCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil }))

	nemHome := t.TempDir()
	_, errb, err := runNem(t, nemHome, "catalog", "fill", "example.com/cat:v2", "--pkg", "nonexistent")
	if err == nil {
		t.Fatal("unknown --pkg name must error")
	}
	if !strings.Contains(errb, "nonexistent") {
		t.Fatalf("stderr missing the unknown package name:\n%s", errb)
	}
}
