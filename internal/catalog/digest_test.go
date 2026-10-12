package catalog

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

type fetchCounter struct {
	oras.ReadOnlyTarget
	mu      sync.Mutex
	fetches map[string]int
}

func (c *fetchCounter) Fetch(ctx context.Context, d ocispec.Descriptor) (io.ReadCloser, error) {
	c.mu.Lock()
	c.fetches[d.Digest.String()]++
	c.mu.Unlock()
	return c.ReadOnlyTarget.Fetch(ctx, d)
}

func (c *fetchCounter) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, v := range c.fetches {
		n += v
	}
	return n
}

func TestDirDigestMatchesPackageManifest(t *testing.T) {
	dir := t.TempDir()
	testx.WriteFile(t, filepath.Join(dir, "pkgs", "go", "pkg.yaml"), ociGoYAML)
	_, want, err := ocix.PackageManifest(spec.Files{Pkg: []byte(ociGoYAML)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewDir(dir).Digest(context.Background(), "go")
	if err != nil || got != want.Digest.String() {
		t.Fatalf("Digest = %q, %v; want %s", got, err, want.Digest)
	}
	var nf *PackageNotFoundError
	if _, err := NewDir(dir).Digest(context.Background(), "absent"); !errors.As(err, &nf) {
		t.Fatalf("want PackageNotFoundError, got %v", err)
	}
}

func TestOCIDigestReadsOnlyTheIndex(t *testing.T) {
	ctx := context.Background()
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{{Name: "go", Latest: "v1.26.5", YAML: []byte(ociGoYAML)}}, "2")
	counter := &fetchCounter{ReadOnlyTarget: src, fetches: map[string]int{}}
	store, err := ocix.OpenStore(ctx, counter, "v2")
	if err != nil {
		t.Fatal(err)
	}
	cat := NewOCI("example.com/cat:v2", store)
	before := counter.total()

	_, want, err := ocix.PackageManifest(spec.Files{Pkg: []byte(ociGoYAML)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := cat.Digest(ctx, "go")
	if err != nil || got != want.Digest.String() {
		t.Fatalf("Digest = %q, %v; want %s", got, err, want.Digest)
	}
	if n := counter.total() - before; n != 0 {
		t.Fatalf("Digest fetched %d blobs; the index already carries the manifest digest", n)
	}
	var nf *PackageNotFoundError
	if _, err := cat.Digest(ctx, "absent"); !errors.As(err, &nf) {
		t.Fatalf("want PackageNotFoundError, got %v", err)
	}
}
