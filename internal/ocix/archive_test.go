package ocix_test

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

	"github.com/vi-dev/nem/internal/ocix"
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

func TestPullArchiveFromReturnsRightPlatform(t *testing.T) {
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
	path, err := ocix.PullArchiveFrom(ctx, store, "v1.26.5", spec.Platform{OS: "linux", Arch: "amd64"}, dir)
	if err != nil {
		t.Fatalf("PullArchiveFrom: %v", err)
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

	path2, err := ocix.PullArchiveFrom(ctx, store, "v1.26.5", spec.Platform{OS: "darwin", Arch: "arm64"}, dir)
	if err != nil {
		t.Fatalf("PullArchiveFrom (darwin): %v", err)
	}
	got2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read pulled archive: %v", err)
	}
	if string(got2) != string(darwinBytes) {
		t.Fatalf("archive bytes = %q, want %q", got2, darwinBytes)
	}
}

func TestPullArchiveFromMissingTag(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	_, err = ocix.PullArchiveFrom(ctx, store, "v9.9.9", spec.Platform{OS: "linux", Arch: "amd64"}, t.TempDir())
	if !errors.Is(err, ocix.ErrArchiveNotFound) {
		t.Fatalf("want ErrArchiveNotFound, got %v", err)
	}
}

func TestPullArchiveFromMissingPlatform(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	_, err = ocix.PullArchiveFrom(ctx, store, "v1.26.5", spec.Platform{OS: "darwin", Arch: "arm64"}, t.TempDir())
	if !errors.Is(err, ocix.ErrArchiveNotFound) {
		t.Fatalf("want ErrArchiveNotFound, got %v", err)
	}
}

type resolveErrTarget struct {
	*oci.Store
	err error
}

func (f resolveErrTarget) Resolve(_ context.Context, _ string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, f.err
}

func TestPullArchiveFromNonNotFoundErrorPassesThrough(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	authErr := errors.New("401 unauthorized")
	faulty := resolveErrTarget{Store: store, err: authErr}

	_, err = ocix.PullArchiveFrom(ctx, faulty, "v1.26.5", spec.Platform{OS: "linux", Arch: "amd64"}, t.TempDir())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, ocix.ErrArchiveNotFound) {
		t.Fatalf("auth/network error must not map to ErrArchiveNotFound: %v", err)
	}
	if !errors.Is(err, authErr) {
		t.Fatalf("want wrapped authErr, got %v", err)
	}
}

func TestPullArchiveFromForbiddenMapsToArchiveNotFound(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.26.5", map[string][]byte{"linux/amd64": []byte("x")})

	faulty := resolveErrTarget{Store: store, err: &errcode.ErrorResponse{StatusCode: 403}}

	_, err = ocix.PullArchiveFrom(ctx, faulty, "v1.26.5", spec.Platform{OS: "linux", Arch: "amd64"}, t.TempDir())
	if !errors.Is(err, ocix.ErrArchiveNotFound) {
		t.Fatalf("want ErrArchiveNotFound for a 403 resolve error, got %v", err)
	}
}

func TestArchivesRef(t *testing.T) {
	cases := []struct {
		name       string
		catalogRef string
		pkgName    string
		want       string
		wantErr    bool
	}{
		{
			name:       "with tag",
			catalogRef: "ghcr.io/org/cat:v2",
			pkgName:    "go",
			want:       "ghcr.io/org/cat/archives/go",
		},
		{
			name:       "with digest",
			catalogRef: "ghcr.io/org/cat@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			pkgName:    "go",
			want:       "ghcr.io/org/cat/archives/go",
		},
		{
			name:       "no slash",
			catalogRef: "cat:v2",
			pkgName:    "go",
			wantErr:    true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ocix.ArchivesRef(c.catalogRef, c.pkgName)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ArchivesRef(%q): want error, got nil", c.catalogRef)
				}
				return
			}
			if err != nil {
				t.Fatalf("ArchivesRef(%q): unexpected error: %v", c.catalogRef, err)
			}
			if got != c.want {
				t.Fatalf("ArchivesRef(%q) = %q, want %q", c.catalogRef, got, c.want)
			}
		})
	}
}

func TestResolveArchiveTagIsFetchFree(t *testing.T) {
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
	got, err := ocix.ResolveArchiveTag(ctx, counting, "v1.0.0")
	if err != nil {
		t.Fatalf("ResolveArchiveTag: %v", err)
	}
	if got.Digest != want.Digest {
		t.Fatalf("digest = %s, want %s", got.Digest, want.Digest)
	}
	if counting.fetches != 0 {
		t.Fatalf("Fetch calls = %d, want 0: a resolve-only presence check must fetch no blobs", counting.fetches)
	}
}

func TestResolveArchiveTagMissing(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeArchive(t, store, "v1.0.0", map[string][]byte{"linux/amd64": []byte("x")})

	_, err = ocix.ResolveArchiveTag(ctx, store, "v9.9.9")
	if !errors.Is(err, ocix.ErrArchiveNotFound) {
		t.Fatalf("want ErrArchiveNotFound, got %v", err)
	}
}

func TestArchiveLayerDigestMatchesContentWithoutFetchingIt(t *testing.T) {
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
	got, err := ocix.ArchiveLayerDigest(ctx, counting, "v1.0.0", spec.Platform{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("ArchiveLayerDigest: %v", err)
	}
	if got != want {
		t.Fatalf("digest = %s, want %s", got, want)
	}
	if counting.fetchedBytes >= int64(len(payload)) {
		t.Fatalf("fetched %d bytes, which includes the %d-byte archive layer: the layer must never be fetched", counting.fetchedBytes, len(payload))
	}
}
