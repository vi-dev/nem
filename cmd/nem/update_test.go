package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/project"

	"github.com/vi-dev/nem/internal/testx"
)

func versionedDirCatalog(t *testing.T, tools map[string][]string, extra ...map[string]string) string {
	t.Helper()
	archive := makeTarGz(t, map[string]string{"bin/tool": "tool binary bytes"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	return versionedDirCatalogAt(t, srv.URL, sha256Hex(t, string(archive)), tools, extra...)
}

func versionedDirCatalogAt(t *testing.T, url, sha string, tools map[string][]string, extra ...map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, versions := range tools {
		dir := filepath.Join(root, "pkgs", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		var extraYAML string
		for _, e := range extra {
			extraYAML += e[name]
		}
		var b strings.Builder
		fmt.Fprintf(&b, `
schema: 2
name: %s
description: test tool %s
%sartifact:
  url: %q
install:
  - extract: {}
versions:
`, name, name, extraYAML, url)
		for _, v := range versions {
			fmt.Fprintf(&b, `  - version: %s
    sha256: {darwin/arm64: %q, darwin/amd64: %q, linux/arm64: %q, linux/amd64: %q}
`, v, sha, sha, sha, sha)
		}
		if err := os.WriteFile(filepath.Join(dir, "pkg.yaml"), []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write pkg.yaml: %v", err)
		}
	}
	return root
}

func updateProject(t *testing.T, catalogRoot string, useArgs ...string) (nemHomeDir, projDir string) {
	t.Helper()
	nemHomeDir = t.TempDir()
	projDir = t.TempDir()
	chdir(t, projDir)
	if _, errb, err := runNem(t, nemHomeDir, "catalog", "add", "demo", catalogRoot); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}
	if len(useArgs) > 0 {
		if _, errb, err := runNem(t, nemHomeDir, append([]string{"use"}, useArgs...)...); err != nil {
			t.Fatalf("use: %v\n%s", err, errb)
		}
	}
	return nemHomeDir, projDir
}

func TestUpdateBumpsAllDeclaredToolsToLatest(t *testing.T) {
	catalogRoot := versionedDirCatalog(t, map[string][]string{"tool": {"v1.1.0", "v1.0.0"}})
	nemHomeDir, projDir := updateProject(t, catalogRoot, "demo:tool@v1.0.0")

	out, errb, err := runNem(t, nemHomeDir, "update")
	if err != nil {
		t.Fatalf("update: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if out != "" {
		t.Fatalf("stdout must stay empty like use's: %q", out)
	}
	installed := strings.Index(errb, "Installed tool v1.1.0")
	updated := strings.Index(errb, "Updated tool v1.0.0 → v1.1.0")
	if installed < 0 || updated < 0 {
		t.Fatalf("stderr should carry both install and update narration: %q", errb)
	}
	if updated < installed {
		t.Fatalf("Updated must not print before the install finished: %q", errb)
	}

	m, err := project.LoadManifest(filepath.Join(projDir, "nem.toml"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.Tools) != 1 || m.Tools[0].Key.String() != "demo:tool" || m.Tools[0].Version != "v1.1.0" {
		t.Fatalf("manifest tools after update: %+v", m.Tools)
	}

	lf, err := project.LoadLock(filepath.Join(projDir, "nem.lock"))
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if len(lf.Packages) != 1 || lf.Packages[0].Version != "v1.1.0" {
		t.Fatalf("lock packages after update: %+v", lf.Packages)
	}

	if !install.IsInstalled(testNemHome(nemHomeDir), "tool", "v1.1.0") {
		t.Fatal("tool v1.1.0 not installed after update")
	}
}

func TestUpdateSingleToolLeavesOthersPinned(t *testing.T) {
	catalogRoot := versionedDirCatalog(t, map[string][]string{
		"toola": {"v1.1.0", "v1.0.0"},
		"toolb": {"v2.1.0", "v2.0.0"},
	})
	nemHomeDir, projDir := updateProject(t, catalogRoot, "demo:toola@v1.0.0", "demo:toolb@v2.0.0")

	out, errb, err := runNem(t, nemHomeDir, "update", "toola")
	if err != nil {
		t.Fatalf("update toola: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if out != "" {
		t.Fatalf("stdout must stay empty like use's: %q", out)
	}
	if !strings.Contains(errb, "Updated toola v1.0.0 → v1.1.0") {
		t.Fatalf("stderr should narrate the update: %q", errb)
	}
	if strings.Contains(errb, "Updated toolb") {
		t.Fatalf("the unselected tool must not be narrated as updated: %q", errb)
	}

	m, err := project.LoadManifest(filepath.Join(projDir, "nem.toml"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	got := map[string]string{}
	for _, tool := range m.Tools {
		got[tool.Key.Name] = tool.Version
	}
	if got["toola"] != "v1.1.0" {
		t.Fatalf("toola should update to v1.1.0, manifest: %+v", m.Tools)
	}
	if got["toolb"] != "v2.0.0" {
		t.Fatalf("toolb must stay pinned at v2.0.0, manifest: %+v", m.Tools)
	}
}

func TestUpdateFailedInstallDoesNotClaimUpdated(t *testing.T) {
	archive := makeTarGz(t, map[string]string{"bin/tool": "tool binary bytes"})
	var serveGarbage atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveGarbage.Load() {
			w.Write([]byte("not the archive"))
			return
		}
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	root := versionedDirCatalogAt(t, srv.URL, sha256Hex(t, string(archive)), map[string][]string{"tool": {"v1.1.0", "v1.0.0"}})

	nemHomeDir, _ := updateProject(t, root, "demo:tool@v1.0.0")

	serveGarbage.Store(true)
	_, errb, err := runNem(t, nemHomeDir, "update")
	if err == nil {
		t.Fatal("want error when the new version fails to install")
	}
	if strings.Contains(errb, "Updated tool") {
		t.Fatalf("a failed install must not be narrated as updated: %q", errb)
	}
}

func TestUpdateUpToDateIsQuietSuccess(t *testing.T) {
	catalogRoot := versionedDirCatalog(t, map[string][]string{"tool": {"v1.0.0"}})
	nemHomeDir, _ := updateProject(t, catalogRoot, "demo:tool")

	out, errb, err := runNem(t, nemHomeDir, "update")
	if err != nil {
		t.Fatalf("update on an up-to-date project must succeed: %v\n%s", err, errb)
	}
	if out != "" {
		t.Fatalf("stdout must stay empty when nothing changes: %q", out)
	}
	if !strings.Contains(errb, "up to date") {
		t.Fatalf("stderr should say the tools are up to date: %q", errb)
	}
}

func TestUpdateDryRunWritesNothing(t *testing.T) {
	catalogRoot := versionedDirCatalog(t, map[string][]string{"tool": {"v1.1.0", "v1.0.0"}})
	nemHomeDir, projDir := updateProject(t, catalogRoot, "demo:tool@v1.0.0")

	out, errb, err := runNem(t, nemHomeDir, "update", "--dry-run")
	if err != nil {
		t.Fatalf("update --dry-run: %v\n%s", err, errb)
	}
	if !strings.Contains(out, "tool") || !strings.Contains(out, "v1.0.0") || !strings.Contains(out, "v1.1.0") {
		t.Fatalf("stdout should list the would-be change: %q", out)
	}

	m, err := project.LoadManifest(filepath.Join(projDir, "nem.toml"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.Tools) != 1 || m.Tools[0].Version != "v1.0.0" {
		t.Fatalf("dry run must not touch the manifest: %+v", m.Tools)
	}
	lf, err := project.LoadLock(filepath.Join(projDir, "nem.lock"))
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if len(lf.Packages) != 1 || lf.Packages[0].Version != "v1.0.0" {
		t.Fatalf("dry run must not touch the lock: %+v", lf.Packages)
	}
	if install.IsInstalled(testNemHome(nemHomeDir), "tool", "v1.1.0") {
		t.Fatal("dry run must not install anything")
	}
}

func TestUpdateUndeclaredPackageErrors(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)
	if err := os.WriteFile(filepath.Join(projDir, "nem.toml"), []byte("[tools]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runNem(t, nemHomeDir, "update", "ghost")
	if err == nil {
		t.Fatal("want error for undeclared package")
	}
	if !strings.Contains(err.Error(), "ghost") || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("error should name the package and say it's not declared: %v", err)
	}
}

func TestUpdateRefusesDowngrade(t *testing.T) {
	cases := []struct {
		name        string
		versions    []string
		useArg      string
		manifest    string
		wantErr     []string
		wantVersion string
		wantNoLock  bool
	}{
		{
			name:        "declared version absent from catalog",
			versions:    []string{"v1.0.0"},
			manifest:    "[tools]\ntool = 'v2.0.0'\n",
			wantErr:     []string{"downgrade", "v2.0.0", "v1.0.0", "nem use tool@v1.0.0"},
			wantVersion: "v2.0.0",
			wantNoLock:  true,
		},
		{
			name:        "declared prerelease sorts above the catalog head",
			versions:    []string{"v1.2.3", "v1.3.0-rc1"},
			useArg:      "demo:tool@v1.3.0-rc1",
			wantErr:     []string{"refusing to downgrade tool from v1.3.0-rc1 to v1.2.3"},
			wantVersion: "v1.3.0-rc1",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			catalogRoot := versionedDirCatalog(t, map[string][]string{"tool": c.versions})
			var useArgs []string
			if c.useArg != "" {
				useArgs = append(useArgs, c.useArg)
			}
			nemHomeDir, projDir := updateProject(t, catalogRoot, useArgs...)
			if c.manifest != "" {
				if err := os.WriteFile(filepath.Join(projDir, "nem.toml"), []byte(c.manifest), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			_, _, err := runNem(t, nemHomeDir, "update")
			if err == nil {
				t.Fatal("want error when update would pick below the declared version")
			}
			for _, want := range c.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q should contain %q", err, want)
				}
			}
			m, err := project.LoadManifest(filepath.Join(projDir, "nem.toml"))
			if err != nil {
				t.Fatalf("LoadManifest: %v", err)
			}
			if len(m.Tools) != 1 || m.Tools[0].Version != c.wantVersion {
				t.Fatalf("a refused downgrade must leave the manifest untouched: %+v", m.Tools)
			}
			if c.wantNoLock {
				if _, err := os.Stat(filepath.Join(projDir, "nem.lock")); !os.IsNotExist(err) {
					t.Fatal("a refused downgrade must not write a lock")
				}
			}
		})
	}
}

func TestUpdateWarnsWhenCatalogStale(t *testing.T) {
	nemHomeDir := t.TempDir()
	h := testNemHome(nemHomeDir)
	projDir := t.TempDir()
	chdir(t, projDir)

	if _, _, err := runNem(t, nemHomeDir, "catalog", "add", "demo", "ghcr.io/x/y:v2"); err != nil {
		t.Fatal(err)
	}
	var calls []string
	testx.Swap(t, &syncCatalogStore, fakeOCICatalogSync(t, &calls, otherPlatform(t)))

	if _, errb, err := runNem(t, nemHomeDir, "use", "demo:tool"); err != nil {
		t.Fatalf("use: %v\n%s", err, errb)
	}

	store, err := h.CatalogStore("demo")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(filepath.Join(store, "index.json"), old, old); err != nil {
		t.Fatalf("age the mirror: %v", err)
	}

	_, errb, err := runNem(t, nemHomeDir, "update")
	if err != nil {
		t.Fatalf("update: %v\n%s", err, errb)
	}
	if strings.Contains(errb, "last synced") {
		t.Fatalf("a mirror under a week old must not warn: %q", errb)
	}

	old = time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(store, "index.json"), old, old); err != nil {
		t.Fatalf("age the mirror: %v", err)
	}

	_, errb, err = runNem(t, nemHomeDir, "update")
	if err != nil {
		t.Fatalf("update: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "Catalog demo last synced 8 days ago") {
		t.Fatalf("stderr should warn about the stale mirror: %q", errb)
	}
	if !strings.Contains(errb, "nem catalog update") {
		t.Fatalf("stderr should hint at refreshing catalogs: %q", errb)
	}
	if strings.Contains(errb, "Catalog official") {
		t.Fatalf("uninvolved catalogs must not be warned about: %q", errb)
	}
}

func TestUpdateGlobalTargetsGlobalManifest(t *testing.T) {
	catalogRoot := versionedDirCatalog(t, map[string][]string{"tool": {"v1.1.0", "v1.0.0"}})
	nemHomeDir, projDir := updateProject(t, catalogRoot, "-g", "demo:tool@v1.0.0")

	_, errb, err := runNem(t, nemHomeDir, "update", "-g")
	if err != nil {
		t.Fatalf("update -g: %v\n%s", err, errb)
	}

	m, err := project.LoadManifest(testNemHome(nemHomeDir).GlobalManifest())
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.Tools) != 1 || m.Tools[0].Version != "v1.1.0" {
		t.Fatalf("global manifest after update -g: %+v", m.Tools)
	}
	if _, err := os.Stat(filepath.Join(projDir, "nem.toml")); !os.IsNotExist(err) {
		t.Fatal("update -g must not create a project manifest")
	}
}

func TestUpdateGlobalWithoutGlobalManifestErrors(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	_, _, err := runNem(t, nemHomeDir, "update", "-g")
	if err == nil {
		t.Fatal("want error when no global manifest exists")
	}
	if !errors.Is(err, project.ErrNoManifest) {
		t.Fatalf("want ErrNoManifest, got %v", err)
	}
}

func TestUpdateAcceptsQualifiedDeclaredName(t *testing.T) {
	catalogRoot := versionedDirCatalog(t, map[string][]string{"tool": {"v1.1.0", "v1.0.0"}})
	nemHomeDir, _ := updateProject(t, catalogRoot, "demo:tool@v1.0.0")

	_, errb, err := runNem(t, nemHomeDir, "update", "demo:tool")
	if err != nil {
		t.Fatalf("update by the declared qualified name must work: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "Updated tool v1.0.0 → v1.1.0") {
		t.Fatalf("stderr: %q", errb)
	}

	_, _, err = runNem(t, nemHomeDir, "update", "other:tool")
	if err == nil {
		t.Fatal("want error for a mismatched catalog qualifier")
	}
	if !strings.Contains(err.Error(), "demo:tool") {
		t.Fatalf("error should name the declared key: %v", err)
	}
}

func TestUpdateStaleWarningSurvivesRefusal(t *testing.T) {
	nemHomeDir := t.TempDir()
	h := testNemHome(nemHomeDir)
	projDir := t.TempDir()
	chdir(t, projDir)

	if _, _, err := runNem(t, nemHomeDir, "catalog", "add", "demo", "ghcr.io/x/y:v2"); err != nil {
		t.Fatal(err)
	}
	var calls []string
	testx.Swap(t, &syncCatalogStore, fakeOCICatalogSync(t, &calls, otherPlatform(t)))

	if _, errb, err := runNem(t, nemHomeDir, "use", "demo:tool"); err != nil {
		t.Fatalf("use: %v\n%s", err, errb)
	}
	if err := os.WriteFile(filepath.Join(projDir, "nem.toml"), []byte("[tools]\n\"demo:tool\" = 'v2.0.0'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := h.CatalogStore("demo")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(store, "index.json"), old, old); err != nil {
		t.Fatalf("age the mirror: %v", err)
	}

	_, errb, err := runNem(t, nemHomeDir, "update")
	if err == nil {
		t.Fatal("want the downgrade refusal")
	}
	if !strings.Contains(err.Error(), "refusing to downgrade") {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(err.Error(), "nem use demo:tool@v1.0.0") {
		t.Fatalf("the hint must keep the catalog qualifier: %v", err)
	}
	if !strings.Contains(errb, "Catalog demo last synced 8 days ago") {
		t.Fatalf("the refusal must not hide the stale-mirror warning: %q", errb)
	}
}

func TestSyncAgePhrase(t *testing.T) {
	cases := []struct {
		age  time.Duration
		want string
	}{
		{47 * time.Hour, "1 day"},
		{72 * time.Hour, "3 days"},
	}
	for _, c := range cases {
		if got := syncAgePhrase(c.age); got != c.want {
			t.Errorf("syncAgePhrase(%v) = %q, want %q", c.age, got, c.want)
		}
	}
}

func TestUpdateVersionArgRefused(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)
	if err := os.WriteFile(filepath.Join(projDir, "nem.toml"), []byte("[tools]\ntool = 'v1.0.0'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runNem(t, nemHomeDir, "update", "tool@v1.1.0")
	if err == nil {
		t.Fatal("want error for versioned argument")
	}
	if !strings.Contains(err.Error(), "nem use") {
		t.Fatalf("error should point at `nem use` for version pinning: %v", err)
	}
}
