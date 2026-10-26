package publish

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/vi-dev/clix/console/consoletest"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func swapOpenTarget(f func(context.Context, string) (oras.Target, error)) (restore func()) {
	return SetTargetOpener(f)
}

func swapNow(f func() time.Time) (restore func()) {
	prev := nowFunc
	nowFunc = f
	return func() { nowFunc = prev }
}

func newStore(t *testing.T) (*oci.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := oci.New(dir)
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	return s, dir
}

func assertTagResolves(t *testing.T, store oras.ReadOnlyTarget, tag string) {
	t.Helper()
	if _, err := store.Resolve(context.Background(), tag); err != nil {
		t.Fatalf("resolve tag %s: %v", tag, err)
	}
}

func countBlobs(t *testing.T, storeDir string) int {
	t.Helper()
	root := filepath.Join(storeDir, "blobs")
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return count
}

type countingTarget struct {
	oras.Target
	mu     sync.Mutex
	exists map[string]int
}

func newCountingTarget(t oras.Target) *countingTarget {
	return &countingTarget{Target: t, exists: map[string]int{}}
}

func (c *countingTarget) Exists(ctx context.Context, d ocispec.Descriptor) (bool, error) {
	c.mu.Lock()
	c.exists[d.Digest.String()]++
	c.mu.Unlock()
	return c.Target.Exists(ctx, d)
}

func (c *countingTarget) existsCount(digest string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exists[digest]
}

func TestPublishHappyPathAndIdempotency(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg, "kubectl": pkgNamed("kubectl")})
	store, storeDir := newStore(t)

	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
	defer swapNow(func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })()

	if _, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"v2"}}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	assertTagResolves(t, store, "v2")
	assertTagResolves(t, store, "v4.20260102T030405Z")

	blobsAfterFirst := countBlobs(t, storeDir)

	if _, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"v2"}}); err != nil {
		t.Fatal(err)
	}

	if got := countBlobs(t, storeDir); got != blobsAfterFirst {
		t.Fatalf("re-publish pushed new blobs: %d → %d", blobsAfterFirst, got)
	}
	assertTagResolves(t, store, "v2")
	assertTagResolves(t, store, "v4.20260102T030405Z")
}

func TestPublishRejectsEmptyCatalog(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := t.TempDir()
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) {
		t.Fatal("openTarget must not be called for an empty catalog")
		return nil, nil
	})()

	_, err := Publish(ctx, dir, "example.com/cat", Options{})
	if err == nil || !strings.Contains(err.Error(), "no package manifests under") {
		t.Fatalf("err = %v, want no-package error", err)
	}
}

func TestPublishReportsResult(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
	defer swapNow(func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })()

	res, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"v2"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 1 || res.Unchanged != 0 {
		t.Fatalf("counts = %d/%d, want 1/0", res.Pushed, res.Unchanged)
	}
	if len(res.Packages) != 1 || res.Packages[0].Name != "go" || res.Packages[0].Status != StatusNew {
		t.Fatalf("packages = %+v", res.Packages)
	}
	if want := []string{"v2", "v4.20260102T030405Z"}; !slices.Equal(res.Tags, want) {
		t.Fatalf("tags = %v, want %v", res.Tags, want)
	}

	res, err = Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"v2"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 0 || res.Unchanged != 1 || res.Packages[0].Status != StatusUnchanged {
		t.Fatalf("second run = %+v", res)
	}
}

func TestPublishForceDisablesSkip(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	store, _ := newStore(t)
	ct := newCountingTarget(store)

	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return ct, nil })()
	defer swapNow(func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })()

	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}

	layerDigest := content.NewDescriptorFromBytes(ocix.MediaTypePkgV3, []byte(validGoPkg)).Digest.String()

	beforeSkip := ct.existsCount(layerDigest)
	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err != nil {
		t.Fatalf("skip-unchanged publish: %v", err)
	}
	if got := ct.existsCount(layerDigest) - beforeSkip; got != 0 {
		t.Fatalf("skip-unchanged path must never check the layer blob, got %d new Exists calls", got)
	}

	beforeForce := ct.existsCount(layerDigest)
	if _, err := Publish(ctx, dir, "example.com/cat", Options{Force: true}); err != nil {
		t.Fatalf("force publish: %v", err)
	}
	if got := ct.existsCount(layerDigest) - beforeForce; got != 1 {
		t.Fatalf("force must bypass the skip decision and re-attempt the push, got %d new layer Exists calls", got)
	}
}

func TestPublishDefaultsTagsToV3(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	store, _ := newStore(t)

	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
	defer swapNow(func() time.Time { return time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC) })()

	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err != nil {
		t.Fatal(err)
	}
	assertTagResolves(t, store, "v4")
	assertTagResolves(t, store, "v4.20260304T050607Z")
}

func TestPublishRejectsTaggedRef(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) {
		t.Fatal("openTarget must not be called for an invalid ref")
		return nil, nil
	})()

	_, err := Publish(ctx, dir, "example.com/cat:v2", Options{})
	if err == nil {
		t.Fatal("expected ValidateBaseRef to reject a tagged ref")
	}
	if !strings.Contains(err.Error(), "bare repository ref") {
		t.Fatalf("error should name the bare-ref requirement, got: %v", err)
	}
}

func TestPublishSingleFile(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	store, _ := newStore(t)

	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
	defer swapNow(func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })()

	if _, err := Publish(ctx, filepath.Join(dir, "pkgs", "go", "pkg.yaml"), "example.com/cat", Options{}); err != nil {
		t.Fatal(err)
	}
	assertTagResolves(t, store, "v4")
}

func TestPublishTargetOpenFailureLeavesTagsUntouched(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	wantErr := errFailingOpen{}
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return nil, wantErr })()

	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err == nil {
		t.Fatal("expected the target-open failure to surface")
	}
}

type errFailingOpen struct{}

func (errFailingOpen) Error() string { return "open failed" }

type failOnManifestPushTarget struct {
	oras.Target
}

func (f failOnManifestPushTarget) Push(ctx context.Context, d ocispec.Descriptor, r io.Reader) error {
	if d.MediaType == ocispec.MediaTypeImageManifest {
		return errors.New("simulated manifest push failure")
	}
	return f.Target.Push(ctx, d, r)
}

func TestPublishMidPushFailureLeavesTagsUntouched(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	store, _ := newStore(t)

	dirA := writeCatalog(t, map[string]string{"go": validGoPkg, "kubectl": pkgNamed("kubectl")})
	func() {
		defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
		defer swapNow(func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })()
		if _, err := Publish(ctx, dirA, "example.com/cat", Options{}); err != nil {
			t.Fatalf("initial publish: %v", err)
		}
	}()

	indexA, err := store.Resolve(ctx, "v4")
	if err != nil {
		t.Fatalf("resolve v4 after initial publish: %v", err)
	}

	changedGoPkg := strings.Replace(validGoPkg,
		"description: The Go programming language",
		"description: The Go programming language, updated",
		1)
	dirB := writeCatalog(t, map[string]string{"go": changedGoPkg, "kubectl": pkgNamed("kubectl")})

	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) {
		return failOnManifestPushTarget{Target: store}, nil
	})()

	defer swapNow(func() time.Time { return time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC) })()

	if _, err := Publish(ctx, dirB, "example.com/cat", Options{}); err == nil {
		t.Fatal("expected the mid-push manifest failure to surface")
	}

	indexAfter, err := store.Resolve(ctx, "v4")
	if err != nil {
		t.Fatalf("resolve v4 after failed publish: %v", err)
	}
	if indexAfter.Digest != indexA.Digest {
		t.Fatalf("tag v4 moved despite a mid-push failure: %s -> %s", indexA.Digest, indexAfter.Digest)
	}

	if _, err := store.Resolve(ctx, "v4.20260203T040506Z"); !errors.Is(err, errdef.ErrNotFound) {
		t.Fatalf("the failed publish's release tag must never be created, got %v", err)
	}
}

func TestPublishPushesFrozenRecipeLayers(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := t.TempDir()
	testx.WriteFile(t, filepath.Join(dir, "pkgs", "tool", "pkg.yaml"), "schema: 3\nname: tool\nartifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\nversions: [2.0.0, 1.0.0]\n")
	testx.WriteFile(t, filepath.Join(dir, "pkgs", "tool", "recipe@1.yaml"), "artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {strip: 1}}]\n")
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()

	res, err := Publish(ctx, dir, "example.com/cat", Options{})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.Pushed != 1 || res.Tags[0] != "v4" || !strings.HasPrefix(res.Tags[1], "v4.") {
		t.Fatalf("result: %+v", res)
	}
	s, err := ocix.OpenStore(ctx, store, "v4")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := s.Files(ctx, "tool")
	if err != nil || len(files.Recipes) != 1 || !strings.Contains(string(files.Recipes["recipe@1.yaml"]), "strip: 1") {
		t.Fatalf("files: %+v, %v", files, err)
	}

	again, err := Publish(ctx, dir, "example.com/cat", Options{})
	if err != nil || again.Unchanged != 1 {
		t.Fatalf("second publish must be unchanged: %+v, %v", again, err)
	}
	testx.WriteFile(t, filepath.Join(dir, "pkgs", "tool", "recipe@1.yaml"), "artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {strip: 2}}]\n")
	third, err := Publish(ctx, dir, "example.com/cat", Options{})
	if err != nil || third.Pushed != 1 {
		t.Fatalf("a recipe-only edit must change the package digest: %+v, %v", third, err)
	}
}

func TestPublishUnchangedRepublishMakesNoPerPackageRequests(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg, "kubectl": pkgNamed("kubectl")})
	store, _ := newStore(t)
	ct := newCountingTarget(store)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return ct, nil })()

	if _, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"v2"}}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}

	_, goDesc, err := ocix.PackageManifest(spec.Files{Pkg: []byte(validGoPkg)})
	if err != nil {
		t.Fatal(err)
	}
	before := ct.existsCount(goDesc.Digest.String())

	res, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"v2"}})
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if res.Pushed != 0 || res.Unchanged != 2 {
		t.Fatalf("counts = %d pushed, %d unchanged, want 0/2", res.Pushed, res.Unchanged)
	}
	if got := ct.existsCount(goDesc.Digest.String()) - before; got != 0 {
		t.Fatalf("republish of an unchanged catalog checked the go manifest %d times, want 0 (the v2 index already lists it)", got)
	}
}

func TestPublishFallsBackToManifestChecksWhenTagIsAbsent(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	dir := writeCatalog(t, map[string]string{"go": validGoPkg, "kubectl": pkgNamed("kubectl")})
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()

	if _, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"v2"}}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}
	res, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"other"}})
	if err != nil {
		t.Fatalf("publish under a new tag: %v", err)
	}
	if res.Pushed != 0 || res.Unchanged != 2 {
		t.Fatalf("counts = %d pushed, %d unchanged, want 0/2 (manifests already exist)", res.Pushed, res.Unchanged)
	}
	assertTagResolves(t, store, "other")
}

func TestPublishClassifiesPackagesAgainstTargetIndex(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()

	dirA := writeCatalog(t, map[string]string{"go": validGoPkg, "alpha": pkgNamed("alpha")})
	if _, err := Publish(ctx, dirA, "example.com/cat", Options{}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}

	changedGoPkg := strings.Replace(validGoPkg, "description: The Go programming language", "description: Go, updated", 1)
	dirB := writeCatalog(t, map[string]string{"go": changedGoPkg, "helm": pkgNamed("helm")})
	res, err := Publish(ctx, dirB, "example.com/cat", Options{})
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}

	want := []Package{
		{Name: "alpha", Version: "v1.26.5", Status: StatusRemoved},
		{Name: "go", Version: "v1.26.5", Status: StatusUpdated},
		{Name: "helm", Version: "v1.26.5", Status: StatusNew},
	}
	if !slices.Equal(res.Packages, want) {
		t.Fatalf("packages = %+v, want %+v", res.Packages, want)
	}
	if res.Pushed != 2 || res.Unchanged != 0 || res.Removed != 1 {
		t.Fatalf("counts = %d pushed, %d unchanged, %d removed, want 2/0/1", res.Pushed, res.Unchanged, res.Removed)
	}

	s, err := ocix.OpenStore(ctx, store, "v4")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Files(ctx, "alpha"); err == nil {
		t.Fatal("alpha must be gone from the published index")
	}
}

func TestPublishDryRunClassifiesWithoutWriting(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	store, storeDir := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()

	dirA := writeCatalog(t, map[string]string{"go": validGoPkg})
	if _, err := Publish(ctx, dirA, "example.com/cat", Options{}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}
	blobsBefore := countBlobs(t, storeDir)
	defer swapNow(func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })()

	dirB := writeCatalog(t, map[string]string{"go": validGoPkg, "helm": pkgNamed("helm")})
	res, err := Publish(ctx, dirB, "example.com/cat", Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	want := []Package{
		{Name: "go", Version: "v1.26.5", Status: StatusUnchanged},
		{Name: "helm", Version: "v1.26.5", Status: StatusNew},
	}
	if !slices.Equal(res.Packages, want) {
		t.Fatalf("packages = %+v, want %+v", res.Packages, want)
	}
	if res.Pushed != 1 || res.Unchanged != 1 {
		t.Fatalf("counts = %d pushed, %d unchanged, want 1/1", res.Pushed, res.Unchanged)
	}
	if got := countBlobs(t, storeDir); got != blobsBefore {
		t.Fatalf("dry run wrote blobs: %d → %d", blobsBefore, got)
	}
	if _, err := store.Resolve(ctx, "v4.20260102T030405Z"); !errors.Is(err, errdef.ErrNotFound) {
		t.Fatalf("dry run must not create the release tag, got %v", err)
	}
}

func TestPublishDryRunAgainstAbsentTagMarksEverythingNew(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()

	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	res, err := Publish(ctx, dir, "example.com/cat", Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(res.Packages) != 1 || res.Packages[0].Status != StatusNew || res.Pushed != 1 {
		t.Fatalf("result = %+v", res)
	}
}

func TestPublishNarratesCompareAndPushesAsTasks(t *testing.T) {
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()

	dirA := writeCatalog(t, map[string]string{"go": validGoPkg, "kubectl": pkgNamed("kubectl")})
	ctxA, repA := consoletest.Context(context.Background())
	if _, err := Publish(ctxA, dirA, "example.com/cat", Options{}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}
	first := repA.TaskFor("Comparing with example.com/cat:v4")
	if first == nil {
		t.Fatalf("tasks = %v, want the compare task", labelsOf(repA.Tasks()))
	}
	if done, _, outcome := first.Snapshot(); !done || outcome != "Compared with example.com/cat:v4: 2 to push (tag not found)" {
		t.Fatalf("first compare outcome = %q (done %v)", outcome, done)
	}

	changedGoPkg := strings.Replace(validGoPkg, "description: The Go programming language", "description: Go, updated", 1)
	dirB := writeCatalog(t, map[string]string{"go": changedGoPkg, "helm": pkgNamed("helm")})
	ctxB, repB := consoletest.Context(context.Background())
	if _, err := Publish(ctxB, dirB, "example.com/cat", Options{}); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	compare := repB.TaskFor("Comparing with example.com/cat:v4")
	if compare == nil {
		t.Fatalf("tasks = %v, want the compare task", labelsOf(repB.Tasks()))
	}
	if done, _, outcome := compare.Snapshot(); !done || outcome != "Compared with example.com/cat:v4: 2 to push, 1 removed" {
		t.Fatalf("compare outcome = %q (done %v)", outcome, done)
	}
	push := repB.TaskFor("Pushing go v1.26.5")
	if push == nil {
		t.Fatalf("tasks = %v, want a push task for go", labelsOf(repB.Tasks()))
	}
	if done, _, outcome := push.Snapshot(); !done || outcome != "Pushed go v1.26.5" {
		t.Fatalf("push outcome = %q (done %v)", outcome, done)
	}
	if got := push.Phases(); !slices.Contains(got, "pushing manifest") {
		t.Fatalf("push segments = %v, want pushing manifest", got)
	}
	if repB.TaskFor("Pushing kubectl v1.26.5") != nil {
		t.Fatal("a removed package must not get a push task")
	}
	if slices.ContainsFunc(repB.Successes(), func(s string) bool { return strings.HasPrefix(s, "Pushed ") }) {
		t.Fatalf("pushed outcomes must come from tasks, not bare Success lines: %v", repB.Successes())
	}
}

func TestPublishUnchangedPackagesGetNoTask(t *testing.T) {
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	ctx, _ := consoletest.Context(context.Background())
	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}
	ctx, rep := consoletest.Context(context.Background())
	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err != nil {
		t.Fatalf("republish: %v", err)
	}
	if rep.TaskFor("Pushing go v1.26.5") != nil {
		t.Fatal("an unchanged package must not get a push task")
	}
	compare := rep.TaskFor("Comparing with example.com/cat:v4")
	if compare == nil {
		t.Fatalf("tasks = %v, want the compare task", labelsOf(rep.Tasks()))
	}
	if done, _, outcome := compare.Snapshot(); !done || outcome != "Compared with example.com/cat:v4: 1 unchanged" {
		t.Fatalf("compare outcome = %q (done %v)", outcome, done)
	}
}

func TestPublishFailedPushFailsItsTask(t *testing.T) {
	store, _ := newStore(t)
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) {
		return failOnManifestPushTarget{Target: store}, nil
	})()
	ctx, rep := consoletest.Context(context.Background())
	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err == nil {
		t.Fatal("expected the manifest push failure to surface")
	}
	push := rep.TaskFor("Pushing go v1.26.5")
	if push == nil {
		t.Fatalf("tasks = %v, want a push task for go", labelsOf(rep.Tasks()))
	}
	if _, failed, outcome := push.Snapshot(); !failed || outcome != "Failed to push go v1.26.5" {
		t.Fatalf("push outcome = %q (failed %v)", outcome, failed)
	}
}

func labelsOf(tasks []*consoletest.Task) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.Label)
	}
	return out
}

func TestPublishReportsIndexDigest(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
	dir := writeCatalog(t, map[string]string{"go": validGoPkg, "kubectl": pkgNamed("kubectl")})

	dry, err := Publish(ctx, dir, "example.com/cat", Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	real, err := Publish(ctx, dir, "example.com/cat", Options{})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	desc, err := store.Resolve(ctx, "v4")
	if err != nil {
		t.Fatal(err)
	}
	if real.Index != desc.Digest.String() {
		t.Fatalf("Index = %q, want the published index digest %s", real.Index, desc.Digest)
	}
	if dry.Index != real.Index {
		t.Fatalf("dry-run Index = %q, want the same digest the publish produced (%s)", dry.Index, real.Index)
	}
}

func TestPublishClassifiesAgainstTheFirstResolvedTagOnly(t *testing.T) {
	ctx, _ := consoletest.Context(context.Background())
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()

	dirA := writeCatalog(t, map[string]string{"go": validGoPkg, "alpha": pkgNamed("alpha")})
	if _, err := Publish(ctx, dirA, "example.com/cat", Options{Tags: []string{"old"}}); err != nil {
		t.Fatalf("publish under old: %v", err)
	}
	dirB := writeCatalog(t, map[string]string{"go": validGoPkg})
	if _, err := Publish(ctx, dirB, "example.com/cat", Options{Tags: []string{"v4"}}); err != nil {
		t.Fatalf("publish under v4: %v", err)
	}

	res, err := Publish(ctx, dirB, "example.com/cat", Options{Tags: []string{"v4", "old"}})
	if err != nil {
		t.Fatalf("publish under v4 and old: %v", err)
	}
	if res.Removed != 0 || res.Unchanged != 1 || res.Pushed != 0 {
		t.Fatalf("counts = %d pushed, %d removed, %d unchanged; the stale old tag must not make alpha look removed", res.Pushed, res.Removed, res.Unchanged)
	}

	res, err = Publish(ctx, dirA, "example.com/cat", Options{Tags: []string{"v4", "old"}})
	if err != nil {
		t.Fatalf("re-add alpha: %v", err)
	}
	if res.Packages[0].Name != "alpha" || res.Packages[0].Status != StatusNew || res.Pushed != 1 {
		t.Fatalf("result = %+v, want alpha new relative to v4", res)
	}
}

func TestPublishDryRunChecksManifestsWhenTagIsAbsent(t *testing.T) {
	store, _ := newStore(t)
	defer swapOpenTarget(func(context.Context, string) (oras.Target, error) { return store, nil })()
	dir := writeCatalog(t, map[string]string{"go": validGoPkg})
	ctx, _ := consoletest.Context(context.Background())
	if _, err := Publish(ctx, dir, "example.com/cat", Options{}); err != nil {
		t.Fatalf("initial publish: %v", err)
	}

	ctx, rep := consoletest.Context(context.Background())
	res, err := Publish(ctx, dir, "example.com/cat", Options{Tags: []string{"other"}, DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.Pushed != 0 || res.Unchanged != 1 || res.Packages[0].Status != StatusUnchanged {
		t.Fatalf("result = %+v, want go unchanged (its manifest already exists)", res)
	}
	compare := rep.TaskFor("Comparing with example.com/cat:other")
	if compare == nil {
		t.Fatalf("tasks = %v, want the compare task", labelsOf(rep.Tasks()))
	}
	if done, _, outcome := compare.Snapshot(); !done || outcome != "Compared with example.com/cat:other: 1 unchanged (tag not found)" {
		t.Fatalf("compare outcome = %q (done %v)", outcome, done)
	}
}
