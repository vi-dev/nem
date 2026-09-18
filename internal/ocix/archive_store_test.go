package ocix

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vi-dev/nem/internal/spec"
)

var storePlat = spec.Platform{OS: "linux", Arch: "amd64"}

func TestArchiveStoreRoundtrip(t *testing.T) {
	s := NewArchiveStore(t.TempDir())
	if s.Has("foo") {
		t.Fatal("Has must be false before any push")
	}
	target, err := s.Open("foo")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	payload := []byte("archive bytes")
	if _, pushed, err := PushArchive(context.Background(), target, "v1.0.0", storePlat, payload, false); err != nil || !pushed {
		t.Fatalf("PushArchive: pushed=%v err=%v", pushed, err)
	}
	if !s.Has("foo") {
		t.Fatal("Has must be true after push")
	}

	s2 := NewArchiveStore(s.Root())
	target2, err := s2.Open("foo")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := ReadArchive(context.Background(), target2, "v1.0.0", storePlat)
	if err != nil {
		t.Fatalf("ReadArchive: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("roundtrip mismatch: %q", got)
	}
	pullDir := t.TempDir()
	path, err := PullArchiveFrom(context.Background(), target2, "v1.0.0", storePlat, pullDir)
	if err != nil {
		t.Fatalf("PullArchiveFrom: %v", err)
	}
	if filepath.Dir(path) != pullDir {
		t.Fatalf("pulled file %s must land directly in the given dir %s", path, pullDir)
	}
	pulled, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pulled file: %v", err)
	}
	if !bytes.Equal(pulled, payload) {
		t.Fatalf("pulled file content = %q, want %q", pulled, payload)
	}
}

func TestArchiveStoreHasFalseAfterOpenWithoutPush(t *testing.T) {
	s := NewArchiveStore(t.TempDir())
	if _, err := s.Open("foo"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), "archives", "foo", "index.json")); err != nil {
		t.Skipf("Open no longer writes index.json eagerly: %v", err)
	}
	if s.Has("foo") {
		t.Fatal("Has must stay false for a layout that was opened but never pushed to")
	}
}

func TestArchiveStoreHasTag(t *testing.T) {
	s := NewArchiveStore(t.TempDir())
	target, err := s.Open("foo")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, _, err := PushArchive(context.Background(), target, "v1", storePlat, []byte("x"), false); err != nil {
		t.Fatalf("PushArchive: %v", err)
	}

	if !s.HasTag("foo", "v1") {
		t.Fatal("HasTag(foo, v1) must be true for the pushed version")
	}
	if s.HasTag("foo", "v2") {
		t.Fatal("HasTag(foo, v2) must be false: that version was never staged")
	}
	if s.HasTag("other", "v1") {
		t.Fatal("HasTag(other, v1) must be false: that name was never staged")
	}

	if _, err := s.Open("bare"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s.HasTag("bare", "v1") {
		t.Fatal("HasTag must be false for a layout that was opened but never pushed to")
	}

	if _, err := os.Stat(filepath.Join(s.Root(), "archives", "never-opened", "index.json")); !os.IsNotExist(err) {
		t.Fatalf("HasTag must not create index.json for a never-opened name: err=%v", err)
	}
	if s.HasTag("never-opened", "v1") {
		t.Fatal("HasTag must be false for a name the store never touched")
	}
	if _, err := os.Stat(filepath.Join(s.Root(), "archives", "never-opened", "index.json")); !os.IsNotExist(err) {
		t.Fatalf("HasTag must not create index.json for a never-opened name: err=%v", err)
	}
}

func TestArchiveStoreRejectsPathySegments(t *testing.T) {
	s := NewArchiveStore(t.TempDir())
	for _, bad := range []string{"a/b", `a\b`, "..", "."} {
		if _, err := s.Open(bad); err == nil {
			t.Fatalf("Open(%q) must fail", bad)
		}
	}
}
