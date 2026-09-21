package install

import (
	"slices"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func Jobs(result *resolve.Result, set *catalog.Set, local *archive.Dir) []Job {
	current := spec.Current().String()
	var jobs []Job
	for _, entry := range result.Entries {
		if !slices.Contains(entry.Platforms, current) {
			continue
		}
		var stores []archive.Store
		if e, ok := set.Find(entry.Catalog); ok && e.Archives != nil {
			stores = append(stores, e.Archives)
		}
		job := Job{
			Pkg:     result.Pkgs[entry.Name],
			Version: entry.Version,
			Catalog: entry.Catalog,
			Source:  fetch.Source{Archives: stores},
		}
		if local != nil && local.HasVersion(entry.Name, entry.Version) {
			job.Source.Archives = append([]archive.Store{local}, job.Source.Archives...)
			job.Reinstall = true
		}
		jobs = append(jobs, job)
	}
	return jobs
}
