package ocix

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLastUpdated(t *testing.T) {
	store := t.TempDir()
	if _, err := LastUpdated(store); err == nil {
		t.Fatal("want error for a never-updated store")
	}

	path := filepath.Join(store, "index.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	ts, err := LastUpdated(store)
	if err != nil {
		t.Fatalf("LastUpdated: %v", err)
	}
	if d := ts.Sub(old); d < -time.Second || d > time.Second {
		t.Fatalf("LastUpdated = %v, want ~%v", ts, old)
	}
}
