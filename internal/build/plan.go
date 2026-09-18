package build

import (
	"fmt"
	"slices"
	"sort"

	"github.com/vi-dev/nem/internal/spec"
)

type Selection struct {
	Pkg       *spec.Package
	Version   string
	Reason    string
	Platforms []spec.Platform
}

type PlanEntry struct {
	Selection
	Wave  int
	Needs []string
}

type Plan struct{ Entries []PlanEntry }

type CycleError struct{ Names []string }

func (e *CycleError) Error() string {
	return fmt.Sprintf("dependency cycle detected among packages: %v", e.Names)
}

func ComputePlan(sels []Selection) (Plan, error) {
	byName := make(map[string][]Selection, len(sels))
	seenVersions := make(map[string]bool, len(sels))
	for _, s := range sels {
		name := s.Pkg.Name
		key := name + "@" + s.Version
		if seenVersions[key] {
			return Plan{}, fmt.Errorf("duplicate selection %s@%s", name, s.Version)
		}
		seenVersions[key] = true
		byName[name] = append(byName[name], s)
	}

	unionDeps := make(map[string][]spec.Dep, len(byName))
	unionPlatforms := make(map[string][]spec.Platform, len(byName))
	for name, group := range byName {
		var deps []spec.Dep
		var plats []spec.Platform
		for _, s := range group {
			deps = append(deps, s.Pkg.Deps...)
			if s.Pkg.Build != nil {
				deps = append(deps, s.Pkg.Build.Deps...)
			}
			plats = append(plats, s.Platforms...)
		}
		unionDeps[name] = deps
		unionPlatforms[name] = plats
	}

	needs := make(map[string]map[string]bool, len(byName))
	indegree := make(map[string]int, len(byName))
	dependents := make(map[string][]string, len(byName))

	for name := range byName {
		needs[name] = map[string]bool{}
		indegree[name] = 0
	}

	for name := range byName {
		for _, dep := range unionDeps[name] {
			if _, ok := byName[dep.Name]; !ok || dep.Name == name {
				continue
			}
			if sharesFilteredPlatform(unionPlatforms[name], unionPlatforms[dep.Name], dep.Platforms) {
				if !needs[name][dep.Name] {
					needs[name][dep.Name] = true
					indegree[name]++
					dependents[dep.Name] = append(dependents[dep.Name], name)
				}
			}
		}
	}

	scheduled := make(map[string]int, len(byName))
	remaining := make(map[string]bool, len(byName))
	for name := range byName {
		remaining[name] = true
	}

	wave := 0
	for len(remaining) > 0 {
		var ready []string
		for name := range remaining {
			if indegree[name] == 0 {
				ready = append(ready, name)
			}
		}
		if len(ready) == 0 {
			break
		}
		sort.Strings(ready)
		wave++
		for _, name := range ready {
			scheduled[name] = wave
			delete(remaining, name)
		}
		for _, name := range ready {
			for _, dependent := range dependents[name] {
				if remaining[dependent] {
					indegree[dependent]--
				}
			}
		}
	}

	if len(remaining) > 0 {
		leftover := make([]string, 0, len(remaining))
		for name := range remaining {
			leftover = append(leftover, name)
		}
		sort.Strings(leftover)
		return Plan{}, &CycleError{Names: leftover}
	}

	entries := make([]PlanEntry, 0, len(sels))
	for name, group := range byName {
		needList := make([]string, 0, len(needs[name]))
		for dep := range needs[name] {
			needList = append(needList, dep)
		}
		sort.Strings(needList)
		for _, s := range group {
			entries = append(entries, PlanEntry{
				Selection: s,
				Wave:      scheduled[name],
				Needs:     needList,
			})
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Wave != entries[j].Wave {
			return entries[i].Wave < entries[j].Wave
		}
		return entries[i].Pkg.Name < entries[j].Pkg.Name
	})

	return Plan{Entries: entries}, nil
}

func sharesFilteredPlatform(aPlats, bPlats []spec.Platform, depPlats []spec.Platform) bool {
	for _, p := range aPlats {
		if !spec.PlatformsInclude(depPlats, p) {
			continue
		}
		if slices.Contains(bPlats, p) {
			return true
		}
	}
	return false
}
