package archive_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote/errcode"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/spec"
)

type fetchCountingTarget struct {
	oras.ReadOnlyTarget
	fetches      int
	fetchedBytes int64
}

func (f *fetchCountingTarget) Fetch(ctx context.Context, d ocispec.Descriptor) (io.ReadCloser, error) {
	f.fetches++
	f.fetchedBytes += d.Size
	return f.ReadOnlyTarget.Fetch(ctx, d)
}

func TestPullReturnsRightPlatform(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	linuxBytes := []byte("linux/amd64 archive payload")
	darwinBytes := []byte("darwin/arm64 archive payload")
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{
		"linux/amd64":  linuxBytes,
		"darwin/arm64": darwinBytes,
	})

	dir := t.TempDir()
	path, err := archive.Pull(ctx, store, "v1.26.5", spec.Platform{OS: "linux", Arch: "amd64"}, dir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pulled archive: %v", err)
	}
	if string(got) != string(linuxBytes) {
		t.Fatalf("archive bytes = %q, want %q", got, linuxBytes)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("archive written to %q, want under %q", path, dir)
	}

	path2, err := archive.Pull(ctx, store, "v1.26.5", spec.Platform{OS: "darwin", Arch: "arm64"}, dir)
	if err != nil {
		t.Fatalf("Pull (darwin): %v", err)
	}
	got2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read pulled archive: %v", err)
	}
	if string(got2) != string(darwinBytes) {
		t.Fatalf("archive bytes = %q, want %q", got2, darwinBytes)
	}
}

func TestPullMissingTag(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	_, err = archive.Pull(ctx, store, "v9.9.9", spec.Platform{OS: "linux", Arch: "amd64"}, t.TempDir())
	if !errors.Is(err, archive.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPullMissingPlatform(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	_, err = archive.Pull(ctx, store, "v1.26.5", spec.Platform{OS: "darwin", Arch: "arm64"}, t.TempDir())
	if !errors.Is(err, archive.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

type resolveErrTarget struct {
	*oci.Store
	err error
}

func (f resolveErrTarget) Resolve(_ context.Context, _ string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, f.err
}

func TestPullNonNotFoundErrorPassesThrough(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	authErr := errors.New("401 unauthorized")
	faulty := resolveErrTarget{Store: store, err: authErr}

	_, err = archive.Pull(ctx, faulty, "v1.26.5", spec.Platform{OS: "linux", Arch: "amd64"}, t.TempDir())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, archive.ErrNotFound) {
		t.Fatalf("auth/network error must not map to ErrNotFound: %v", err)
	}
	if !errors.Is(err, authErr) {
		t.Fatalf("want wrapped authErr, got %v", err)
	}
}

func TestPullForbiddenMapsToNotFound(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	faulty := resolveErrTarget{Store: store, err: &errcode.ErrorResponse{StatusCode: 403}}

	_, err = archive.Pull(ctx, faulty, "v1.26.5", spec.Platform{OS: "linux", Arch: "amd64"}, t.TempDir())
	if !errors.Is(err, archive.ErrNotFound) {
		t.Fatalf("want ErrNotFound for a 403 resolve error, got %v", err)
	}
}

func TestResolveIndexIsFetchFree(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.0.0", map[string][]byte{"linux/amd64": []byte("x")})
	want, err := store.Resolve(ctx, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	counting := &fetchCountingTarget{ReadOnlyTarget: store}
	got, err := archive.ResolveIndex(ctx, counting, "v1.0.0")
	if err != nil {
		t.Fatalf("ResolveIndex: %v", err)
	}
	if got.Digest != want.Digest {
		t.Fatalf("digest = %s, want %s", got.Digest, want.Digest)
	}
	if counting.fetches != 0 {
		t.Fatalf("Fetch calls = %d, want 0: a resolve-only presence check must fetch no blobs", counting.fetches)
	}
}

func TestResolveIndexMissing(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.0.0", map[string][]byte{"linux/amd64": []byte("x")})

	_, err = archive.ResolveIndex(ctx, store, "v9.9.9")
	if !errors.Is(err, archive.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestResolveMatchesContentDigestWithoutFetchingIt(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("archive-bytes"), 4096)
	ocixtest.PushFakeArchive(t, store, "v1.0.0", map[string][]byte{
		"linux/amd64":  payload,
		"darwin/arm64": []byte("small"),
	})
	want := digest.FromBytes(payload)

	counting := &fetchCountingTarget{ReadOnlyTarget: store}
	desc, err := archive.Resolve(ctx, counting, "v1.0.0", spec.Platform{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if desc.Digest != want {
		t.Fatalf("digest = %s, want %s", desc.Digest, want)
	}
	if counting.fetchedBytes >= int64(len(payload)) {
		t.Fatalf("fetched %d bytes, which includes the %d-byte archive layer: the layer must never be fetched", counting.fetchedBytes, len(payload))
	}
}

func TestFetchVerifiesLayerDigest(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := oci.New(root)
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	payload := []byte("archive payload to corrupt")
	ocixtest.PushFakeArchive(t, store, "v1.0.0", map[string][]byte{"linux/amd64": payload})
	plat := spec.Platform{OS: "linux", Arch: "amd64"}

	rc, err := archive.Fetch(ctx, store, "v1.0.0", plat)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Fetch bytes = %q, want %q", got, payload)
	}

	desc2, err := archive.Resolve(ctx, store, "v1.0.0", plat)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	blob := filepath.Join(root, "blobs", "sha256", desc2.Digest.Encoded())
	if err := os.Chmod(blob, 0o644); err != nil {
		t.Fatalf("chmod blob: %v", err)
	}
	if err := os.WriteFile(blob, []byte("tampered content of same-ish"), 0o644); err != nil {
		t.Fatalf("corrupt blob: %v", err)
	}

	rc, err = archive.Fetch(ctx, store, "v1.0.0", plat)
	if err != nil {
		t.Fatalf("Fetch after corruption: %v", err)
	}
	if _, err := io.ReadAll(rc); err == nil {
		t.Fatal("reading a corrupted layer must fail digest verification")
	}
	rc.Close()
}
