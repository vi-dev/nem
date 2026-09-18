package build

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

type BatchOptions struct {
	Target *Target
	Push   bool
	Force  bool

	TestFor func(pkg *spec.Package, store *ocix.ArchiveStore) func(ctx context.Context, p *spec.Package, version, artifactPath string) error
}

type SummaryRow struct {
	Name, Version string
	Result        string
	Detail        string
	PushFailed    bool
}

func RunBatch(ctx context.Context, h home.Home, set *catalog.Set,
	plan Plan, opts BatchOptions) ([]SummaryRow, error) {
	rep := report.FromContext(ctx)
	entries := currentPlatformEntries(plan)
	if len(entries) == 0 {
		return nil, nil
	}

	store, cleanup, err := batchStore(h, opts)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	live := make(map[string]int, len(entries))
	for _, e := range entries {
		live[e.Pkg.Name]++
	}

	rows := make([]SummaryRow, 0, len(entries))
	for i, e := range entries {
		name := e.Pkg.Name
		if dep, ok := firstExhaustedNeed(e.Needs, live); ok {
			rep.Warn("Skipping %s@%s: dependency %s failed (all versions)", name, e.Version, dep)
			rows = append(rows, SummaryRow{Name: name, Version: e.Version,
				Result: "skipped", Detail: fmt.Sprintf("needs %s (all versions failed)", dep)})
			live[name]--
			continue
		}

		rep.Info("[%d/%d] wave %d: building %s@%s", i+1, len(entries), e.Wave, name, e.Version)

		bopts := Options{Version: e.Version, LocalStore: store}
		if opts.TestFor != nil {
			bopts.Test = opts.TestFor(e.Pkg, store)
		}
		if err := Build(ctx, h, set, e.Pkg, bopts); err != nil {
			rep.Warn("%s@%s failed: %v", name, e.Version, err)
			rows = append(rows, SummaryRow{Name: name, Version: e.Version,
				Result: "failed", Detail: firstLine(err.Error())})
			live[name]--
			continue
		}

		rows = append(rows, pushBuiltEntry(ctx, store, name, e.Version, opts))
	}
	return rows, nil
}

func Verdict(rows []SummaryRow) (built, pushed, failed, skipped, pushFailed int, err error) {
	for _, r := range rows {
		switch r.Result {
		case "pushed":
			built++
			pushed++
		case "built":
			built++
		case "failed":
			failed++
		case "skipped":
			skipped++
		}
		if r.PushFailed {
			pushFailed++
		}
	}
	broken := failed + skipped
	switch {
	case broken > 0:
		err = fmt.Errorf("%d of %d builds failed or were skipped", broken, len(rows))
	case pushFailed > 0:
		err = fmt.Errorf("%d of %d builds failed to push", pushFailed, len(rows))
	}
	return built, pushed, failed, skipped, pushFailed, err
}

func batchStore(h home.Home, opts BatchOptions) (*ocix.ArchiveStore, func(), error) {
	if opts.Push && opts.Target.IsDir() {
		return opts.Target.Overlay, func() {}, nil
	}
	return newBatchStore(h)
}

func newBatchStore(h home.Home) (*ocix.ArchiveStore, func(), error) {
	if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create tmp dir: %w", err)
	}
	root, err := os.MkdirTemp(h.Tmp(), "batch"+home.BuildStagingInfix)
	if err != nil {
		return nil, nil, fmt.Errorf("create batch archive store: %w", err)
	}
	return ocix.NewArchiveStore(root), func() { os.RemoveAll(root) }, nil
}

func currentPlatformEntries(plan Plan) []PlanEntry {
	var out []PlanEntry
	for _, e := range plan.Entries {
		if slices.Contains(e.Platforms, spec.Current()) {
			out = append(out, e)
		}
	}
	return out
}

func firstExhaustedNeed(needs []string, live map[string]int) (string, bool) {
	for _, dep := range needs {
		if n, ok := live[dep]; ok && n <= 0 {
			return dep, true
		}
	}
	return "", false
}

func pushBuiltEntry(ctx context.Context, store *ocix.ArchiveStore, name, version string, opts BatchOptions) SummaryRow {
	rep := report.FromContext(ctx)
	row := SummaryRow{Name: name, Version: version, Result: "built", Detail: "-"}
	if !opts.Push {
		return row
	}
	if opts.Target.IsDir() {

		row.Result = "pushed"
		rep.Success("Pushed %s %s", name, version)
		return row
	}
	switch pushed, err := pushFromStore(ctx, store, name, version, opts); {
	case err != nil:
		row.Detail = "push failed: " + firstLine(err.Error())
		row.PushFailed = true
	case pushed:
		row.Result = "pushed"
		rep.Success("Pushed %s %s", name, version)
	default:

		row.Result = "pushed"
		row.Detail = "unchanged"
	}
	return row
}

func pushFromStore(ctx context.Context, store *ocix.ArchiveStore, name, version string, opts BatchOptions) (bool, error) {
	staged, err := store.Open(name)
	if err != nil {
		return false, err
	}
	plat := spec.Current()
	archive, err := ocix.ReadArchive(ctx, staged, version, plat)
	if err != nil {
		return false, err
	}
	remote, err := archivesOpener(opts.Target.Ref, name)
	if err != nil {
		return false, err
	}
	entry, pushed, err := ocix.PushArchive(ctx, remote, version, plat, archive, opts.Force)
	if err != nil {
		return false, err
	}
	if !pushed {
		return false, nil
	}
	if err := ocix.VerifyArchiveCommitted(ctx, remote, version, plat, entry); err != nil {
		return false, err
	}
	return true, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
