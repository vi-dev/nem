package main

import (
	"maps"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/envx"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

func newStatusCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:         "status",
		Aliases:     []string{"st"},
		Short:       "Show declared packages and composed environment variables",
		Annotations: guide("Packages", "/docs/using/packages"),
		Long: "Print the packages the current scope declares with their declared version, " +
			"catalog, and whether each is locked and installed, then the environment " +
			"variables nem composes for it with the package or manifest each one comes from. " +
			"Inside a project the result is the project layered over the global scope.",
		Example: "  nem status                   # this project, layered over the global scope\n" +
			"  nem status -g                # the global scope alone",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(global)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "show the global scope")
	return cmd
}

func runStatus(global bool) error {
	manifestP, err := manifestPath(global, false)
	if err != nil {
		return err
	}
	m, err := project.LoadManifest(manifestP)
	if err != nil {
		return err
	}

	lock, err := project.LoadLock(lockPathFor(manifestP))
	if err != nil {
		return err
	}

	lockedVersion := project.Versions(lock.Packages)

	var rows [][]string
	for _, tool := range m.Tools {
		catalog := tool.Key.Catalog
		if catalog == "" {
			catalog = "-"
		}
		lockedCell := "no"
		if lock.Covers(tool) {
			lockedCell = "yes"
		}
		installedCell := "-"
		if lv, ok := lockedVersion[tool.Key.Name]; ok {
			installedCell = "no"
			if install.IsInstalled(nemHome, tool.Key.Name, lv) {
				installedCell = "yes"
			}
		}
		rows = append(rows, []string{tool.Key.Name, tool.Version, catalog, lockedCell, installedCell})
	}
	console.Table([]string{"PACKAGE", "VERSION", "CATALOG", "LOCKED", "INSTALLED"}, rows)

	var other *project.Manifest
	var otherLock *project.Lockfile
	if global {
		other, otherLock, err = loadProjectLayer()
		if err != nil {
			return err
		}
	} else {
		other, err = project.LoadManifest(nemHome.GlobalManifest())
		if err != nil {
			return err
		}
		otherLock, err = project.LoadLock(nemHome.GlobalLock())
		if err != nil {
			return err
		}
	}
	projLock, globalLock := lock, otherLock
	if global {
		projLock, globalLock = otherLock, lock
	}
	warnMissingInstalls(projLock, globalLock)
	warnLinkDrift(lock, lockedVersion)

	result := envx.ComposeScope(m, other, lock, otherLock, nemHome, installMetaLookup, os.LookupEnv)
	for _, w := range result.Warnings {
		console.Warn("%s", w)
	}
	if len(result.Vars) > 0 {
		console.Blank()
		var envRows [][]string
		for _, v := range result.Vars {
			envRows = append(envRows, []string{v.Name, v.Value, v.Source})
		}
		console.Table([]string{"VARIABLE", "VALUE", "SOURCE"}, envRows)
	}
	return nil
}

func warnMissingInstalls(projLock, globalLock *project.Lockfile) {
	nProj, nGlobal := countMissing(projLock), countMissing(globalLock)
	if nProj == 0 && nGlobal == 0 {
		return
	}
	console.BlankErr()
	if nProj > 0 {
		console.Warn("%s not installed — run `nem sync`", report.Plural(nProj, "project package"))
	}
	if nGlobal > 0 {
		console.Warn("%s not installed — run `nem sync -g`", report.Plural(nGlobal, "global package"))
	}
}

func countMissing(lock *project.Lockfile) int {
	current := spec.Current().String()
	n := 0
	for _, e := range lock.Packages {
		if slices.Contains(e.Platforms, current) && !install.IsInstalled(nemHome, e.Name, e.Version) {
			n++
		}
	}
	return n
}

func warnLinkDrift(lock *project.Lockfile, locked map[string]string) {
	for _, e := range lock.Packages {
		dir, err := nemHome.PackageDir(e.Name, e.Version)
		if err != nil {
			continue
		}
		have, err := install.ReadLinks(dir)
		if err != nil {
			continue
		}
		for _, dep := range slices.Sorted(maps.Keys(have)) {
			if want := locked[dep]; want != "" && want != have[dep] {
				console.Warn("%s %s: linked to %s %s, this scope locks %s — run `nem sync`", e.Name, e.Version, dep, have[dep], want)
			}
		}
	}
}
