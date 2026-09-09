package fsx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockCreatesAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")

	release, err := Lock(path)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
	release()

	release2, err := Lock(path)
	if err != nil {
		t.Fatalf("re-Lock: %v", err)
	}
	release2()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file deleted: %v", err)
	}
}

func TestLockExcludesConcurrentAcquirer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	release, err := Lock(path)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		r2, err := Lock(path)
		if err != nil {
			t.Errorf("second Lock: %v", err)
			close(acquired)
			return
		}
		close(acquired)
		r2()
	}()

	select {
	case <-acquired:
		t.Fatal("second Lock acquired while first still held")
	case <-time.After(100 * time.Millisecond):

	}
	release()
	select {
	case <-acquired:

	case <-time.After(2 * time.Second):
		t.Fatal("second Lock never acquired after release")
	}
}
