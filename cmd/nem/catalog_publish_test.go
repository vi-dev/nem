package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/publish"
)

const publishFixtureGoPkg = `
schema: 2
name: go
description: The Go programming language
homepage: https://go.dev
license: BSD-3-Clause
artifact:
  url: "https://go.dev/dl/go{{.Version}}.{{.OS}}-{{.Arch}}.tar.gz"
install:
  - extract: {strip: 1}
versions:
  - version: v1.26.5
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "bbb"
      linux/arm64: "ccc"
      linux/amd64: "ddd"
`

func TestCatalogPublishPushesToTarget(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	restore := publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) { return store, nil })
	defer restore()

	nemHome := t.TempDir()
	fixtureDir := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})

	_, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", fixtureDir, "--tag", "v2")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !strings.Contains(errb, "Published example.com/cat: 1 pushed, 0 unchanged") {
		t.Fatalf("stderr = %q, want the publish summary", errb)
	}
	if _, err := store.Resolve(ctx, "v2"); err != nil {
		t.Fatalf("resolve v2: %v", err)
	}
}

func TestCatalogPublishDryRunPrintsPlanToStdout(t *testing.T) {
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	restore := publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) { return store, nil })
	defer restore()

	nemHome := t.TempDir()
	fixtureDir := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})

	out, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", fixtureDir, "--dry-run")
	if err != nil {
		t.Fatalf("publish --dry-run: %v", err)
	}
	if !strings.Contains(out, "STATUS") || !strings.Contains(out, "go") || !strings.Contains(out, "v1.26.5") || !strings.Contains(out, "new") {
		t.Fatalf("stdout = %q, want the plan table with a STATUS column", out)
	}
	if !strings.Contains(errb, "Would publish example.com/cat: 1 to push, tags v4, v4.") {
		t.Fatalf("stderr = %q, want the dry-run summary", errb)
	}
	if _, err := store.Resolve(context.Background(), "v4"); err == nil {
		t.Fatal("dry run must not move the v4 tag")
	}
}

func TestCatalogPublishLintFindingsBlockWrites(t *testing.T) {
	restore := publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) {
		t.Fatal("lint findings must block the publish before the target is opened")
		return nil, nil
	})
	defer restore()

	nemHome := t.TempDir()
	fixtureDir := writeLintFixture(t, map[string]string{"go": strings.Replace(publishFixtureGoPkg, "name: go", "name: golang", 1)})

	_, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", fixtureDir)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("err = %v, want *ExitError{Code:1}", err)
	}
	if !strings.Contains(errb, "does not match its directory") {
		t.Fatalf("stderr = %q, want the lint finding", errb)
	}
}

func publishStore(t *testing.T) *oci.Store {
	t.Helper()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	t.Cleanup(publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) { return store, nil }))
	return store
}

func TestCatalogPublishDryRunListsOnlyChanges(t *testing.T) {
	publishStore(t)
	nemHome := t.TempDir()
	dirA := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})
	if _, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dirA); err != nil {
		t.Fatalf("initial publish: %v\n%s", err, errb)
	}

	helm := strings.Replace(publishFixtureGoPkg, "name: go", "name: helm", 1)
	dirB := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg, "helm": helm})
	out, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dirB, "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, errb)
	}
	if !strings.Contains(out, "STATUS") || !strings.Contains(out, "helm") || !strings.Contains(out, "new") {
		t.Fatalf("stdout = %q, want helm listed as new", out)
	}
	if strings.Contains(out, "\ngo ") {
		t.Fatalf("stdout = %q, must not list the unchanged package", out)
	}
	if !strings.Contains(errb, "Would publish example.com/cat: 1 to push, 1 unchanged, tags v4, v4.") {
		t.Fatalf("stderr = %q, want the dry-run summary", errb)
	}
}

func TestCatalogPublishDryRunWithoutChangesPrintsNoTable(t *testing.T) {
	publishStore(t)
	nemHome := t.TempDir()
	dir := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})
	if _, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dir); err != nil {
		t.Fatalf("initial publish: %v\n%s", err, errb)
	}
	out, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dir, "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, errb)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want no table when nothing changes", out)
	}
	if !strings.Contains(errb, "Would publish example.com/cat: 1 unchanged, tags v4, v4.") {
		t.Fatalf("stderr = %q, want the dry-run summary", errb)
	}
}

func TestCatalogPublishSummaryCountsRemovedPackages(t *testing.T) {
	publishStore(t)
	nemHome := t.TempDir()
	alpha := strings.Replace(publishFixtureGoPkg, "name: go", "name: alpha", 1)
	dirA := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg, "alpha": alpha})
	if _, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dirA); err != nil {
		t.Fatalf("initial publish: %v\n%s", err, errb)
	}
	dirB := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})
	_, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dirB)
	if err != nil {
		t.Fatalf("second publish: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "Published example.com/cat: 0 pushed, 1 removed, 1 unchanged, tags v4, v4.") {
		t.Fatalf("stderr = %q, want the summary with the removed group", errb)
	}
}

type publishJSON struct {
	Ref      string   `json:"ref"`
	Tags     []string `json:"tags"`
	Index    string   `json:"index"`
	Packages []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Status  string `json:"status"`
	} `json:"packages"`
}

func TestCatalogPublishOutputJSON(t *testing.T) {
	store := publishStore(t)
	nemHome := t.TempDir()
	dirA := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})
	if _, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dirA); err != nil {
		t.Fatalf("initial publish: %v\n%s", err, errb)
	}
	helm := strings.Replace(publishFixtureGoPkg, "name: go", "name: helm", 1)
	dirB := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg, "helm": helm})

	out, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dirB, "--dry-run", "--output", "json")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, errb)
	}
	var dry publishJSON
	if err := json.Unmarshal([]byte(out), &dry); err != nil {
		t.Fatalf("stdout = %q, want a JSON document: %v", out, err)
	}
	if dry.Ref != "example.com/cat" || len(dry.Tags) != 2 || dry.Tags[0] != "v4" || !strings.HasPrefix(dry.Index, "sha256:") {
		t.Fatalf("document = %+v", dry)
	}
	if len(dry.Packages) != 1 || dry.Packages[0].Name != "helm" || dry.Packages[0].Version != "v1.26.5" || dry.Packages[0].Status != "new" {
		t.Fatalf("packages = %+v, want helm as new only", dry.Packages)
	}
	if !strings.Contains(errb, "Would publish example.com/cat: 1 to push, 1 unchanged") {
		t.Fatalf("stderr = %q, want the summary on stderr", errb)
	}

	out, errb, err = runNem(t, nemHome, "catalog", "publish", "example.com/cat", dirB, "--output", "json")
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, errb)
	}
	var real publishJSON
	if err := json.Unmarshal([]byte(out), &real); err != nil {
		t.Fatalf("stdout = %q, want a JSON document: %v", out, err)
	}
	desc, err := store.Resolve(context.Background(), "v4")
	if err != nil {
		t.Fatal(err)
	}
	if real.Index != desc.Digest.String() || real.Index != dry.Index {
		t.Fatalf("index = %q (dry run %q), want %s", real.Index, dry.Index, desc.Digest)
	}
	if len(real.Packages) != 1 || real.Packages[0].Status != "new" {
		t.Fatalf("packages = %+v", real.Packages)
	}
}

func TestCatalogPublishRejectsUnknownOutput(t *testing.T) {
	publishStore(t)
	dir := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})
	_, errOut, err := runNem(t, t.TempDir(), "catalog", "publish", "example.com/cat", dir, "--output", "yaml")
	if err == nil || !strings.Contains(errOut, "yaml") {
		t.Fatalf("err = %v, stderr = %q; want an error naming the bad format", err, errOut)
	}
}

func TestCatalogPublishCompletesOutputValues(t *testing.T) {
	out := runNemComplete(t, t.TempDir(), "catalog", "publish", "example.com/cat", "--output", "")
	if !strings.Contains(out, "text") || !strings.Contains(out, "json") {
		t.Fatalf("completion = %q, want text and json", out)
	}
}

func TestCatalogPublishForceOutputJSONListsPushedPackages(t *testing.T) {
	publishStore(t)
	nemHome := t.TempDir()
	dir := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})
	if _, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dir); err != nil {
		t.Fatalf("initial publish: %v\n%s", err, errb)
	}
	out, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", dir, "--force", "--output", "json")
	if err != nil {
		t.Fatalf("force publish: %v\n%s", err, errb)
	}
	var doc publishJSON
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout = %q, want a JSON document: %v", out, err)
	}
	if len(doc.Packages) != 1 || doc.Packages[0].Name != "go" || doc.Packages[0].Status != "unchanged" {
		t.Fatalf("packages = %+v, want the force-pushed package listed", doc.Packages)
	}
	if !strings.Contains(errb, "Published example.com/cat: 1 pushed, 0 unchanged") {
		t.Fatalf("stderr = %q", errb)
	}
}
