package bump

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBumpBackfillNeverReaddsExcludedVersion(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.8.1", "1.8.0")
	fixture := strings.Replace(jqFixture(srv.URL, jqVersionsYAML("1.8.2")+"  - version: 1.8.1\n    excluded: \"broken\"\n"),
		"schema: 2", "schema: 4", 1)
	dir := writeFixture(t, map[string]string{"jq": fixture})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	before := mustRead(t, path)
	stubList(t, "1.8.2", "1.8.1")

	_, errOut, err := runBump(t, Options{Backfill: 2}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	if !strings.Contains(errOut, "up to date") {
		t.Fatalf("stderr = %q, want up-to-date notice", errOut)
	}
	assertUnchanged(t, path, before)
}
