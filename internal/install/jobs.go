package install

import (
	"slices"

	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func Jobs(result *resolve.Result, set *catalog.Set, local *ocix.ArchiveStore) []Job {
	current := spec.Current().String()
	var jobs []Job
	for _, entry := range result.Entries {
		if !slices.Contains(entry.Platforms, current) {
			continue
		}
		ref := ""
		if e, ok := set.Find(entry.Catalog); ok {
			ref = e.Ref
		}
		job := Job{
			Pkg:     result.Pkgs[entry.Name],
			Version: entry.Version,
			Catalog: entry.Catalog,
			Source:  fetch.Source{CatalogRef: ref},
		}
		if local != nil && local.HasTag(entry.Name, entry.Version) {
			job.Source.LocalArchives = func(name string) (oras.ReadOnlyTarget, error) {
				return local.Open(name)
			}
			job.Reinstall = true
		}
		jobs = append(jobs, job)
	}
	return jobs
}
