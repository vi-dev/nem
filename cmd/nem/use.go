package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

var syncCatalogStore = func(ctx context.Context, ref, storePath string, progress ocix.ProgressFunc) error {
	src, srcRef, err := ocix.RemoteCatalog(ref)
	if err != nil {
		return err
	}
	_, err = ocix.SyncLocalCatalog(ctx, src, srcRef, storePath, progress)
	return err
}

func newUseCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:               "use [<catalog>:]<pkg>[@<version>]...",
		Short:             "Declare and install tools",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeUseArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUse(cmd, args, global)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "target the global manifest")
	return cmd
}

func newUnuseCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "unuse <pkg>...",
		Short: "Remove declared tools",
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeDeclaredPackages(global, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnuse(cmd, args, global)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "target the global manifest")
	return cmd
}

type mirrorOpener interface {
	Open(ctx context.Context) error
}

type useArg struct {
	Key     project.ToolKey
	Version string
}

func parseUseArg(arg string) (useArg, error) {
	keyPart, version := arg, ""
	if i := strings.LastIndex(arg, "@"); i != -1 {
		keyPart, version = arg[:i], arg[i+1:]
	}
	key, err := project.ParseToolKey(keyPart)
	if err != nil {
		return useArg{}, err
	}
	return useArg{Key: key, Version: version}, nil
}

func manifestPath(global, createIfMissing bool) (string, error) {
	if global {
		return nemHome.GlobalManifest(), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err := project.Discover(cwd)
	if err != nil {
		if createIfMissing && errors.Is(err, project.ErrNoManifest) {
			return filepath.Join(cwd, "nem.toml"), nil
		}
		return "", err
	}
	return filepath.Join(dir, "nem.toml"), nil
}

func requireGlobalManifest(global bool, path string) error {
	if !global {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w at %s", project.ErrNoManifest, path)
	}
	return nil
}

func loadUseState(path string) (*project.Manifest, *config.Config, []catalog.Named, error) {
	manifest, err := project.LoadManifest(path)
	if err != nil {
		return nil, nil, nil, err
	}
	cfg, err := config.OpenConfig(nemHome)
	if err != nil {
		return nil, nil, nil, err
	}
	sources, err := catalog.Open(cfg, nemHome)
	if err != nil {
		return nil, nil, nil, err
	}
	return manifest, cfg, sources, nil
}

func manifestTools(m *project.Manifest) []resolve.Tool {
	tools := make([]resolve.Tool, len(m.Tools))
	for i, t := range m.Tools {
		tools[i] = resolve.Tool{Key: t.Key, Version: t.Version}
	}
	return tools
}

func writeManifestAndLock(manifest *project.Manifest, result *resolve.Result) error {
	if err := project.WriteManifest(manifest); err != nil {
		return err
	}
	return writeLock(manifest, result)
}

func lockPathFor(manifestPath string) string {
	return filepath.Join(filepath.Dir(manifestPath), "nem.lock")
}

func writeLock(manifest *project.Manifest, result *resolve.Result) error {
	lf := &project.Lockfile{Path: lockPathFor(manifest.Path), Packages: result.Entries}
	return project.WriteLock(lf)
}

func currentPlatformJobs(cfg *config.Config, result *resolve.Result) []install.Job {
	current := spec.Current().String()
	var jobs []install.Job
	for _, entry := range result.Entries {
		if !slices.Contains(entry.Platforms, current) {
			continue
		}
		ref := ""
		if e := cfg.Find(entry.Catalog); e != nil && e.Type == "oci" {
			ref = e.Ref
		}
		jobs = append(jobs, install.Job{
			Pkg:     result.Pkgs[entry.Name],
			Version: entry.Version,
			Catalog: entry.Catalog,
			Source:  fetch.Source{CatalogRef: ref},
		})
	}
	return jobs
}

func autoSyncUnsyncedCatalogs(ctx context.Context, cfg *config.Config, sources []catalog.Named) error {
	for _, n := range sources {
		src, ok := n.Source.(mirrorOpener)
		if !ok {
			continue
		}
		err := src.Open(ctx)
		if err == nil {
			continue
		}
		if !errors.Is(err, ocix.ErrNotSynced) {
			return err
		}
		e := cfg.Find(n.Name)
		if e == nil {
			continue
		}
		store, err := nemHome.CatalogStore(n.Name)
		if err != nil {
			return err
		}
		labels := report.TaskLabels{
			Run:    "Syncing catalog " + n.Name,
			Status: "copying",
			Done:   "Synced catalog " + n.Name,
			Fail:   "Sync failed",
		}
		if err := report.RunTask(console, labels, func(count func(done, total int64)) error {
			return syncCatalogStore(ctx, e.Ref, store, count)
		}); err != nil {
			console.Warn("Could not sync catalog %s: %v", n.Name, err)
			continue
		}
	}
	return nil
}

func resolveManifest(cmd *cobra.Command, manifest *project.Manifest, cfg *config.Config, sources []catalog.Named) (*resolve.Result, error) {
	if err := autoSyncUnsyncedCatalogs(cmd.Context(), cfg, sources); err != nil {
		return nil, err
	}
	return resolve.Resolve(cmd.Context(), manifestTools(manifest), sources)
}

func resolvedVersions(result *resolve.Result) map[string]string {
	m := make(map[string]string, len(result.Entries))
	for _, e := range result.Entries {
		m[e.Name] = e.Version
	}
	return m
}

func pinResolved(manifest *project.Manifest, result *resolve.Result, keys []project.ToolKey) error {
	resolved := resolvedVersions(result)
	for _, k := range keys {
		if v, ok := resolved[k.Name]; ok {
			project.AddTool(manifest, k, v)
		}
	}
	return writeManifestAndLock(manifest, result)
}

func runUse(cmd *cobra.Command, args []string, global bool) error {
	parsedArgs := make([]useArg, len(args))
	for i, a := range args {
		p, err := parseUseArg(a)
		if err != nil {
			return err
		}
		parsedArgs[i] = p
	}

	release, err := fsx.Lock(nemHome.LockFile())
	if err != nil {
		return err
	}

	path, err := manifestPath(global, true)
	if err != nil {
		release()
		return err
	}
	manifest, cfg, sources, err := loadUseState(path)
	if err != nil {
		release()
		return err
	}

	keys := make([]project.ToolKey, len(parsedArgs))
	for i, p := range parsedArgs {
		project.AddTool(manifest, p.Key, p.Version)
		keys[i] = p.Key
	}

	result, err := resolveManifest(cmd, manifest, cfg, sources)
	if err != nil {
		release()
		return err
	}
	if err := pinResolved(manifest, result, keys); err != nil {
		release()
		return err
	}
	release()

	return install.Run(cmd.Context(), nemHome, console, currentPlatformJobs(cfg, result))
}

func runUnuse(cmd *cobra.Command, args []string, global bool) error {
	release, err := fsx.Lock(nemHome.LockFile())
	if err != nil {
		return err
	}
	defer release()

	path, err := manifestPath(global, false)
	if err != nil {
		return err
	}
	manifest, _, sources, err := loadUseState(path)
	if err != nil {
		return err
	}

	for _, name := range args {
		if !project.RemoveTool(manifest, name) {
			return fmt.Errorf("package %s is not declared", name)
		}
	}

	result, err := resolve.Resolve(cmd.Context(), manifestTools(manifest), sources)
	if err != nil {
		return err
	}

	return writeManifestAndLock(manifest, result)
}
