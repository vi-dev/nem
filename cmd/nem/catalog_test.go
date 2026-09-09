package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/ocix"
)

func TestCatalogAddListRemove(t *testing.T) {
	nemHome := t.TempDir()
	catDir := t.TempDir()

	_, errb, err := runNem(t, nemHome, "catalog", "add", "dev", catDir)
	if err != nil {
		t.Fatalf("add: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "Added catalog dev") {
		t.Fatalf("narration: %q", errb)
	}

	out, _, err := runNem(t, nemHome, "catalog", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NAME", "TYPE", "SOURCE", "official", "oci", "dev", "dir", catDir} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q:\n%s", want, out)
		}
	}

	if _, _, err := runNem(t, nemHome, "catalog", "add", "dev", catDir); err == nil {
		t.Fatal("duplicate add must fail")
	}

	os.MkdirAll(filepath.Join(nemHome, "catalogs", "dev", "store"), 0o755)
	_, _, err = runNem(t, nemHome, "catalog", "remove", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nemHome, "catalogs", "dev")); !os.IsNotExist(err) {
		t.Fatal("mirror dir not deleted")
	}
	out, _, _ = runNem(t, nemHome, "catalog", "list")

	if strings.Contains(out, catDir) {
		t.Fatalf("dev still listed:\n%s", out)
	}

	if _, _, err := runNem(t, nemHome, "catalog", "remove", "ghost"); err == nil {
		t.Fatal("removing unknown catalog must fail")
	}
}

func TestCatalogAddTypeDetection(t *testing.T) {
	nemHome := t.TempDir()
	if _, _, err := runNem(t, nemHome, "catalog", "add", "team", "ghcr.io/org/cat:v2"); err != nil {
		t.Fatalf("oci add: %v", err)
	}
	out, _, _ := runNem(t, nemHome, "catalog", "list")
	if !strings.Contains(out, "ghcr.io/org/cat:v2") {
		t.Fatalf("oci ref not listed:\n%s", out)
	}

	forced := t.TempDir()
	if _, _, err := runNem(t, nemHome, "catalog", "add", "forced", forced, "--type", "dir"); err != nil {
		t.Fatalf("forced dir add: %v", err)
	}
}

func TestCatalogAddRejectsTaglessOCIRef(t *testing.T) {
	nemHome := t.TempDir()
	if _, _, err := runNem(t, nemHome, "catalog", "add", "o", "ghcr.io/x/y"); err == nil {
		t.Fatal("tagless oci ref must be rejected")
	}
	out, _, _ := runNem(t, nemHome, "catalog", "list")
	if strings.Contains(out, "ghcr.io/x/y") {
		t.Fatalf("rejected catalog must not be persisted:\n%s", out)
	}
}

func TestCatalogUpdateSyncsOCI(t *testing.T) {
	nemHome := t.TempDir()
	var synced []string
	orig := syncCatalog
	syncCatalog = func(ctx context.Context, ref, storePath string, progress ocix.ProgressFunc) (string, error) {
		synced = append(synced, ref+"|"+storePath)
		return "sha256:fake", nil
	}
	defer func() { syncCatalog = orig }()

	dir := t.TempDir()
	if _, _, err := runNem(t, nemHome, "catalog", "add", "dev", dir); err != nil {
		t.Fatal(err)
	}
	_, errb, err := runNem(t, nemHome, "catalog", "update")
	if err != nil {
		t.Fatalf("update: %v\n%s", err, errb)
	}
	if len(synced) != 1 || !strings.Contains(synced[0], config.OfficialRef) {
		t.Fatalf("synced: %v", synced)
	}
	if !strings.Contains(errb, "Synced catalog official") {
		t.Fatalf("narration: %q", errb)
	}

	_, errb, err = runNem(t, nemHome, "catalog", "update", "dev")
	if err != nil || !strings.Contains(errb, "dir catalog") {
		t.Fatalf("dir update: %v, %q", err, errb)
	}

	if _, _, err := runNem(t, nemHome, "catalog", "update", "ghost"); err == nil {
		t.Fatal("unknown catalog must fail")
	}
}

func TestCatalogUpdateHonorsConfiguredHostTrust(t *testing.T) {
	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[],"annotations":{"org.vi-dev.nem.catalog.schemaVersion":"2"}}`)
	sum := sha256.Sum256(manifest)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v2/cat/manifests/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Length", strconv.Itoa(len(manifest)))
		if r.Method == http.MethodGet {
			w.Write(manifest)
		}
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	caPath := filepath.Join(t.TempDir(), "ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	host := srv.Listener.Addr().String()

	nemHomeDir := t.TempDir()
	cfgYAML := "hosts:\n  - host: " + host + "\n    ca: " + caPath + "\n"
	if err := os.WriteFile(filepath.Join(nemHomeDir, "config.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, errb, err := runNem(t, nemHomeDir, "catalog", "add", "trusted", host+"/cat:v2"); err != nil {
		t.Fatalf("add: %v\n%s", err, errb)
	}
	_, errb, err := runNem(t, nemHomeDir, "catalog", "update", "trusted")
	if err != nil {
		t.Fatalf("update over configured-CA TLS: %v\nstderr: %s", err, errb)
	}
	if !strings.Contains(errb, "Synced catalog trusted") {
		t.Fatalf("narration: %q", errb)
	}
}

func TestCatalogUpdateSkipsDisabled(t *testing.T) {
	dir := t.TempDir()
	h := testNemHome(dir)
	if err := config.SaveConfig(h, &config.Config{Catalogs: []config.CatalogEntry{
		{Name: "on", Type: "oci", Ref: "ghcr.io/x/on:v2"},
		{Name: "off", Type: "oci", Ref: "ghcr.io/x/off:v2", Disabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	var synced []string
	orig := syncCatalog
	syncCatalog = func(ctx context.Context, ref, storePath string, progress ocix.ProgressFunc) (string, error) {
		synced = append(synced, ref)
		return "", nil
	}
	defer func() { syncCatalog = orig }()

	if _, _, err := runNem(t, dir, "catalog", "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !slices.Equal(synced, []string{"ghcr.io/x/on:v2"}) {
		t.Fatalf("update should sync only enabled catalogs, synced %v", synced)
	}
}

func TestCatalogDisableEnable(t *testing.T) {
	dir := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, "")
	if _, _, err := runNem(t, dir, "catalog", "add", "tools", catalogRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runNem(t, dir, "catalog", "disable", "tools"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	cfg, _ := config.OpenConfig(testNemHome(dir))
	if e := cfg.Find("tools"); e == nil || !e.Disabled {
		t.Fatalf("tools should be disabled, got %+v", e)
	}

	if _, _, err := runNem(t, dir, "catalog", "disable", "tools"); err != nil {
		t.Fatalf("disable (idempotent): %v", err)
	}
	if _, _, err := runNem(t, dir, "catalog", "enable", "tools"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	cfg, _ = config.OpenConfig(testNemHome(dir))
	if e := cfg.Find("tools"); e == nil || e.Disabled {
		t.Fatalf("tools should be enabled, got %+v", e)
	}
}

func TestCatalogDisableUnknownNameErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runNem(t, dir, "catalog", "disable", "ghost"); err == nil {
		t.Fatal("disabling an unknown catalog should error")
	}
}

func TestCatalogListShowsStatus(t *testing.T) {
	dir := t.TempDir()
	h := testNemHome(dir)
	if err := config.SaveConfig(h, &config.Config{Catalogs: []config.CatalogEntry{
		{Name: "on", Type: "dir", Path: "/tmp/on"},
		{Name: "off", Type: "dir", Path: "/tmp/off", Disabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	out, _, err := runNem(t, dir, "catalog", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "disabled") || !strings.Contains(out, "enabled") {
		t.Fatalf("list should show enabled/disabled status, got:\n%s", out)
	}
}

func TestCatalogReorder(t *testing.T) {
	nemHome := t.TempDir()
	dir := t.TempDir()
	runNem(t, nemHome, "catalog", "add", "dev", dir)

	if _, _, err := runNem(t, nemHome, "catalog", "reorder", "dev", "official"); err != nil {
		t.Fatal(err)
	}
	out, _, _ := runNem(t, nemHome, "catalog", "list")
	if strings.Index(out, "dev") > strings.Index(out, "official") {
		t.Fatalf("order not applied:\n%s", out)
	}
	if _, _, err := runNem(t, nemHome, "catalog", "reorder", "dev"); err == nil {
		t.Fatal("partial reorder must fail")
	}
}

func TestCatalogHelpGroups(t *testing.T) {
	out, _, err := runNem(t, t.TempDir(), "catalog", "--help")
	if err != nil {
		t.Fatalf("catalog --help: %v", err)
	}
	consumption := strings.Index(out, "Catalog consumption:")
	maintenance := strings.Index(out, "Catalog maintenance:")
	if consumption < 0 || maintenance < 0 {
		t.Fatalf("catalog help missing group titles:\n%s", out)
	}
	if maintenance < consumption {
		t.Fatalf("catalog groups listed out of order:\n%s", out)
	}
}
