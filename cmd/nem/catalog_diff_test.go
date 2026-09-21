package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/diff"
)

const diffWiringFixture = `schema: 2
name: atool
description: Test tool
artifact:
  url: "https://example.com/atool-{{.Version}}.tar.gz"
install:
  - extract: {strip: 1}
bins: ["bin"]
versions:
  - version: 1.0.0
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "aaa"
      linux/arm64: "aaa"
      linux/amd64: "aaa"
`

const diffWiringFixtureV2 = `schema: 2
name: atool
description: Test tool
artifact:
  url: "https://example.com/atool-{{.Version}}.tar.gz"
install:
  - extract: {strip: 1}
bins: ["bin"]
versions:
  - version: 1.1.0
    sha256:
      darwin/arm64: "bbb"
      darwin/amd64: "bbb"
      linux/arm64: "bbb"
      linux/amd64: "bbb"
  - version: 1.0.0
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "aaa"
      linux/arm64: "aaa"
      linux/amd64: "aaa"
`

func writeDiffTarget(t *testing.T) string {
	t.Helper()
	return writeLintFixture(t, map[string]string{"atool": diffWiringFixture})
}

func TestCatalogDiffTableListsOnlyChanged(t *testing.T) {
	target := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixture,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
		"ztool": strings.Replace(diffWiringFixture, "name: atool", "name: ztool", 1),
	})
	dir := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixtureV2,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
	})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	for _, want := range []string{"PACKAGE", "STATUS", "BASE", "TARGET", "DIFF", "atool", "updated", "1.1.0", "ztool", "removed", "-"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want %q", out, want)
		}
	}
	if strings.Contains(out, "btool") {
		t.Fatalf("stdout = %q, unchanged btool must not be listed", out)
	}
	if !strings.Contains(errOut, "2 changed (0 new, 1 updated, 1 removed), 1 unchanged") {
		t.Fatalf("stderr = %q, want the counts summary", errOut)
	}
	if strings.Contains(errOut, dir) || strings.Contains(errOut, target) {
		t.Fatalf("stderr = %q, the summary must not repeat the catalog args", errOut)
	}
}

func TestCatalogDiffOutputJSONEmitsChanges(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixtureV2})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--output", "json")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	var changes []diff.Change
	if err := json.Unmarshal([]byte(out), &changes); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if len(changes) != 1 || changes[0].Name != "atool" || changes[0].Status != "updated" {
		t.Fatalf("changes = %+v, want one updated atool c", changes)
	}
	for _, key := range []string{"\"name\"", "\"status\"", "\"build\"", "\"base\"", "\"target\"", "\"diff\""} {
		if !strings.Contains(out, key) {
			t.Fatalf("stdout = %s, want key %s", out, key)
		}
	}
}

func TestCatalogDiffOutputJSONOmitsUnchanged(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixture})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--output", "json")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("stdout = %q, want an empty JSON array", out)
	}
}

func TestCatalogDiffRejectsUnknownOutput(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixture})

	_, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--output", "yaml")
	if err == nil || !strings.Contains(errOut, "yaml") {
		t.Fatalf("err = %v, stderr = %q; want an error naming the bad format", err, errOut)
	}
}

func TestCatalogDiffRequiresBothCatalogs(t *testing.T) {
	target := writeDiffTarget(t)

	_, _, err := runNem(t, t.TempDir(), "catalog", "diff", target)
	if err == nil || !strings.Contains(err.Error(), "accepts 2 arg(s)") {
		t.Fatalf("err = %v, want cobra's exact-args error", err)
	}
}

func TestCatalogDiffPackageFlagScopes(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixtureV2,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
	})

	_, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--package", "btool")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(errOut, "1 changed (1 new, 0 updated, 0 removed), 0 unchanged") {
		t.Fatalf("stderr = %q, want only btool compared", errOut)
	}
}

func TestCatalogDiffLintsBaseBeforeComparing(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{
		"atool": strings.Replace(diffWiringFixtureV2, "name: atool", "name: btool", 1),
	})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target)
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("err = %v, want exit code 1 from lint findings", err)
	}
	if !strings.Contains(errOut, `name "btool" does not match its directory "atool"`) {
		t.Fatalf("stderr = %q, want the lint finding", errOut)
	}
	if strings.Contains(errOut, "changed (") || out != "" {
		t.Fatalf("stdout = %q, stderr = %q; want no diff output after lint failure", out, errOut)
	}
}

func TestCatalogDiffCompletesOutputValues(t *testing.T) {
	out := runNemComplete(t, t.TempDir(), "catalog", "diff", "ghcr.io/org/cat:v2", "--output", "")
	if !strings.Contains(out, "text") || !strings.Contains(out, "json") {
		t.Fatalf("completion = %q, want text and json", out)
	}
}

func TestCatalogDiffCompletesPackagesFromBaseCatalog(t *testing.T) {
	dir := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixture,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
	})
	out := runNemComplete(t, t.TempDir(), "catalog", "diff", dir, "ghcr.io/org/cat:v2", "--package", "b")
	if !strings.Contains(out, "btool") || strings.Contains(out, "atool") {
		t.Fatalf("completion = %q, want btool only", out)
	}
}
