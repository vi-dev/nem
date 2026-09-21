package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/discover"
	"github.com/vi-dev/nem/internal/spec"
)

var discoverLatest = discover.Latest

type outdatedRow struct {
	Name     string `json:"name"`
	Current  string `json:"current"`
	Latest   string `json:"latest,omitempty"`
	Outdated bool   `json:"outdated"`
	Error    string `json:"error,omitempty"`
}

func newCatalogOutdatedCmd() *cobra.Command {
	var packages []string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:               "outdated [catalog]",
		Aliases:           []string{"old"},
		Short:             "Report packages whose upstream has a newer version",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cat := "."
			if len(args) == 1 {
				cat = args[0]
			}
			entry, err := catalog.Open(ctx, cat)
			if err != nil {
				return err
			}
			names, err := entry.Catalog.PackageNames(ctx)
			if err != nil {
				return err
			}
			if len(packages) > 0 {
				var selected []string
				for _, p := range packages {
					if slices.Contains(selected, p) {
						continue
					}
					if !slices.Contains(names, p) {
						return &catalog.PackageNotFoundError{Name: p, Catalogs: []string{entry.Name}}
					}
					selected = append(selected, p)
				}
				names = selected
			}

			_, isFile := entry.Catalog.(*catalog.File)
			explicit := len(packages) > 0 || isFile

			var checked []*spec.Package
			skipped := 0
			for _, name := range names {
				data, err := entry.Catalog.ReadManifest(ctx, name)
				if err != nil {
					return err
				}
				pkg, err := spec.Parse(data)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				if pkg.VersionDiscovery == nil {
					skipped++
					console.Debug("Skipping %s: no versionDiscovery", pkg.Name)
					continue
				}
				checked = append(checked, pkg)
			}

			rows := make([]outdatedRow, len(checked))
			g, gctx := errgroup.WithContext(ctx)
			g.SetLimit(8)
			for i, pkg := range checked {
				g.Go(func() error {
					rows[i] = checkOutdated(gctx, pkg, explicit)
					return nil
				})
			}
			_ = g.Wait()

			outdated, failed := 0, 0
			var tableRows [][]string
			for _, r := range rows {
				switch {
				case r.Error != "":
					failed++
				case r.Outdated:
					outdated++
					tableRows = append(tableRows, []string{r.Name, r.Current, r.Latest})
				}
			}
			if jsonOut {
				if err := console.JSON(rows); err != nil {
					return err
				}
			} else if len(tableRows) > 0 {
				console.Table([]string{"PACKAGE", "CURRENT", "LATEST"}, tableRows)
			}
			console.Success("%s", outdatedSummary(outdated, failed, len(checked)-outdated-failed, skipped))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"check this package (repeatable; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit every checked package as JSON")
	return cmd
}

func checkOutdated(ctx context.Context, pkg *spec.Package, explicit bool) outdatedRow {
	row := outdatedRow{Name: pkg.Name}
	if len(pkg.Versions) > 0 {
		row.Current = pkg.Versions[0].Version
	}
	task := console.Task("Checking " + pkg.Name)
	task.Segment("discovering")
	latest, err := discoverLatest(ctx, pkg)
	if err != nil {
		row.Error = err.Error()
		console.Warn("%s: discovery failed: %v", pkg.Name, err)
		task.Fail("Failed " + pkg.Name)
		return row
	}
	row.Latest = latest
	row.Outdated = spec.CompareVersions(latest, row.Current) > 0
	switch {
	case row.Outdated:
		task.Done(fmt.Sprintf("%s %s → %s available", pkg.Name, orDash(row.Current), latest))
	case explicit:
		task.Done(fmt.Sprintf("%s up to date (%s)", pkg.Name, orDash(row.Current)))
	default:
		task.Discard()
	}
	return row
}

func outdatedSummary(outdated, failed, upToDate, skipped int) string {
	var parts []string
	if outdated > 0 {
		parts = append(parts, fmt.Sprintf("%d outdated", outdated))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if upToDate > 0 {
		parts = append(parts, fmt.Sprintf("%d up to date", upToDate))
	}
	return countSummary("Nothing to check", parts, skipped)
}

func countSummary(empty string, parts []string, skipped int) string {
	msg := strings.Join(parts, ", ")
	if msg == "" {
		msg = empty
	}
	if skipped > 0 {
		msg += fmt.Sprintf(" (%d without discovery)", skipped)
	}
	return msg
}
