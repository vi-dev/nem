package mirror

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/publish/publishtest"
	"github.com/vi-dev/nem/internal/testx"
)

func urlPkg(name, version, digest string) string {
	return string(publishtest.URLPkgYAML(name, version, "https://example.com/"+name+"/{{.Version}}", publishtest.UniformSha256(digest)))
}

func ociRelativePkg(name, version string) string {
	return string(publishtest.OCIPkgYAML(name, version))
}

func TestRunStagesCatalogAndReadsSrcIndexOnce(t *testing.T) {
	pkgs := map[string]string{}
	for i := range 8 {
		name := "pkg" + string(rune('a'+i))
		pkgs[name] = urlPkg(name, "1.0.0", "deadbeef")
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	opts := Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}

	stagingOnly := &testx.CountingTarget{ReadOnlyTarget: srcStore}
	wireCatalog(t, stagingOnly, srcTag, memory.New(), "v2")
	if _, err := syncCatalog(context.Background(), opts, &testx.Reporter{}); err != nil {
		t.Fatalf("syncCatalog: %v", err)
	}
	wantResolves, wantFetches := stagingOnly.Resolves.Load(), stagingOnly.Fetches.Load()
	if wantFetches == 0 {
		t.Fatal("staging must touch src at all")
	}

	if wantResolves != 1 {
		t.Fatalf("src resolves during staging = %d, want exactly 1 (the index ref, resolved once)", wantResolves)
	}

	counted := &testx.CountingTarget{ReadOnlyTarget: srcStore}
	wireCatalog(t, counted, srcTag, memory.New(), "v2")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), opts, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Packages != 8 {
		t.Fatalf("Packages = %d, want 8", summary.Packages)
	}
	if got := counted.Resolves.Load(); got != wantResolves {
		t.Fatalf("src catalog resolves = %d, want exactly staging's own cost %d (per-package processing must never touch src again)", got, wantResolves)
	}
	if got := counted.Fetches.Load(); got != wantFetches {
		t.Fatalf("src catalog fetches = %d, want exactly staging's own cost %d (per-package processing must never touch src again)", got, wantFetches)
	}
}

func TestRunPublishesCatalogDigestChecked(t *testing.T) {
	srcStore, srcTag := newCatalog(t, map[string]string{"go": urlPkg("go", "1.0.0", "aaaa")})
	wantDesc, err := srcStore.Resolve(context.Background(), srcTag)
	if err != nil {
		t.Fatalf("resolve src: %v", err)
	}

	dst := memory.New()
	wireCatalog(t, srcStore, srcTag, dst, "mirrored")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	if _, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:mirrored"}, &testx.Reporter{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	gotDesc, err := dst.Resolve(context.Background(), "mirrored")
	if err != nil {
		t.Fatalf("resolve dst: %v", err)
	}
	if gotDesc.Digest != wantDesc.Digest {
		t.Fatalf("dst catalog digest = %s, want %s (byte-identical to src)", gotDesc.Digest, wantDesc.Digest)
	}
}

func TestRunDryRunSkipsCatalogPublishEntirely(t *testing.T) {
	srcStore, srcTag := newCatalog(t, map[string]string{"go": urlPkg("go", "1.0.0", "aaaa")})
	dst := memory.New()
	dstOpened := wireCatalog(t, srcStore, srcTag, dst, "v2")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2", DryRun: true}, &testx.Reporter{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if *dstOpened {
		t.Fatal("dry run must never open the dst catalog target")
	}
	if summary.Packages != 1 {
		t.Fatalf("Packages = %d, want 1", summary.Packages)
	}
}

func TestRunCopiesAbsentAndHealsDifferingArchives(t *testing.T) {
	pkgs := map[string]string{
		"present": urlPkg("present", "1.0.0", "aaaa"),
		"absent":  urlPkg("absent", "1.0.0", "bbbb"),
		"stale":   urlPkg("stale", "1.0.0", "cccc"),
	}
	srcStore, srcTag := newCatalog(t, pkgs)

	src := testx.NewArchiveFixtures()
	dst := testx.NewArchiveFixtures()
	for name, payload := range map[string][]byte{
		"present": []byte("present-payload"),
		"absent":  []byte("absent-payload"),
		"stale":   []byte("stale-payload-new"),
	} {
		s := memory.New()
		ocixtest.PushFakeArchive(t, s, "1.0.0", map[string][]byte{"linux/amd64": payload})
		src.Set(name, s)
	}
	presentDst := memory.New()
	ocixtest.PushFakeArchive(t, presentDst, "1.0.0", map[string][]byte{"linux/amd64": []byte("present-payload")})
	dst.Set("present", presentDst)
	staleDst := memory.New()
	ocixtest.PushFakeArchive(t, staleDst, "1.0.0", map[string][]byte{"linux/amd64": []byte("stale-payload-old")})
	dst.Set("stale", staleDst)

	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")
	wireArchives(t, src, dst)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Copied != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, want {Copied:2}", summary)
	}
	if len(rep.Warns()) != 0 {
		t.Fatalf("warns = %v, want none", rep.Warns())
	}

	presentTask := rep.TaskFor("Mirroring present")
	if presentTask == nil {
		t.Fatal("no task for present")
	}
	if statuses := presentTask.Statuses(); len(statuses) != 1 || statuses[0] != "probing" {
		t.Fatalf("present statuses = %v, want [\"probing\"] only: present items must not add a status update", statuses)
	}

	absentTask := rep.TaskFor("Mirroring absent")
	if absentTask == nil {
		t.Fatal("no task for absent")
	}
	if statuses := absentTask.Statuses(); len(statuses) != 2 || statuses[0] != "probing" || statuses[1] != "copying 1.0.0" {
		t.Fatalf("absent statuses = %v, want [\"probing\" \"copying 1.0.0\"]", statuses)
	}

	for _, name := range []string{"present", "absent", "stale"} {
		srcDesc, err := src.Open(name).Resolve(context.Background(), "1.0.0")
		if err != nil {
			t.Fatalf("%s: resolve src: %v", name, err)
		}
		dstDesc, err := dst.Open(name).Resolve(context.Background(), "1.0.0")
		if err != nil {
			t.Fatalf("%s: resolve dst: %v", name, err)
		}
		if srcDesc.Digest != dstDesc.Digest {
			t.Fatalf("%s: dst digest %s != src digest %s", name, dstDesc.Digest, srcDesc.Digest)
		}
	}
}

func TestRunUnfilledIsSilent(t *testing.T) {
	srcStore, srcTag := newCatalog(t, map[string]string{"curl": urlPkg("curl", "1.0.0", "aaaa")})
	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Failed != 0 || summary.Copied != 0 {
		t.Fatalf("summary = %+v, want no copied/failed counts", summary)
	}

	task := rep.TaskFor("Mirroring curl")
	if task == nil {
		t.Fatal("no task for curl: eager creation on pickup must still register one")
	}
	if done, failed, _ := task.Snapshot(); done || failed {
		t.Fatalf("curl task resolved via Done/Fail (done=%v failed=%v), want Discard only", done, failed)
	}
	if !task.Discarded() {
		t.Fatal("curl task was not discarded")
	}
	if len(rep.Warns()) != 0 {
		t.Fatalf("warns = %v, want none for the ordinary unfilled state", rep.Warns())
	}
}

func TestRunPrebuiltMissingTagWarnsAndFails(t *testing.T) {
	srcStore, srcTag := newCatalog(t, map[string]string{"kubectl": ociRelativePkg("kubectl", "1.28.0")})
	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Failed != 1 {
		t.Fatalf("summary = %+v, want {Failed:1}", summary)
	}
	task := rep.TaskFor("Mirroring kubectl")
	if task == nil {
		t.Fatal("no task for kubectl")
	}
	_, failed, outcome := task.Snapshot()
	if !failed || outcome != "Failed kubectl" {
		t.Fatalf("kubectl task failed=%v outcome=%q, want failed \"Failed kubectl\"", failed, outcome)
	}
	warns := rep.Warns()
	if len(warns) != 1 || warns[0] != "kubectl 1.28.0: archive missing from source" {
		t.Fatalf("warns = %v, want exactly one prebuilt-missing warning", warns)
	}
}

func TestRunAbsoluteOCIRefIsFullyIgnored(t *testing.T) {
	pkgs := map[string]string{
		"vendor-tool": absoluteOCIPkgYAML("vendor-tool", "other-registry.example.com/vendor/tool:{{.Version}}", "2.0.0"),
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")
	src := testx.NewArchiveFixtures()
	dst := testx.NewArchiveFixtures()
	wireArchives(t, src, dst)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Packages != 1 || summary.Copied+summary.Failed != 0 {
		t.Fatalf("summary = %+v, want only Packages:1, everything else 0", summary)
	}
	if len(src.OpenedNames()) != 0 || len(dst.OpenedNames()) != 0 {
		t.Fatalf("archives opened for an absolute oci ref: src=%v dst=%v", src.OpenedNames(), dst.OpenedNames())
	}

	task := rep.TaskFor("Mirroring vendor-tool")
	if task == nil {
		t.Fatal("no task for vendor-tool: eager creation on pickup must still register one")
	}
	if !task.Discarded() {
		t.Fatal("vendor-tool task was not discarded")
	}
	if done, failed, _ := task.Snapshot(); done || failed {
		t.Fatalf("vendor-tool task resolved via Done/Fail (done=%v failed=%v), want Discard only", done, failed)
	}
}

func TestRunWarnAndContinueSiblingCompletes(t *testing.T) {
	pkgs := map[string]string{
		"broken":  urlPkg("broken", "1.0.0", "aaaa"),
		"healthy": urlPkg("healthy", "1.0.0", "bbbb"),
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")

	src := testx.NewArchiveFixtures()
	for name, payload := range map[string][]byte{
		"broken":  []byte("broken-payload"),
		"healthy": []byte("healthy-payload"),
	} {
		s := memory.New()
		ocixtest.PushFakeArchive(t, s, "1.0.0", map[string][]byte{"linux/amd64": payload})
		src.Set(name, s)
	}
	dst := testx.NewArchiveFixtures()
	brokenErr := errors.New("dst archives unreachable")
	t.Cleanup(SetDstArchivesOpener(func(_, name string) (oras.Target, error) {
		if name == "broken" {
			return nil, brokenErr
		}
		return dst.Open(name), nil
	}))
	t.Cleanup(SetSrcArchivesOpener(func(_, name string) (oras.ReadOnlyTarget, error) {
		return src.Open(name), nil
	}))

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run must not abort on an item failure: %v", err)
	}
	if summary.Failed != 1 || summary.Copied != 1 {
		t.Fatalf("summary = %+v, want {Failed:1 Copied:1}", summary)
	}

	brokenTask := rep.TaskFor("Mirroring broken")
	if brokenTask == nil {
		t.Fatal("no task for broken")
	}
	if _, failed, outcome := brokenTask.Snapshot(); !failed || outcome != "Failed broken" {
		t.Fatalf("broken task failed=%v outcome=%q", failed, outcome)
	}

	healthyTask := rep.TaskFor("Mirroring healthy")
	if healthyTask == nil {
		t.Fatal("no task for healthy")
	}
	if done, failed, outcome := healthyTask.Snapshot(); !done || failed || outcome != "Mirrored healthy (1 tag(s))" {
		t.Fatalf("healthy task done=%v failed=%v outcome=%q, want done \"Mirrored healthy (1 tag(s))\"", done, failed, outcome)
	}
}

func TestRunCopyFailureIsWarnedAndCounted(t *testing.T) {
	srcStore, srcTag := newCatalog(t, map[string]string{"go": urlPkg("go", "1.0.0", "aaaa")})
	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")

	src := testx.NewArchiveFixtures()
	s := memory.New()
	ocixtest.PushFakeArchive(t, s, "1.0.0", map[string][]byte{"linux/amd64": []byte("go-payload")})
	src.Set("go", s)
	dst := testx.NewArchiveFixtures()
	dst.Set("go", &testx.RejectPushTarget{Target: memory.New()})
	wireArchives(t, src, dst)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run must not abort on a copy failure: %v", err)
	}
	if summary.Failed != 1 {
		t.Fatalf("summary = %+v, want Failed:1", summary)
	}
	warns := rep.Warns()
	if len(warns) != 1 {
		t.Fatalf("warns = %v, want exactly one", warns)
	}
}

func TestRunMidCopyCancellationIsNotWarnedOrCounted(t *testing.T) {
	srcStore, srcTag := newCatalog(t, map[string]string{"go": urlPkg("go", "1.0.0", "aaaa")})
	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")

	src := testx.NewArchiveFixtures()
	s := memory.New()
	ocixtest.PushFakeArchive(t, s, "1.0.0", map[string][]byte{"linux/amd64": []byte("go-payload")})
	src.Set("go", s)

	ctx, cancel := context.WithCancel(context.Background())
	dst := testx.NewArchiveFixtures()
	dst.Set("go", &testx.CancelOnPushTarget{Target: memory.New(), Cancel: cancel})
	wireArchives(t, src, dst)

	rep := &testx.Reporter{}
	summary, err := Run(ctx, Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if summary.Failed != 0 {
		t.Fatalf("Failed = %d, want 0: a mid-run cancellation is not an item failure", summary.Failed)
	}
	for _, w := range rep.Warns() {
		if strings.Contains(w, "context canceled") {
			t.Fatalf("warns = %v, cancellation must not be warned as an item failure", rep.Warns())
		}
	}
}

func TestRunDeterministicSummaryUnderParallel(t *testing.T) {
	const n = 12
	pkgs := map[string]string{}
	src := testx.NewArchiveFixtures()
	dst := testx.NewArchiveFixtures()
	var wantCopied int
	for i := range n {
		name := "pkg" + string(rune('a'+i))
		pkgs[name] = urlPkg(name, "1.0.0", "aaaa")
		switch i % 3 {
		case 0:
			s := memory.New()
			ocixtest.PushFakeArchive(t, s, "1.0.0", map[string][]byte{"linux/amd64": []byte(name)})
			src.Set(name, s)
			d := memory.New()
			ocixtest.PushFakeArchive(t, d, "1.0.0", map[string][]byte{"linux/amd64": []byte(name)})
			dst.Set(name, d)
		case 1:
			s := memory.New()
			ocixtest.PushFakeArchive(t, s, "1.0.0", map[string][]byte{"linux/amd64": []byte(name)})
			src.Set(name, s)
			wantCopied++
		case 2:
		}
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	dstCatalog := memory.New()
	wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")
	wireArchives(t, src, dst)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := Summary{Packages: n, Copied: wantCopied}
	if summary != want {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
}

func TestSummaryString(t *testing.T) {
	s := Summary{Packages: 600, Copied: 102, Failed: 1}
	want := "Mirrored 600 packages, 102 tag(s), 1 tag(s) failed"
	if got := s.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	s = Summary{Packages: 600, Copied: 102}
	want = "Mirrored 600 packages, 102 tag(s)"
	if got := s.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestRunRejectsUntaggedRefs(t *testing.T) {
	if _, err := Run(context.Background(), Options{SrcRef: "example.com/cat", DstRef: "internal.example.com/cat:v2"}, &testx.Reporter{}); err == nil {
		t.Fatal("untagged src ref must be rejected")
	}
	if _, err := Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat"}, &testx.Reporter{}); err == nil {
		t.Fatal("untagged dst ref must be rejected")
	}
}

func TestRunCtxCancelledBeforeStartAborts(t *testing.T) {
	srcStore, srcTag := newCatalog(t, map[string]string{"go": urlPkg("go", "1.0.0", "aaaa")})
	dstCatalog := memory.New()
	dstOpened := wireCatalog(t, srcStore, srcTag, dstCatalog, "v2")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rep := &testx.Reporter{}
	_, err := Run(ctx, Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if *dstOpened {
		t.Fatal("cancelled-before-start run must not open the dst catalog")
	}
}

func wantStableProgress(t *testing.T, taskName string, counts []testx.ProgressCall, wantTotal int64) {
	t.Helper()
	if len(counts) == 0 {
		t.Fatalf("no Progress calls recorded on the %s task", taskName)
	}
	last := counts[len(counts)-1]
	if last.Done != wantTotal || last.Total != wantTotal {
		t.Fatalf("%s: final count = %+v, want done=total=%d", taskName, last, wantTotal)
	}
	for _, c := range counts {
		if c.Total != 0 && c.Total != wantTotal {
			t.Fatalf("%s: count %+v: total changed mid-run, want a stable %d once known", taskName, c, wantTotal)
		}
	}
}

func TestPullCatalogReportsProgressOnPullingTask(t *testing.T) {
	const n = 5
	pkgs := map[string]string{}
	for i := range n {
		name := "pkg" + string(rune('a'+i))
		pkgs[name] = urlPkg(name, "1.0.0", "deadbeef")
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	wireCatalog(t, srcStore, srcTag, memory.New(), "v2")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	opts := Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2", DryRun: true}
	if _, err := Run(context.Background(), opts, rep); err != nil {
		t.Fatalf("Run: %v", err)
	}

	task := rep.TaskFor("Pulling catalog")
	if task == nil {
		t.Fatal("no Pulling catalog task")
	}
	if len(task.Statuses()) == 0 {
		t.Fatal("no Status call recorded on the pull task: Progress alone never renders (segment stays \"\")")
	}
	wantStableProgress(t, "Pulling catalog", task.ProgressCalls(), n+1)

	if rep.TaskFor("Pushing catalog") != nil {
		t.Fatal("dry run must never create a Pushing catalog task")
	}
}

func TestPushCatalogReportsProgressOnPushingTask(t *testing.T) {
	const n = 5
	pkgs := map[string]string{}
	for i := range n {
		name := "pkg" + string(rune('a'+i))
		pkgs[name] = urlPkg(name, "1.0.0", "deadbeef")
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	wireCatalog(t, srcStore, srcTag, memory.New(), "v2")
	wireArchives(t, testx.NewArchiveFixtures(), testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	opts := Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}
	if _, err := Run(context.Background(), opts, rep); err != nil {
		t.Fatalf("Run: %v", err)
	}

	task := rep.TaskFor("Pushing catalog")
	if task == nil {
		t.Fatal("no Pushing catalog task")
	}
	if len(task.Statuses()) == 0 {
		t.Fatal("no Status call recorded on the push task: Progress alone never renders (segment stays \"\")")
	}
	wantStableProgress(t, "Pushing catalog", task.ProgressCalls(), n+1)

	if done, failed, outcome := task.Snapshot(); !done || failed || outcome != "Pushed catalog" {
		t.Fatalf("push task done=%v failed=%v outcome=%q, want done \"Pushed catalog\"", done, failed, outcome)
	}
}
