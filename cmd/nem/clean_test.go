package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vi-dev/nem/internal/testx"
	"github.com/vi-dev/nem/internal/usage"
)

func seedStaging(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "tmp", "go-build-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big"), bytes.Repeat([]byte("x"), 1024), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

func seedPackageVersion(t *testing.T, root, name, version string) string {
	t.Helper()
	dir := filepath.Join(root, "packages", name, version, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	return filepath.Join(root, "packages", name, version)
}

func TestCleanRemovesLeakedStaging(t *testing.T) {
	root := t.TempDir()
	dir := seedStaging(t, root)

	out, err := execNemMerged(t, root, "", "clean", "--grace", "0s")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("staging survived bare clean: %v", err)
	}
}

func TestCleanDryRunDeletesNothing(t *testing.T) {
	root := t.TempDir()
	dir := seedStaging(t, root)

	out, err := execNemMerged(t, root, "", "clean", "--grace", "0s", "--dry-run")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("--dry-run deleted something: %v", err)
	}
	if !strings.Contains(out, "leaked staging") {
		t.Errorf("--dry-run should still print the plan, got:\n%s", out)
	}
}

func TestCleanRejectsUnusedWithAll(t *testing.T) {
	root := t.TempDir()
	_, err := execNemMerged(t, root, "", "clean", "--unused", "30d", "--all")
	if err == nil {
		t.Fatal("--unused with --all must be a usage error")
	}
}

func TestCleanRefusalLeavesPackagesIntact(t *testing.T) {
	root := t.TempDir()
	dir := seedPackageVersion(t, root, "go", "1.26.5")

	orig := usageIndex
	t.Cleanup(func() { usageIndex = orig })
	old := time.Now().Add(-90 * 24 * time.Hour)
	usageIndex = func() usage.Index {
		return usage.Index{usage.Key("go", "1.26.5"): old}
	}

	out, err := execNemMerged(t, root, "n\n", "clean", "--unused", "30d")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("a refused confirmation deleted the package store: %v", statErr)
	}
}

func TestCleanYesSkipsThePromptAndDeletes(t *testing.T) {
	root := t.TempDir()
	dir := seedPackageVersion(t, root, "go", "1.26.5")

	out, err := execNemMerged(t, root, "", "clean", "--all", "-y")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("-y left the package version behind: %v", statErr)
	}
	if !strings.Contains(out, "Reclaimed") {
		t.Errorf("a completed run must report what it reclaimed, got:\n%s", out)
	}
}

func TestCleanReportsASkippedRevivedVersion(t *testing.T) {
	root := t.TempDir()
	seedPackageVersion(t, root, "go", "1.26.5")

	h := testx.HomeAt(root)
	if err := usage.Save(h, usage.Index{usage.Key("go", "1.26.5"): time.Now()}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	orig := usageIndex
	t.Cleanup(func() { usageIndex = orig })
	old := time.Now().Add(-90 * 24 * time.Hour)
	usageIndex = func() usage.Index {
		return usage.Index{usage.Key("go", "1.26.5"): old}
	}

	out, err := execNemMerged(t, root, "", "clean", "--unused", "30d", "-y")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	if !strings.Contains(out, "skipped go/1.26.5: used since planning") {
		t.Errorf("missing skip line for a revived version, got:\n%s", out)
	}
}

func TestCleanReportsFreedBytesOnAMidRunFailure(t *testing.T) {
	root := t.TempDir()
	seedPackageVersion(t, root, "go", "1.26.5")
	blocked := seedPackageVersion(t, root, "zig", "0.16.0")

	if err := os.Chmod(blocked, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	out, err := execNemMerged(t, root, "", "clean", "--all", "-y")
	if err == nil {
		t.Skip("could not force a deletion failure in this environment (e.g. running as root)")
	}
	if !strings.Contains(out, "Reclaimed") {
		t.Errorf("a failed run must still report what it freed, got:\n%s", out)
	}
}

func TestCleanUnusedPadsTheWindowByTheStampDebounce(t *testing.T) {
	root := t.TempDir()
	recent := seedPackageVersion(t, root, "recent", "1.0.0")
	stale := seedPackageVersion(t, root, "stale", "1.0.0")

	orig := usageIndex
	t.Cleanup(func() { usageIndex = orig })
	usageIndex = func() usage.Index {
		return usage.Index{
			usage.Key("recent", "1.0.0"): time.Now().Add(-61 * time.Minute),
			usage.Key("stale", "1.0.0"):  time.Now().Add(-2*time.Hour - time.Minute),
		}
	}

	out, err := execNemMerged(t, root, "", "clean", "--unused", "1h", "-y")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(recent); statErr != nil {
		t.Fatalf("a version stamped inside the debounce-padded window was evicted: %v", statErr)
	}
	if _, statErr := os.Stat(stale); !os.IsNotExist(statErr) {
		t.Fatalf("a version stamped past the debounce-padded window survived: %v", statErr)
	}
}

func TestCleanAllLeavesSyncedCatalogStoreOnDisk(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "catalogs", "official", "store")
	if err := os.MkdirAll(filepath.Join(store, "blobs", "sha256"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store, "blobs", "sha256", "aaa"), bytes.Repeat([]byte("x"), 2048), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store, "index.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	pkgDir := seedPackageVersion(t, root, "go", "1.26.5")

	out, err := execNemMerged(t, root, "", "clean", "--all", "-y")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	if strings.Contains(out, store) {
		t.Errorf("--all listed the catalog store in its plan:\n%s", out)
	}
	if _, statErr := os.Stat(store); statErr != nil {
		t.Fatalf("--all removed the catalog store: %v", statErr)
	}
	if _, statErr := os.Stat(pkgDir); !os.IsNotExist(statErr) {
		t.Fatalf("--all left the package version behind: %v", statErr)
	}
}
