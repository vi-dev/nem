package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

var testPlat = spec.Platform{OS: "linux", Arch: "amd64"}

func urlPkg(url, sha string) *spec.Package {
	return &spec.Package{
		Name:     "go",
		Artifact: spec.Artifact{URL: url},
		Versions: []spec.VersionEntry{{
			Version: "v1.2.3",
			Sha256:  map[string]string{testPlat.String(): sha},
		}},
	}
}

func ociPkg(ociRef string) *spec.Package {
	return &spec.Package{
		Name:     "go",
		Artifact: spec.Artifact{OCI: ociRef},
		Versions: []spec.VersionEntry{{Version: "v1.2.3"}},
	}
}

type fakeStore struct {
	target oras.ReadOnlyTarget
	err    error
}

func (f fakeStore) Open(context.Context, string) (oras.ReadOnlyTarget, error) {
	return f.target, f.err
}

type resolveErrorTarget struct{ err error }

func (r resolveErrorTarget) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return nil, r.err
}

func (r resolveErrorTarget) Exists(context.Context, ocispec.Descriptor) (bool, error) {
	return false, r.err
}

func (r resolveErrorTarget) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, r.err
}

func stagedStore(t *testing.T, name, tag string, plat spec.Platform, blob []byte) fakeStore {
	t.Helper()
	mem := memory.New()
	if _, _, err := archive.Push(context.Background(), mem, tag, plat, archive.BytesBlob(blob), false); err != nil {
		t.Fatal(err)
	}
	return fakeStore{target: mem}
}

func hitCountingServer(t *testing.T, body []byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestAcquireRegistryHitSkipsUpstreamButVerifiesSha256(t *testing.T) {
	body := []byte("upstream bytes, never fetched")
	srv, hits := hitCountingServer(t, body)

	wrongBytes := []byte("wrong archive bytes from registry")
	dir := t.TempDir()

	store := stagedStore(t, "go", "v1.2.3", testPlat, wrongBytes)

	pkg := urlPkg(srv.URL, strings.Repeat("a", 64))
	src := Source{Archives: []archive.Store{store}}

	_, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if _, ok := errors.AsType[*ChecksumMismatchError](err); !ok {
		t.Fatalf("want ChecksumMismatchError, got %v", err)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream hit %d times, want 0 (registry hit must skip upstream)", hits.Load())
	}
}

func TestAcquirePrefersEarlierStore(t *testing.T) {
	blob := []byte("layer-one")
	sum := sha256.Sum256(blob)
	wantSHA := hex.EncodeToString(sum[:])

	first := stagedStore(t, "go", "v1.2.3", testPlat, blob)
	second := fakeStore{err: fmt.Errorf("wrap: %w", archive.ErrNotFound)}

	pkg := urlPkg("https://example.invalid/must-not-be-fetched", wantSHA)
	src := Source{Archives: []archive.Store{first, second}}
	dir := t.TempDir()

	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(blob) {
		t.Errorf("bytes = %q, want %q", got, blob)
	}
}

func TestAcquireFallsThroughNotFoundToUpstream(t *testing.T) {
	body := []byte("upstream fallback bytes")
	sum := sha256.Sum256(body)
	wantSHA := hex.EncodeToString(sum[:])

	srv := testx.NewFileServer(t)
	srv.Set("/pkg", body)

	src := Source{Archives: []archive.Store{fakeStore{err: archive.ErrNotFound}}}
	pkg := urlPkg(srv.URL+"/pkg", wantSHA)
	dir := t.TempDir()

	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer os.Remove(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("downloaded bytes = %q, want %q", got, body)
	}
	if srv.Hits() != 1 {
		t.Errorf("upstream hit %d times, want 1", srv.Hits())
	}
}

func TestAcquireNonNotFoundOpenErrorAbortsWithoutFallback(t *testing.T) {
	body := []byte("must never be fetched")
	srv, hits := hitCountingServer(t, body)

	openErr := errors.New("401 unauthorized")
	src := Source{Archives: []archive.Store{fakeStore{err: openErr}}}

	pkg := urlPkg(srv.URL, strings.Repeat("a", 64))
	dir := t.TempDir()

	_, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, openErr) {
		t.Errorf("want wrapped openErr, got %v", err)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream hit %d times, want 0 (non-not-found open error must not fall back)", hits.Load())
	}
}

func TestAcquireNonNotFoundPullErrorAbortsWithoutFallback(t *testing.T) {
	body := []byte("must never be fetched")
	srv, hits := hitCountingServer(t, body)

	pullErr := errors.New("500 internal server error")
	first := fakeStore{target: resolveErrorTarget{err: pullErr}}
	second := fakeStore{err: errors.New("second store must not be tried after a non-not-found pull error")}
	src := Source{Archives: []archive.Store{first, second}}

	pkg := urlPkg(srv.URL, strings.Repeat("a", 64))
	dir := t.TempDir()

	_, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, pullErr) {
		t.Errorf("want wrapped pullErr, got %v", err)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream hit %d times, want 0 (non-not-found pull error must not fall back)", hits.Load())
	}
}

func TestAcquireOCIArtifactPullSuccess(t *testing.T) {
	dir := t.TempDir()
	blob := []byte("archive bytes")
	store := stagedStore(t, "go", "v1.2.3", testPlat, blob)

	pkg := ociPkg(":{{.Version}}")
	src := Source{Archives: []archive.Store{store}}

	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(blob) {
		t.Errorf("bytes = %q, want %q", got, blob)
	}
}

func TestAcquireOCIArtifactPullNotFoundErrorsWithoutFallback(t *testing.T) {
	pkg := ociPkg(":{{.Version}}")
	src := Source{Archives: []archive.Store{fakeStore{err: archive.ErrNotFound}}}
	dir := t.TempDir()

	_, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if !errors.Is(err, archive.ErrNotFound) {
		t.Fatalf("want ErrArchiveNotFound, got %v", err)
	}
}

func TestAcquireDirCatalogGoesStraightUpstream(t *testing.T) {
	body := []byte("dir catalog upstream bytes")
	sum := sha256.Sum256(body)
	wantSHA := hex.EncodeToString(sum[:])
	srv, hits := hitCountingServer(t, body)

	pkg := urlPkg(srv.URL, wantSHA)
	src := Source{}
	dir := t.TempDir()

	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer os.Remove(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("downloaded bytes = %q, want %q", got, body)
	}
	if hits.Load() != 1 {
		t.Errorf("upstream hit %d times, want 1", hits.Load())
	}
}

func TestAcquireRegistrySuccessAcceptsWithoutUpstream(t *testing.T) {
	body := []byte("must never be fetched")
	srv, hits := hitCountingServer(t, body)

	archiveBytes := []byte("correct archive bytes from registry")
	sum := sha256.Sum256(archiveBytes)
	wantSHA := hex.EncodeToString(sum[:])
	dir := t.TempDir()

	store := stagedStore(t, "go", "v1.2.3", testPlat, archiveBytes)

	pkg := urlPkg(srv.URL, wantSHA)
	src := Source{Archives: []archive.Store{store}}

	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(archiveBytes) {
		t.Errorf("bytes = %q, want %q", got, archiveBytes)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream hit %d times, want 0 (registry success must not fall back)", hits.Load())
	}
}

func TestAcquireMissingSha256Errors(t *testing.T) {
	pkg := &spec.Package{
		Name:     "go",
		Artifact: spec.Artifact{URL: "https://example.invalid/x"},
		Versions: []spec.VersionEntry{{Version: "v1.2.3", Sha256: map[string]string{}}},
	}
	dir := t.TempDir()

	_, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, Source{}, dir, nil)
	if err == nil {
		t.Fatal("want error for missing pinned sha256")
	}
}

func TestAcquireAbsoluteOCIRefUsesRemoteByRef(t *testing.T) {
	dir := t.TempDir()
	archivePath := dir + "/prewritten-absolute"
	if err := os.WriteFile(archivePath, []byte("absolute ref archive"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	origRemoteByRef := remoteByRef
	t.Cleanup(func() { remoteByRef = origRemoteByRef })
	var gotRef string
	remoteByRef = func(ctx context.Context, ref string, plat spec.Platform, dir string) (string, error) {
		gotRef = ref
		return archivePath, nil
	}

	pkg := ociPkg("ghcr.io/other/repo:{{.Version}}")
	poisoned := Source{Archives: []archive.Store{fakeStore{err: errors.New("catalog archives must not be consulted for an absolute ref")}}}
	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, poisoned, dir, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if path != archivePath {
		t.Errorf("path = %q, want %q", path, archivePath)
	}
	if gotRef != "ghcr.io/other/repo:v1.2.3" {
		t.Errorf("ref = %q", gotRef)
	}
}

func TestAcquireOCIRefTemplateErrorPropagates(t *testing.T) {
	pkg := ociPkg(":{{.Bogus}}")
	dir := t.TempDir()

	_, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, Source{}, dir, nil)
	if err == nil {
		t.Fatal("want error for unknown template key")
	}
}

func TestVerifyFileOpenErrorWraps(t *testing.T) {
	meta := Meta{Name: "go", Version: "v1.2.3", Platform: testPlat}
	err := VerifyFile("/nonexistent/path/for/test", strings.Repeat("a", 64), meta)
	if err == nil {
		t.Fatal("want error for nonexistent file")
	}
	if _, ok := errors.AsType[*ChecksumMismatchError](err); ok {
		t.Fatal("open failure must not be reported as ChecksumMismatchError")
	}
}

func TestAcquireOCIRelativeRefWithoutStoresErrors(t *testing.T) {
	pkg := ociPkg(":{{.Version}}")
	src := Source{}
	dir := t.TempDir()

	_, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, dir, nil)
	if err == nil {
		t.Fatal("want error for relative oci ref without catalog")
	}
	want := "relative oci ref requires an oci catalog"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestVerifyFileMatch(t *testing.T) {
	dir := t.TempDir()
	body := []byte("verify me")
	sum := sha256.Sum256(body)
	wantSHA := hex.EncodeToString(sum[:])

	path := dir + "/f"
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	meta := Meta{Name: "go", Version: "v1.2.3", Platform: testPlat}
	if err := VerifyFile(path, wantSHA, meta); err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
}

func TestVerifyFileMismatch(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/f"
	if err := os.WriteFile(path, []byte("actual bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	meta := Meta{Name: "go", Version: "v1.2.3", Platform: testPlat}
	err := VerifyFile(path, strings.Repeat("a", 64), meta)
	var cme *ChecksumMismatchError
	if !errors.As(err, &cme) {
		t.Fatalf("want ChecksumMismatchError, got %v", err)
	}
	if cme.Name != "go" || cme.Version != "v1.2.3" {
		t.Errorf("Name/Version = %q/%q", cme.Name, cme.Version)
	}
}

func TestAcquireRelativeOCIRefPrefersLocalArchives(t *testing.T) {
	s := archive.NewDir(t.TempDir())
	target, err := s.OpenRW(context.Background(), "tool")
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	if _, _, err := archive.Push(context.Background(), target, "v1.2.3", testPlat, archive.BytesBlob([]byte("local bytes")), false); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	mustNotRun := fakeStore{err: errors.New("catalog pull must not run on a local hit")}

	pkg := ociPkg(":{{.Version}}")
	pkg.Name = "tool"
	src := Source{Archives: []archive.Store{s, mustNotRun}}
	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "local bytes" {
		t.Fatalf("got %q", data)
	}
}

func TestAcquireRelativeOCIRefLocalMissFallsBack(t *testing.T) {
	s := archive.NewDir(t.TempDir())
	blob := []byte("catalog bytes")
	fallback := stagedStore(t, "tool", "v1.2.3", testPlat, blob)

	pkg := ociPkg(":{{.Version}}")
	pkg.Name = "tool"
	src := Source{Archives: []archive.Store{s, fallback}}
	path, err := Acquire(context.Background(), pkg, "v1.2.3", testPlat, src, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(blob) {
		t.Fatalf("fallback store must be used after a local miss: got %q, want %q", got, blob)
	}
}
