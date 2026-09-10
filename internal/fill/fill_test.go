package fill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/publish/publishtest"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func TestRunStagesCatalogAndReadsSrcIndexOnce(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)

	pkgs := map[string]string{}
	for i := range 8 {
		name := "pkg" + string(rune('a'+i))
		pkgs[name] = urlPkg(name, "1.0.0", up.URL+"/"+name+"/{{.Version}}", "deadbeef")
	}
	store, tag := newCatalog(t, pkgs)

	stagingOnly := &testx.CountingTarget{ReadOnlyTarget: store}
	wireCatalog(t, stagingOnly, tag)
	if _, err := stageCatalog(context.Background(), "example.com/cat:v2", &testx.Reporter{}); err != nil {
		t.Fatalf("stageCatalog: %v", err)
	}
	wantResolves, wantFetches := stagingOnly.Resolves.Load(), stagingOnly.Fetches.Load()
	if wantFetches == 0 {
		t.Fatal("staging must touch the catalog at all")
	}

	if wantResolves != 1 {
		t.Fatalf("catalog resolves during staging = %d, want exactly 1 (the index ref, resolved once)", wantResolves)
	}

	counted := &testx.CountingTarget{ReadOnlyTarget: store}
	wireCatalog(t, counted, tag)
	wireArchives(t, testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Packages != 8 {
		t.Fatalf("Packages = %d, want 8", summary.Packages)
	}
	if got := counted.Resolves.Load(); got != wantResolves {
		t.Fatalf("catalog resolves = %d, want exactly staging's own cost %d", got, wantResolves)
	}
	if got := counted.Fetches.Load(); got != wantFetches {
		t.Fatalf("catalog fetches = %d, want exactly staging's own cost %d", got, wantFetches)
	}
}

func TestRunPresentSkipsDownload(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	payload := []byte("present-payload")

	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", testx.Sha256Hex(payload)),
	})
	wireCatalog(t, store, tag)

	archives := testx.NewArchiveFixtures()
	s := memory.New()
	platMap := map[string][]byte{}
	for _, plat := range spec.SupportedPlatforms {
		platMap[plat.String()] = payload
	}
	ocixtest.PushFakeArchive(t, s, "1.0.0", platMap)
	archives.Set("go", s)
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := Summary{Packages: 1, Present: len(spec.SupportedPlatforms)}
	if summary != want {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
	if got := up.Hits(); got != 0 {
		t.Fatalf("upstream hits = %d, want 0 (nothing missing)", got)
	}

	task := rep.TaskFor("Filling go")
	if task == nil {
		t.Fatal("no task for go: eager creation on pickup must still register one")
	}
	if !task.Discarded() {
		t.Fatal("go task was not discarded")
	}
	if done, failed, _ := task.Snapshot(); done || failed {
		t.Fatalf("go task resolved via Done/Fail (done=%v failed=%v), want Discard only", done, failed)
	}
	if statuses := task.Statuses(); len(statuses) != 1 || statuses[0] != "probing" {
		t.Fatalf("statuses = %v, want [\"probing\"] only: present items must not add a status update", statuses)
	}
}

func TestRunHealsStaleArchive(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	newPayload := []byte("new-payload")
	up.Set("/go/1.0.0", newPayload)

	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", testx.Sha256Hex(newPayload)),
	})
	wireCatalog(t, store, tag)

	archives := testx.NewArchiveFixtures()
	s := memory.New()
	stalePlat := spec.SupportedPlatforms[0]
	ocixtest.PushFakeArchive(t, s, "1.0.0", map[string][]byte{stalePlat.String(): []byte("old-payload")})
	archives.Set("go", s)
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Healed != 1 || summary.Filled != len(spec.SupportedPlatforms)-1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, want Healed:1 Filled:%d", summary, len(spec.SupportedPlatforms)-1)
	}

	task := rep.TaskFor("Filling go")
	if task == nil {
		t.Fatal("no task for go")
	}
	wantStatus := "healing 1.0.0 " + stalePlat.String()
	var sawHealing bool
	for _, s := range task.Statuses() {
		if s == wantStatus {
			sawHealing = true
		}
	}
	if !sawHealing {
		t.Fatalf("statuses = %v, want %q among them", task.Statuses(), wantStatus)
	}

	for _, plat := range spec.SupportedPlatforms {
		got, err := ocix.ArchiveLayerDigest(context.Background(), s, "1.0.0", plat)
		if err != nil {
			t.Fatalf("resolve %s: %v", plat, err)
		}
		if got.Encoded() != testx.Sha256Hex(newPayload) {
			t.Fatalf("%s digest = %s, want healed %s", plat, got.Encoded(), testx.Sha256Hex(newPayload))
		}
	}
}

func TestRunFillsMultiPlatformWithOneIndexCommit(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	payload := []byte("go-payload")
	up.Set("/go/1.0.0", payload)

	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", testx.Sha256Hex(payload)),
	})
	wireCatalog(t, store, tag)

	counted := &testx.IndexPushCountingTarget{Target: memory.New()}
	archives := testx.NewArchiveFixtures()
	archives.Set("go", counted)
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Filled != len(spec.SupportedPlatforms) || summary.Failed != 0 {
		t.Fatalf("summary = %+v, want Filled:%d Failed:0", summary, len(spec.SupportedPlatforms))
	}
	if got := counted.IndexPushes.Load(); got != 1 {
		t.Fatalf("index pushes = %d, want exactly 1 for a %d-platform fill", got, len(spec.SupportedPlatforms))
	}
	if got := counted.Tags.Load(); got != 1 {
		t.Fatalf("tag calls = %d, want exactly 1 for a %d-platform fill", got, len(spec.SupportedPlatforms))
	}
	if len(rep.Infos()) != 0 {
		t.Fatalf("infos = %v, want none: per-item progress must not print", rep.Infos())
	}

	task := rep.TaskFor("Filling go")
	if task == nil {
		t.Fatal("no task for go")
	}
	wantOutcome := fmt.Sprintf("Filled go (%d fill(s), 0 heal(s))", len(spec.SupportedPlatforms))
	if done, failed, outcome := task.Snapshot(); !done || failed || outcome != wantOutcome {
		t.Fatalf("go task done=%v failed=%v outcome=%q, want done %q", done, failed, outcome, wantOutcome)
	}
	wantStatus := "filling 1.0.0 " + spec.SupportedPlatforms[0].String()
	var sawFilling bool
	for _, s := range task.Statuses() {
		if s == wantStatus {
			sawFilling = true
		}
	}
	if !sawFilling {
		t.Fatalf("statuses = %v, want %q among them", task.Statuses(), wantStatus)
	}

	for _, plat := range spec.SupportedPlatforms {
		got, err := ocix.ArchiveLayerDigest(context.Background(), counted, "1.0.0", plat)
		if err != nil {
			t.Fatalf("resolve published archive for %s: %v", plat, err)
		}
		if got.Encoded() != testx.Sha256Hex(payload) {
			t.Fatalf("%s digest = %s, want %s", plat, got.Encoded(), testx.Sha256Hex(payload))
		}
	}
}

func TestRunPartialBatchCommitsSuccessfulPlatforms(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)

	failing := spec.SupportedPlatforms[0]
	sha := map[string]string{}
	for _, p := range spec.SupportedPlatforms {
		if p == failing {
			sha[p.String()] = "deadbeef"
			continue
		}
		payload := []byte("payload-" + p.String())
		up.Set("/go/"+p.String()+"/1.0.0", payload)
		sha[p.String()] = testx.Sha256Hex(payload)
	}

	store, tag := newCatalog(t, map[string]string{
		"go": string(publishtest.URLPkgYAML("go", "1.0.0", up.URL+"/go/{{.OS}}/{{.Arch}}/{{.Version}}", sha)),
	})
	wireCatalog(t, store, tag)

	counted := &testx.IndexPushCountingTarget{Target: memory.New()}
	archives := testx.NewArchiveFixtures()
	archives.Set("go", counted)
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run must not abort on an item failure: %v", err)
	}
	wantFilled := len(spec.SupportedPlatforms) - 1
	if summary.Filled != wantFilled || summary.Failed != 1 {
		t.Fatalf("summary = %+v, want Filled:%d Failed:1", summary, wantFilled)
	}
	if got := counted.IndexPushes.Load(); got != 1 {
		t.Fatalf("index pushes = %d, want exactly 1 for the batch's %d successful platforms", got, wantFilled)
	}

	task := rep.TaskFor("Filling go")
	if task == nil {
		t.Fatal("no task for go")
	}
	if _, failed, outcome := task.Snapshot(); !failed || outcome != "Failed go" {
		t.Fatalf("go task failed=%v outcome=%q, want failed \"Failed go\"", failed, outcome)
	}

	plats, err := ocix.ArchivePlatforms(context.Background(), counted, "1.0.0")
	if err != nil {
		t.Fatalf("ArchivePlatforms: %v", err)
	}
	if len(plats) != wantFilled {
		t.Fatalf("committed platforms = %v, want %d (every platform but the failing one)", plats, wantFilled)
	}
	for _, p := range plats {
		if p == failing {
			t.Fatalf("failing platform %s was committed despite its own publish failing", failing)
		}
	}
}

type rejectIndexTagTarget struct {
	oras.Target
}

func (r *rejectIndexTagTarget) Tag(context.Context, ocispec.Descriptor, string) error {
	return errors.New("tag rejected")
}

func TestRunBatchCommitFailureCountsAsFailedNotFilled(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	payload := []byte("go-payload")
	up.Set("/go/1.0.0", payload)

	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", testx.Sha256Hex(payload)),
	})
	wireCatalog(t, store, tag)
	t.Cleanup(SetArchivesOpener(func(_, _ string) (oras.Target, error) {
		return &rejectIndexTagTarget{Target: memory.New()}, nil
	}))

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run must not abort on a commit failure: %v", err)
	}
	if summary.Filled != 0 || summary.Healed != 0 {
		t.Fatalf("summary = %+v, want Filled:0 Healed:0 — a failed commit must never count a success", summary)
	}
	if summary.Failed != len(spec.SupportedPlatforms) {
		t.Fatalf("summary.Failed = %d, want %d (every platform in the failed batch)", summary.Failed, len(spec.SupportedPlatforms))
	}
	if len(rep.Warns()) == 0 {
		t.Fatal("want at least one warn for the commit failure")
	}

	task := rep.TaskFor("Filling go")
	if task == nil {
		t.Fatal("no task for go")
	}
	if _, failed, outcome := task.Snapshot(); !failed || outcome != "Failed go" {
		t.Fatalf("go task failed=%v outcome=%q, want failed \"Failed go\"", failed, outcome)
	}
}

func TestRunNotFillablePackageIsSilentAggregate(t *testing.T) {
	store, tag := newCatalog(t, map[string]string{"kubectl": ociPkg("kubectl", "1.28.0")})
	wireCatalog(t, store, tag)
	archives := testx.NewArchiveFixtures()
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.NotFillable != 1 || summary.Failed != 0 || summary.Filled != 0 {
		t.Fatalf("summary = %+v, want NotFillable:1", summary)
	}
	task := rep.TaskFor("Filling kubectl")
	if task == nil {
		t.Fatal("no task for kubectl: eager creation on pickup must still register one")
	}
	if !task.Discarded() {
		t.Fatal("kubectl task was not discarded")
	}
	if done, failed, _ := task.Snapshot(); done || failed {
		t.Fatalf("kubectl task resolved via Done/Fail (done=%v failed=%v), want Discard only", done, failed)
	}
	if len(rep.Warns()) != 0 || len(rep.Infos()) != 0 {
		t.Fatalf("narration = warns:%v infos:%v, want none", rep.Warns(), rep.Infos())
	}
	if len(archives.OpenedNames()) != 0 {
		t.Fatalf("archives opened for a not-fillable package: %v", archives.OpenedNames())
	}
}

func TestRunUpstream404WarnsAndFailsNotAborts(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)

	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", "deadbeef"),
	})
	wireCatalog(t, store, tag)
	wireArchives(t, testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run must not abort on a 404: %v", err)
	}
	if summary.Failed == 0 {
		t.Fatalf("summary = %+v, want Failed > 0", summary)
	}
	task := rep.TaskFor("Filling go")
	if task == nil {
		t.Fatal("no task for go")
	}
	if _, failed, outcome := task.Snapshot(); !failed || outcome != "Failed go" {
		t.Fatalf("go task failed=%v outcome=%q, want failed \"Failed go\"", failed, outcome)
	}
	warns := rep.Warns()
	if len(warns) == 0 {
		t.Fatal("want at least one warn for the 404")
	}
	for _, w := range warns {
		if !strings.Contains(w, "not found") {
			t.Fatalf("warn %q does not mention the artifact being gone", w)
		}
	}
}

func TestRunWarnAndContinueSiblingCompletes(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	payload := []byte("healthy-payload")
	up.Set("/healthy/1.0.0", payload)

	store, tag := newCatalog(t, map[string]string{
		"broken":  urlPkg("broken", "1.0.0", up.URL+"/broken/{{.Version}}", "deadbeef"),
		"healthy": urlPkg("healthy", "1.0.0", up.URL+"/healthy/{{.Version}}", testx.Sha256Hex(payload)),
	})
	wireCatalog(t, store, tag)

	healthyArchives := memory.New()
	brokenErr := errors.New("archives unreachable")
	t.Cleanup(SetArchivesOpener(func(_, name string) (oras.Target, error) {
		if name == "broken" {
			return nil, brokenErr
		}
		return healthyArchives, nil
	}))

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run must not abort on an item failure: %v", err)
	}
	if summary.Failed != 1 || summary.Filled != len(spec.SupportedPlatforms) {
		t.Fatalf("summary = %+v, want Failed:1 Filled:%d", summary, len(spec.SupportedPlatforms))
	}

	brokenTask := rep.TaskFor("Filling broken")
	if brokenTask == nil {
		t.Fatal("no task for broken")
	}
	if _, failed, outcome := brokenTask.Snapshot(); !failed || outcome != "Failed broken" {
		t.Fatalf("broken task failed=%v outcome=%q", failed, outcome)
	}

	healthyTask := rep.TaskFor("Filling healthy")
	if healthyTask == nil {
		t.Fatal("no task for healthy")
	}
	wantHealthyOutcome := fmt.Sprintf("Filled healthy (%d fill(s), 0 heal(s))", len(spec.SupportedPlatforms))
	if done, failed, outcome := healthyTask.Snapshot(); !done || failed || outcome != wantHealthyOutcome {
		t.Fatalf("healthy task done=%v failed=%v outcome=%q, want done %q", done, failed, outcome, wantHealthyOutcome)
	}
}

func TestRunPublishFailureIsWarnedAndCounted(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	payload := []byte("go-payload")
	up.Set("/go/1.0.0", payload)

	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", testx.Sha256Hex(payload)),
	})
	wireCatalog(t, store, tag)
	t.Cleanup(SetArchivesOpener(func(_, _ string) (oras.Target, error) {
		return &testx.RejectPushTarget{Target: memory.New()}, nil
	}))

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run must not abort on a publish failure: %v", err)
	}
	if summary.Failed == 0 {
		t.Fatalf("summary = %+v, want Failed > 0", summary)
	}
	if len(rep.Warns()) == 0 {
		t.Fatal("want at least one warn")
	}
}

func TestRunMidPublishCancellationIsNotWarnedOrCounted(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	payload := []byte("go-payload")
	up.Set("/go/1.0.0", payload)

	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", testx.Sha256Hex(payload)),
	})
	wireCatalog(t, store, tag)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(SetArchivesOpener(func(_, _ string) (oras.Target, error) {
		return &testx.CancelOnPushTarget{Target: memory.New(), Cancel: cancel}, nil
	}))

	rep := &testx.Reporter{}
	summary, err := Run(ctx, newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
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

func TestRunDryRunPlansEverythingNoIO(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	missingPayload := []byte("missing-payload")
	stalePayload := []byte("new-stale-payload")
	up.Set("/missing/1.0.0", missingPayload)
	up.Set("/stale/1.0.0", stalePayload)

	presentPayload := []byte("present-payload")
	store, tag := newCatalog(t, map[string]string{
		"missing":  urlPkg("missing", "1.0.0", up.URL+"/missing/{{.Version}}", testx.Sha256Hex(missingPayload)),
		"stale":    urlPkg("stale", "1.0.0", up.URL+"/stale/{{.Version}}", testx.Sha256Hex(stalePayload)),
		"present":  urlPkg("present", "1.0.0", up.URL+"/present/{{.Version}}", testx.Sha256Hex(presentPayload)),
		"prebuilt": ociPkg("prebuilt", "1.0.0"),
	})
	wireCatalog(t, store, tag)

	archives := testx.NewArchiveFixtures()
	presentStore := memory.New()
	platMap := map[string][]byte{}
	for _, plat := range spec.SupportedPlatforms {
		platMap[plat.String()] = presentPayload
	}
	ocixtest.PushFakeArchive(t, presentStore, "1.0.0", platMap)
	archives.Set("present", presentStore)
	staleStore := memory.New()
	ocixtest.PushFakeArchive(t, staleStore, "1.0.0", map[string][]byte{spec.SupportedPlatforms[0].String(): []byte("old")})
	archives.Set("stale", &noPushTarget{Target: staleStore, t: t})
	archives.Set("missing", &noPushTarget{Target: memory.New(), t: t})
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2", DryRun: true}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := Summary{
		Packages:    4,
		Filled:      len(spec.SupportedPlatforms) + (len(spec.SupportedPlatforms) - 1),
		Healed:      1,
		Present:     len(spec.SupportedPlatforms),
		NotFillable: 1,
		DryRun:      true,
	}
	if summary != want {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
	if got := up.Hits(); got != 0 {
		t.Fatalf("upstream hits = %d, want 0", got)
	}

	if len(rep.Infos()) != 0 {
		t.Fatalf("infos = %v, want none: per-item progress must not print", rep.Infos())
	}

	missingTask := rep.TaskFor("Filling missing")
	if missingTask == nil {
		t.Fatal("no task for missing")
	}
	staleTask := rep.TaskFor("Filling stale")
	if staleTask == nil {
		t.Fatal("no task for stale")
	}

	var sawWouldFill, sawWouldHeal bool
	for _, s := range missingTask.Statuses() {
		if strings.HasPrefix(s, "would fill 1.0.0 ") {
			sawWouldFill = true
		}
	}
	for _, s := range staleTask.Statuses() {
		if strings.HasPrefix(s, "would heal 1.0.0 ") {
			sawWouldHeal = true
		}
	}
	if !sawWouldFill || !sawWouldHeal {
		t.Fatalf("missing statuses = %v, stale statuses = %v, want would-fill/would-heal status transitions",
			missingTask.Statuses(), staleTask.Statuses())
	}

	if _, err := archives.Open("missing").(oras.ReadOnlyTarget).Resolve(context.Background(), "1.0.0"); err == nil {
		t.Fatal("dry run must not write archives")
	}
}

func TestRunDryRunNeverCreatesTmpDir(t *testing.T) {
	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", "https://example.com/go/{{.Version}}", "deadbeef"),
	})
	wireCatalog(t, store, tag)
	wireArchives(t, testx.NewArchiveFixtures())

	h := bareHome(t)
	if _, err := Run(context.Background(), h, Options{CatalogRef: "example.com/cat:v2", DryRun: true}, &testx.Reporter{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(h.Tmp()); !os.IsNotExist(err) {
		t.Fatalf("dry run must not create %s", h.Tmp())
	}
}

type noPushTarget struct {
	oras.Target
	t *testing.T
}

func (n *noPushTarget) Push(context.Context, ocispec.Descriptor, io.Reader) error {
	n.t.Helper()
	n.t.Fatal("dry run must never push an archive layer")
	return nil
}

func TestRunPkgScopingFiltersToNamed(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	payload := []byte("go-payload")
	up.Set("/go/1.0.0", payload)

	store, tag := newCatalog(t, map[string]string{
		"go":   urlPkg("go", "1.0.0", up.URL+"/go/{{.Version}}", testx.Sha256Hex(payload)),
		"curl": urlPkg("curl", "1.0.0", up.URL+"/curl/{{.Version}}", "deadbeef"),
	})
	wireCatalog(t, store, tag)
	archives := testx.NewArchiveFixtures()
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2", Pkgs: []string{"go"}}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Packages != 1 || summary.Filled != len(spec.SupportedPlatforms) {
		t.Fatalf("summary = %+v, want Packages:1 Filled:%d", summary, len(spec.SupportedPlatforms))
	}
	for _, name := range archives.OpenedNames() {
		if name == "curl" {
			t.Fatal("curl's archives were opened despite being out of --pkg scope")
		}
	}
}

func TestRunPkgScopingUnknownNameErrors(t *testing.T) {
	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", "https://example.com/go/{{.Version}}", "deadbeef"),
	})
	wireCatalog(t, store, tag)
	archives := testx.NewArchiveFixtures()
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	_, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2", Pkgs: []string{"zzz", "go", "aaa"}}, rep)
	if err == nil {
		t.Fatal("unknown --pkg name must error")
	}
	if !strings.Contains(err.Error(), "aaa") || !strings.Contains(err.Error(), "zzz") {
		t.Fatalf("error = %v, want it to list both unknown names", err)
	}
	if strings.Index(err.Error(), "aaa") > strings.Index(err.Error(), "zzz") {
		t.Fatalf("error = %v, want unknown names sorted", err)
	}
	if len(archives.OpenedNames()) != 0 {
		t.Fatalf("archives opened despite the scoping error: %v", archives.OpenedNames())
	}
}

func TestScopePackagesDedupesRequestedNames(t *testing.T) {
	all := []ocix.TitledManifest{{Title: "a"}, {Title: "b"}}
	got, err := scopePackages(all, []string{"a", "a"})
	if err != nil {
		t.Fatalf("scopePackages: %v", err)
	}
	if len(got) != 1 || got[0].Title != "a" {
		t.Fatalf("got %+v, want exactly one entry for a", got)
	}
}

func TestRunDeterministicSummaryUnderParallel(t *testing.T) {
	const n = 12
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	pkgs := map[string]string{}
	archives := testx.NewArchiveFixtures()
	var wantFilled, wantPresent, wantNotFillable int
	for i := range n {
		name := "pkg" + string(rune('a'+i))
		switch i % 3 {
		case 0:
			payload := []byte(name)
			pkgs[name] = urlPkg(name, "1.0.0", up.URL+"/"+name+"/{{.Version}}", testx.Sha256Hex(payload))
			s := memory.New()
			platMap := map[string][]byte{}
			for _, plat := range spec.SupportedPlatforms {
				platMap[plat.String()] = payload
			}
			ocixtest.PushFakeArchive(t, s, "1.0.0", platMap)
			archives.Set(name, s)
			wantPresent += len(spec.SupportedPlatforms)
		case 1:
			payload := []byte(name)
			pkgs[name] = urlPkg(name, "1.0.0", up.URL+"/"+name+"/{{.Version}}", testx.Sha256Hex(payload))
			up.Set("/"+name+"/1.0.0", payload)
			wantFilled += len(spec.SupportedPlatforms)
		case 2:
			pkgs[name] = ociPkg(name, "1.0.0")
			wantNotFillable++
		}
	}
	store, tag := newCatalog(t, pkgs)
	wireCatalog(t, store, tag)
	wireArchives(t, archives)

	rep := &testx.Reporter{}
	summary, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := Summary{Packages: n, Filled: wantFilled, Present: wantPresent, NotFillable: wantNotFillable}
	if summary != want {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
}

func TestRunRejectsUntaggedRef(t *testing.T) {
	if _, err := Run(context.Background(), newHome(t), Options{CatalogRef: "example.com/cat"}, &testx.Reporter{}); err == nil {
		t.Fatal("untagged catalog ref must be rejected")
	}
}

func TestRunCtxCancelledBeforeStartAborts(t *testing.T) {
	store, tag := newCatalog(t, map[string]string{
		"go": urlPkg("go", "1.0.0", "https://example.com/go/{{.Version}}", "deadbeef"),
	})
	var opened bool
	t.Cleanup(SetCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) {
		opened = true
		return store, tag, nil
	}))
	wireArchives(t, testx.NewArchiveFixtures())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rep := &testx.Reporter{}
	_, err := Run(ctx, newHome(t), Options{CatalogRef: "example.com/cat:v2"}, rep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if opened {
		t.Fatal("cancelled-before-start run must not open the catalog")
	}
}

func TestStageCatalogReportsProgressOnStagingTask(t *testing.T) {
	const n = 5
	pkgs := map[string]string{}
	for i := range n {
		name := fmt.Sprintf("pkg%02d", i)
		pkgs[name] = urlPkg(name, "1.0.0", "https://example.com/"+name+"/{{.Version}}", "deadbeef")
	}
	store, tag := newCatalog(t, pkgs)
	wireCatalog(t, store, tag)
	wireArchives(t, testx.NewArchiveFixtures())

	rep := &testx.Reporter{}
	opts := Options{CatalogRef: "example.com/cat:v2", DryRun: true}
	if _, err := Run(context.Background(), newHome(t), opts, rep); err != nil {
		t.Fatalf("Run: %v", err)
	}

	task := rep.TaskFor("Pulling catalog")
	if task == nil {
		t.Fatal("no Pulling catalog task")
	}
	if len(task.Statuses()) == 0 {
		t.Fatal("no Status call recorded on the staging task: Progress alone never renders (segment stays \"\")")
	}
	counts := task.ProgressCalls()
	if len(counts) == 0 {
		t.Fatal("no Progress calls recorded on the staging task")
	}
	wantTotal := int64(n + 1)
	last := counts[len(counts)-1]
	if last.Done != wantTotal || last.Total != wantTotal {
		t.Fatalf("final count = %+v, want done=total=%d", last, wantTotal)
	}
	for _, c := range counts {
		if c.Total != 0 && c.Total != wantTotal {
			t.Fatalf("count %+v: total changed mid-run, want a stable %d once known", c, wantTotal)
		}
	}
}
