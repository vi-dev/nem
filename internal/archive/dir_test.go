package archive

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/vi-dev/nem/internal/spec"
)

var storePlat = spec.Platform{OS: "linux", Arch: "amd64"}

func TestDirRoundtrip(t *testing.T) {
	root := t.TempDir()
	s := NewDir(root)
	if s.Has("foo") {
		t.Fatal("Has must be false before any push")
	}
	target, err := s.OpenRW(context.Background(), "foo")
	if err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	payload := []byte("archive bytes")
	if _, pushed, err := Push(context.Background(), target, "v1.0.0", storePlat, BytesBlob(payload), false); err != nil || !pushed {
		t.Fatalf("Push: pushed=%v err=%v", pushed, err)
	}
	if !s.Has("foo") {
		t.Fatal("Has must be true after push")
	}

	s2 := NewDir(root)
	target2, err := s2.Open(context.Background(), "foo")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	rc, err := Fetch(context.Background(), target2, "v1.0.0", storePlat)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("roundtrip mismatch: %q", got)
	}
	pullDir := t.TempDir()
	path, err := Pull(context.Background(), target2, "v1.0.0", storePlat, pullDir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
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

func TestDirHasFalseAfterOpenWithoutPush(t *testing.T) {
	root := t.TempDir()
	s := NewDir(root)
	if _, err := s.OpenRW(context.Background(), "foo"); err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "archives", "foo", "index.json")); err != nil {
		t.Skipf("OpenRW no longer writes index.json eagerly: %v", err)
	}
	if s.Has("foo") {
		t.Fatal("Has must stay false for a layout that was opened but never pushed to")
	}
}

func TestDirHasVersion(t *testing.T) {
	root := t.TempDir()
	s := NewDir(root)
	target, err := s.OpenRW(context.Background(), "foo")
	if err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	if _, _, err := Push(context.Background(), target, "v1", storePlat, BytesBlob([]byte("x")), false); err != nil {
		t.Fatalf("Push: %v", err)
	}

	if !s.HasVersion("foo", "v1") {
		t.Fatal("HasVersion(foo, v1) must be true for the pushed version")
	}
	if s.HasVersion("foo", "v2") {
		t.Fatal("HasVersion(foo, v2) must be false: that version was never staged")
	}
	if s.HasVersion("other", "v1") {
		t.Fatal("HasVersion(other, v1) must be false: that name was never staged")
	}

	if _, err := s.OpenRW(context.Background(), "bare"); err != nil {
		t.Fatalf("OpenRW: %v", err)
	}
	if s.HasVersion("bare", "v1") {
		t.Fatal("HasVersion must be false for a layout that was opened but never pushed to")
	}

	if _, err := os.Stat(filepath.Join(root, "archives", "never-opened", "index.json")); !os.IsNotExist(err) {
		t.Fatalf("HasVersion must not create index.json for a never-opened name: err=%v", err)
	}
	if s.HasVersion("never-opened", "v1") {
		t.Fatal("HasVersion must be false for a name the store never touched")
	}
	if _, err := os.Stat(filepath.Join(root, "archives", "never-opened", "index.json")); !os.IsNotExist(err) {
		t.Fatalf("HasVersion must not create index.json for a never-opened name: err=%v", err)
	}
}

func TestDirRejectsPathySegments(t *testing.T) {
	s := NewDir(t.TempDir())
	for _, bad := range []string{"a/b", `a\b`, "..", "."} {
		if _, err := s.Open(context.Background(), bad); err == nil {
			t.Fatalf("Open(%q) must fail", bad)
		}
	}
}
