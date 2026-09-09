package main

import (
	"encoding/json"
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
	t.Run("version and json", func(t *testing.T) {
		dir := writeLintFixture(t, map[string]string{"tool": bareOCIBumpFixture})
		path := filepath.Join(dir, "pkgs", "tool", "pkg.yaml")

		out, _, err := runNem(t, t.TempDir(), "catalog", "bump", "--json", "--version", "v1.1.0", path)
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
	})

	t.Run("backfill reaches options", func(t *testing.T) {
		dir := writeLintFixture(t, map[string]string{"tool": bareOCIBumpFixture})
		path := filepath.Join(dir, "pkgs", "tool", "pkg.yaml")

		_, _, err := runNem(t, t.TempDir(), "catalog", "bump", "--version", "v1.1.0", "--backfill", "2", path)
		if err == nil || !strings.Contains(err.Error(), "combined") {
			t.Fatalf("err = %v, want the version/backfill conflict from bump.Run", err)
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
	if !strings.Contains(errOut, "Checked 1 packages: 0 bumped, 0 up to date, 0 failed, 1 without discovery") {
		t.Fatalf("stderr = %q, want sweep summary for the current directory", errOut)
	}
}
