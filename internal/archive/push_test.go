package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"

	"github.com/vi-dev/nem/internal/spec"
)

func TestPushMergesPlatformsAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	darwin := spec.Platform{OS: "darwin", Arch: "arm64"}
	linux := spec.Platform{OS: "linux", Arch: "amd64"}
	dArc := []byte("darwin-archive-bytes")
	lArc := []byte("linux-archive-bytes")

	if _, pushed, err := Push(ctx, store, "v1", darwin, BytesBlob(dArc), false); err != nil || !pushed {
		t.Fatalf("push darwin: pushed=%v err=%v", pushed, err)
	}
	if _, pushed, err := Push(ctx, store, "v1", linux, BytesBlob(lArc), false); err != nil || !pushed {
		t.Fatalf("push linux: pushed=%v err=%v", pushed, err)
	}

	if _, pushed, err := Push(ctx, store, "v1", darwin, BytesBlob(dArc), false); err != nil || pushed {
		t.Fatalf("re-push darwin unchanged: pushed=%v (want false) err=%v", pushed, err)
	}

	if _, pushed, err := Push(ctx, store, "v1", darwin, BytesBlob(dArc), true); err != nil || !pushed {
		t.Fatalf("force re-push: pushed=%v (want true) err=%v", pushed, err)
	}

	for plat, want := range map[spec.Platform][]byte{darwin: dArc, linux: lArc} {
		p, err := Pull(ctx, store, "v1", plat, t.TempDir())
		if err != nil {
			t.Fatalf("pull %s: %v", plat, err)
		}
		got, _ := os.ReadFile(p)
		if !bytes.Equal(got, want) {
			t.Fatalf("pull %s = %q, want %q", plat, got, want)
		}
	}
}

func writeTempArchiveFile(t *testing.T, data []byte) (path, sha256Hex string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "archive")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp archive: %v", err)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

func TestStageAndCommitRoundTrips(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	payload := []byte("streamed-archive-bytes")
	path, sha := writeTempArchiveFile(t, payload)
	plat := spec.Platform{OS: "linux", Arch: "amd64"}

	entry, err := Stage(ctx, store, plat, FileBlob(path, sha, int64(len(payload))))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if entry.Platform == nil || entry.Platform.OS != "linux" || entry.Platform.Architecture != "amd64" {
		t.Fatalf("entry platform = %+v", entry.Platform)
	}
	if _, err := Commit(ctx, store, "v1.0.0", []ocispec.Descriptor{entry}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	pulled, err := Pull(ctx, store, "v1.0.0", plat, t.TempDir())
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	got, err := os.ReadFile(pulled)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("pulled bytes = %q, want %q", got, payload)
	}
}

func TestStageAndCommitHealsChangedContent(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	plat := spec.Platform{OS: "linux", Arch: "amd64"}

	oldPayload := []byte("stale-payload")
	oldPath, oldSha := writeTempArchiveFile(t, oldPayload)
	oldEntry, err := Stage(ctx, store, plat, FileBlob(oldPath, oldSha, int64(len(oldPayload))))
	if err != nil {
		t.Fatalf("initial publish: %v", err)
	}
	if _, err := Commit(ctx, store, "v1.0.0", []ocispec.Descriptor{oldEntry}); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	newPayload := []byte("healed-payload-with-different-content")
	newPath, newSha := writeTempArchiveFile(t, newPayload)
	newEntry, err := Stage(ctx, store, plat, FileBlob(newPath, newSha, int64(len(newPayload))))
	if err != nil {
		t.Fatalf("heal publish: %v", err)
	}
	if _, err := Commit(ctx, store, "v1.0.0", []ocispec.Descriptor{newEntry}); err != nil {
		t.Fatalf("heal commit: %v", err)
	}

	pulled, err := Pull(ctx, store, "v1.0.0", plat, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(pulled)
	if !bytes.Equal(got, newPayload) {
		t.Fatalf("healed bytes = %q, want %q", got, newPayload)
	}
}

func TestStageMergesWithPushPlatforms(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	darwin := spec.Platform{OS: "darwin", Arch: "arm64"}
	linux := spec.Platform{OS: "linux", Arch: "amd64"}
	dArc := []byte("darwin-bytes")

	if _, _, err := Push(ctx, store, "v1", darwin, BytesBlob(dArc), false); err != nil {
		t.Fatalf("Push darwin: %v", err)
	}
	lArc := []byte("linux-bytes-from-a-file")
	path, sha := writeTempArchiveFile(t, lArc)
	entry, err := Stage(ctx, store, linux, FileBlob(path, sha, int64(len(lArc))))
	if err != nil {
		t.Fatalf("Stage linux: %v", err)
	}
	if _, err := Commit(ctx, store, "v1", []ocispec.Descriptor{entry}); err != nil {
		t.Fatalf("Commit linux: %v", err)
	}

	plats, err := ResolvePlatforms(ctx, store, "v1")
	if err != nil {
		t.Fatalf("Platforms: %v", err)
	}
	if len(plats) != 2 {
		t.Fatalf("platforms = %v, want both darwin and linux merged", plats)
	}
	for plat, want := range map[spec.Platform][]byte{darwin: dArc, linux: lArc} {
		p, err := Pull(ctx, store, "v1", plat, t.TempDir())
		if err != nil {
			t.Fatalf("pull %s: %v", plat, err)
		}
		got, _ := os.ReadFile(p)
		if !bytes.Equal(got, want) {
			t.Fatalf("pull %s = %q, want %q", plat, got, want)
		}
	}
}

type indexPushCountingTarget struct {
	oras.Target
	indexPushes atomic.Int64
	tags        atomic.Int64
}

func (c *indexPushCountingTarget) Push(ctx context.Context, d ocispec.Descriptor, r io.Reader) error {
	if d.MediaType == ocispec.MediaTypeImageIndex {
		c.indexPushes.Add(1)
	}
	return c.Target.Push(ctx, d, r)
}

func (c *indexPushCountingTarget) Tag(ctx context.Context, d ocispec.Descriptor, ref string) error {
	c.tags.Add(1)
	return c.Target.Tag(ctx, d, ref)
}

func TestCommitPushesIndexOnceForABatch(t *testing.T) {
	ctx := context.Background()
	counted := &indexPushCountingTarget{Target: memory.New()}
	plats := []spec.Platform{
		{OS: "darwin", Arch: "arm64"}, {OS: "darwin", Arch: "amd64"},
		{OS: "linux", Arch: "arm64"}, {OS: "linux", Arch: "amd64"},
	}

	var entries []ocispec.Descriptor
	for _, plat := range plats {
		payload := []byte("payload-" + plat.String())
		path, sha := writeTempArchiveFile(t, payload)
		entry, err := Stage(ctx, counted, plat, FileBlob(path, sha, int64(len(payload))))
		if err != nil {
			t.Fatalf("Stage %s: %v", plat, err)
		}
		entries = append(entries, entry)
	}
	if got := counted.indexPushes.Load(); got != 0 {
		t.Fatalf("index pushes after per-platform publish alone = %d, want 0 (no index touched yet)", got)
	}

	if _, err := Commit(ctx, counted, "v1.0.0", entries); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got := counted.indexPushes.Load(); got != 1 {
		t.Fatalf("index pushes = %d, want exactly 1 for a %d-platform batch", got, len(plats))
	}
	if got := counted.tags.Load(); got != 1 {
		t.Fatalf("tag calls = %d, want exactly 1 for a %d-platform batch", got, len(plats))
	}

	got, err := ResolvePlatforms(ctx, counted, "v1.0.0")
	if err != nil {
		t.Fatalf("Platforms: %v", err)
	}
	if len(got) != len(plats) {
		t.Fatalf("platforms in committed index = %v, want all %d", got, len(plats))
	}
}

func TestCommitSkipsWhenMergedResultUnchanged(t *testing.T) {
	ctx := context.Background()
	counted := &indexPushCountingTarget{Target: memory.New()}
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	payload := []byte("stable-payload")
	path, sha := writeTempArchiveFile(t, payload)
	entry, err := Stage(ctx, counted, plat, FileBlob(path, sha, int64(len(payload))))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if _, err := Commit(ctx, counted, "v1.0.0", []ocispec.Descriptor{entry}); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if got := counted.indexPushes.Load(); got != 1 {
		t.Fatalf("index pushes after first commit = %d, want 1", got)
	}

	if _, err := Commit(ctx, counted, "v1.0.0", []ocispec.Descriptor{entry}); err != nil {
		t.Fatalf("second (unchanged) commit: %v", err)
	}
	if got := counted.indexPushes.Load(); got != 1 {
		t.Fatalf("index pushes after unchanged re-commit = %d, want still 1 (skip)", got)
	}
	if got := counted.tags.Load(); got != 1 {
		t.Fatalf("tag calls after unchanged re-commit = %d, want still 1 (skip)", got)
	}
}

type recordingPushTarget struct {
	oras.Target
	archivePushes int
	sawFileReader bool
}

func (r *recordingPushTarget) Push(ctx context.Context, d ocispec.Descriptor, content io.Reader) error {
	if d.MediaType == MediaType {
		r.archivePushes++
		if _, ok := content.(*os.File); ok {
			r.sawFileReader = true
		}
	}
	return r.Target.Push(ctx, d, content)
}

func TestStageStreamsWithoutBuffering(t *testing.T) {
	ctx := context.Background()
	rec := &recordingPushTarget{Target: memory.New()}
	payload := bytes.Repeat([]byte("x"), 5*1024*1024)
	path, sha := writeTempArchiveFile(t, payload)

	if _, err := Stage(ctx, rec, spec.Platform{OS: "linux", Arch: "amd64"}, FileBlob(path, sha, int64(len(payload)))); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if rec.archivePushes != 1 {
		t.Fatalf("archive layer pushes = %d, want 1", rec.archivePushes)
	}
	if !rec.sawFileReader {
		t.Fatal("archive layer was not streamed from an *os.File — it was buffered before Push")
	}
}

type flakyArchiveLayerTarget struct {
	oras.Target
	mu     sync.Mutex
	failed bool
	got    []byte
}

func (f *flakyArchiveLayerTarget) Exists(ctx context.Context, d ocispec.Descriptor) (bool, error) {
	if d.MediaType == MediaType {
		return false, nil
	}
	return f.Target.Exists(ctx, d)
}

func (f *flakyArchiveLayerTarget) Push(ctx context.Context, d ocispec.Descriptor, r io.Reader) error {
	if d.MediaType != MediaType {
		return f.Target.Push(ctx, d, r)
	}
	f.mu.Lock()
	first := !f.failed
	f.failed = true
	f.mu.Unlock()
	if first {
		io.CopyN(io.Discard, r, 4)
		return errors.New("simulated dropped connection")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.got = data
	return f.Target.Push(ctx, d, bytes.NewReader(data))
}

func TestStageReopensFileOnRetry(t *testing.T) {
	ctx := context.Background()
	flaky := &flakyArchiveLayerTarget{Target: memory.New()}
	payload := bytes.Repeat([]byte("retry-me-"), 1000)
	path, sha := writeTempArchiveFile(t, payload)

	if _, err := Stage(ctx, flaky, spec.Platform{OS: "linux", Arch: "amd64"}, FileBlob(path, sha, int64(len(payload)))); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if !bytes.Equal(flaky.got, payload) {
		t.Fatal("retried push received a partially-consumed reader instead of a freshly reopened file")
	}
}

type alwaysFailArchiveLayerTarget struct {
	oras.Target
}

func (alwaysFailArchiveLayerTarget) Exists(_ context.Context, d ocispec.Descriptor) (bool, error) {
	return d.MediaType != MediaType, nil
}

func (alwaysFailArchiveLayerTarget) Push(_ context.Context, d ocispec.Descriptor, r io.Reader) error {
	if d.MediaType == MediaType {
		io.Copy(io.Discard, r)
		return errors.New("simulated permanent failure")
	}
	return errdef.ErrUnsupported
}

func (alwaysFailArchiveLayerTarget) Tag(_ context.Context, _ ocispec.Descriptor, _ string) error {
	return errdef.ErrUnsupported
}

func (alwaysFailArchiveLayerTarget) Resolve(_ context.Context, _ string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, errdef.ErrNotFound
}

func (alwaysFailArchiveLayerTarget) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	return nil, errdef.ErrNotFound
}

func TestStageFailsAfterRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	payload := []byte("x")
	path, sha := writeTempArchiveFile(t, payload)

	entry, err := Stage(ctx, alwaysFailArchiveLayerTarget{}, spec.Platform{OS: "linux", Arch: "amd64"}, FileBlob(path, sha, int64(len(payload))))
	if err == nil {
		t.Fatal("want error once every attempt fails, got nil")
	}
	if !strings.Contains(err.Error(), "simulated permanent failure") {
		t.Fatalf("err = %v, want it to surface the archive layer push's own failure", err)
	}
	if entry.Digest != "" {
		t.Fatalf("entry = %+v, want a zero-value descriptor on failure", entry)
	}
}

type swallowTagTarget struct {
	oras.Target
}

func (swallowTagTarget) Tag(context.Context, ocispec.Descriptor, string) error { return nil }

func TestPushFailsWhenCommitDoesNotLand(t *testing.T) {
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	target := swallowTagTarget{Target: memory.New()}
	if _, _, err := Push(context.Background(), target, "v1", plat, BytesBlob([]byte("x")), false); err == nil {
		t.Fatal("push must fail when the committed index cannot be resolved")
	}
}

func TestPushFailsWhenIndexHoldsAnotherManifest(t *testing.T) {
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	target := memory.New()
	if _, pushed, err := Push(context.Background(), target, "v1", plat, BytesBlob([]byte("old")), false); err != nil || !pushed {
		t.Fatalf("seed push: %v", err)
	}
	if _, _, err := Push(context.Background(), swallowTagTarget{Target: target}, "v1", plat, BytesBlob([]byte("new")), false); err == nil {
		t.Fatal("push must fail when the index does not hold the committed manifest")
	}
}

type failOnceTarget struct {
	oras.Target
	failed atomic.Bool
}

func (f *failOnceTarget) Push(ctx context.Context, desc ocispec.Descriptor, r io.Reader) error {
	if !f.failed.Swap(true) {
		return errors.New("transient blob push failure")
	}
	return f.Target.Push(ctx, desc, r)
}

func TestPushRetriesTransientBlobPushFailure(t *testing.T) {
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	target := &failOnceTarget{Target: memory.New()}
	if _, pushed, err := Push(context.Background(), target, "v1", plat, BytesBlob([]byte("x")), false); err != nil || !pushed {
		t.Fatalf("Push after transient failure: pushed=%v err=%v", pushed, err)
	}
}

func TestCommitFailsWhenCommitDoesNotLand(t *testing.T) {
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	seed := memory.New()
	entry, pushed, err := Push(context.Background(), seed, "v1", plat, BytesBlob([]byte("x")), false)
	if err != nil || !pushed {
		t.Fatalf("seed push: %v", err)
	}
	target := swallowTagTarget{Target: memory.New()}
	if _, err := Commit(context.Background(), target, "v1", []ocispec.Descriptor{entry}); err == nil {
		t.Fatal("commit must fail when the tagged index cannot be resolved")
	}
}

func countingBlob(data []byte) (Blob, *atomic.Int64) {
	var opens atomic.Int64
	b := BytesBlob(data)
	inner := b.Open
	b.Open = func() (io.ReadCloser, error) {
		opens.Add(1)
		return inner()
	}
	return b, &opens
}

func TestPushNoOpNeverOpensBlob(t *testing.T) {
	ctx := context.Background()
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	target := memory.New()
	payload := []byte("idempotent payload")
	if _, pushed, err := Push(ctx, target, "v1", plat, BytesBlob(payload), false); err != nil || !pushed {
		t.Fatalf("seed push: pushed=%v err=%v", pushed, err)
	}

	blob, opens := countingBlob(payload)
	_, pushed, err := Push(ctx, target, "v1", plat, blob, false)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if pushed {
		t.Fatal("second push of identical content must be a no-op")
	}
	if got := opens.Load(); got != 0 {
		t.Fatalf("no-op push opened the blob %d times, want 0", got)
	}
}

type failLayerOnceTarget struct {
	oras.Target
	failed atomic.Bool
}

func (f *failLayerOnceTarget) Push(ctx context.Context, desc ocispec.Descriptor, r io.Reader) error {
	if desc.MediaType == MediaType && !f.failed.Swap(true) {
		return errors.New("transient layer push failure")
	}
	return f.Target.Push(ctx, desc, r)
}

func TestPushRetryReopensBlob(t *testing.T) {
	ctx := context.Background()
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	target := &failLayerOnceTarget{Target: memory.New()}
	blob, opens := countingBlob([]byte("retry payload"))
	if _, pushed, err := Push(ctx, target, "v1", plat, blob, false); err != nil || !pushed {
		t.Fatalf("push after transient failure: pushed=%v err=%v", pushed, err)
	}
	if got := opens.Load(); got < 2 {
		t.Fatalf("blob opened %d times, want at least 2 across retry attempts", got)
	}
}
