package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bareOCIBumpFixture = `schema: 2
name: tool
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {strip: 0}
versions:
  - v1.0.0
`

func TestCatalogBumpFlagWiring(t *testing.T) {
	t.Run("package version and json", func(t *testing.T) {
		dir := writeLintFixture(t, map[string]string{"tool": bareOCIBumpFixture})

		out, errOut, err := runNem(t, t.TempDir(), "catalog", "bump", "--json", "--package", "tool@v1.1.0", dir)
		if err != nil {
			t.Fatalf("bump: %v", err)
		}
		var rows []struct {
			Name string `json:"name"`
			Head string `json:"head"`
		}
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		if len(rows) != 1 || rows[0].Name != "tool" || rows[0].Head != "v1.1.0" {
			t.Fatalf("rows = %+v, want one tool row with head v1.1.0", rows)
		}
		if !strings.Contains(errOut, "Bumped 1 packages") {
			t.Fatalf("stderr = %q, want the summary line", errOut)
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		dir := writeLintFixture(t, map[string]string{"tool": bareOCIBumpFixture})
		path := filepath.Join(dir, "pkgs", "tool", "pkg.yaml")
		before, _ := os.ReadFile(path)

		_, errOut, err := runNem(t, t.TempDir(), "catalog", "bump", "--package", "tool@v1.1.0", "--dry-run", dir)
		if err != nil {
			t.Fatalf("bump --dry-run: %v", err)
		}
		if !strings.Contains(errOut, "Would bump tool v1.0.0 → v1.1.0") || !strings.Contains(errOut, "Would bump 1 packages") {
			t.Fatalf("stderr = %q, want the dry-run plan and summary", errOut)
		}
		if after, _ := os.ReadFile(path); string(after) != string(before) {
			t.Fatal("dry run must not modify the manifest")
		}
	})

	t.Run("failed package exits nonzero", func(t *testing.T) {
		dir := writeLintFixture(t, map[string]string{"tool": bareOCIBumpFixture})

		_, errOut, err := runNem(t, t.TempDir(), "catalog", "bump", "--package", "tool", dir)
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 1 {
			t.Fatalf("err = %v, want *ExitError{Code:1}", err)
		}
		if !strings.Contains(errOut, "no versionDiscovery") || !strings.Contains(errOut, "Failed tool") || !strings.Contains(errOut, "1 failed") {
			t.Fatalf("stderr = %q, want the discovery failure and summary", errOut)
		}
	})
}

func TestCatalogBumpDefaultsToCurrentDir(t *testing.T) {
	dir := writeLintFixture(t, map[string]string{"tool": bareOCIBumpFixture})
	t.Chdir(dir)

	_, errOut, err := runNem(t, t.TempDir(), "catalog", "bump")
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	if !strings.Contains(errOut, "Nothing to bump (1 without discovery)") {
		t.Fatalf("stderr = %q, want sweep summary for the current directory", errOut)
	}
}
