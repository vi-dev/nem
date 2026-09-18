package main

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/config"
)

func newSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "search [query]",
		Aliases:           []string{"find"},
		Short:             "Search catalogs for packages",
		Long:              "Search catalogs for packages by name or description.\nWithout a query, list every available package.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeSearchQuery),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) == 1 {
				query = args[0]
			}
			return runSearch(cmd, query)
		},
	}
}

func runSearch(cmd *cobra.Command, query string) error {
	cfg, err := config.OpenConfig(nemHome)
	if err != nil {
		return err
	}
	set, err := catalog.OpenConfigured(cfg, nemHome)
	if err != nil {
		return err
	}

	hits, unsynced, err := set.Summaries(cmd.Context())
	if err != nil {
		return err
	}
	for _, name := range unsynced {
		console.Warn("Catalog %s is not synced (run nem catalog update)", name)
	}

	q := strings.ToLower(query)
	filtered := hits[:0]
	for _, h := range hits {
		if searchMatches(h.Summary, q) {
			filtered = append(filtered, h)
		}
	}
	hits = filtered

	sort.SliceStable(hits, func(i, j int) bool {
		ri, rj := searchRank(hits[i].Summary, q), searchRank(hits[j].Summary, q)
		if ri != rj {
			return ri < rj
		}
		return hits[i].Name < hits[j].Name
	})

	if len(hits) == 0 {
		if query == "" {
			console.Warn("No packages available (run nem catalog update)")
		} else {
			console.Warn("No packages match %q", query)
		}
		return nil
	}

	rows := make([][]string, len(hits))
	for i, h := range hits {
		rows[i] = []string{h.Name, h.Latest, h.Catalog, h.Description}
	}
	console.Table([]string{"name", "version", "catalog", "description"}, rows)
	return nil
}

func searchMatches(s catalog.Summary, q string) bool {
	return strings.Contains(strings.ToLower(s.Name), q) || strings.Contains(strings.ToLower(s.Description), q)
}

func searchRank(s catalog.Summary, q string) int {
	name := strings.ToLower(s.Name)
	switch {
	case name == q:
		return 0
	case strings.HasPrefix(name, q):
		return 1
	case strings.Contains(name, q):
		return 2
	default:
		return 3
	}
}
