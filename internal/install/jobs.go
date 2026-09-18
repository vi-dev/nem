package install

import (
	"slices"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func Jobs(result *resolve.Result, set *catalog.Set) []Job {
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
		jobs = append(jobs, Job{
			Pkg:     result.Pkgs[entry.Name],
			Version: entry.Version,
			Catalog: entry.Catalog,
			Source:  fetch.Source{CatalogRef: ref},
		})
	}
	return jobs
}
