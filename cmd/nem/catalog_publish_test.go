package main

import (
	"context"
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
	restore := publish.SetTargetOpener(func(context.Context, string) (oras.Target, error) {
		t.Fatal("dry run must not open the target")
		return nil, nil
	})
	defer restore()

	nemHome := t.TempDir()
	fixtureDir := writeLintFixture(t, map[string]string{"go": publishFixtureGoPkg})

	out, errb, err := runNem(t, nemHome, "catalog", "publish", "example.com/cat", fixtureDir, "--dry-run")
	if err != nil {
		t.Fatalf("publish --dry-run: %v", err)
	}
	if !strings.Contains(out, "PACKAGE") || !strings.Contains(out, "go") || !strings.Contains(out, "v1.26.5") {
		t.Fatalf("stdout = %q, want the plan table", out)
	}
	if !strings.Contains(errb, "Dry run: would publish example.com/cat (1 packages)") {
		t.Fatalf("stderr = %q, want the dry-run summary", errb)
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
