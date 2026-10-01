package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

var syncCatalog = func(ctx context.Context, ref, storePath string, progress ocix.ProgressFunc) (string, error) {
	src, srcRef, err := ocix.RemoteCatalog(ref)
	if err != nil {
		return "", err
	}
	store, err := ocix.SyncLocalCatalog(ctx, src, srcRef, storePath, progress)
	if err != nil {
		return "", err
	}
	return store.Digest(), nil
}

const (
	catalogGroupConsumption = "consumption"
	catalogGroupMaintenance = "maintenance"
)

func newCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "catalog",
		Aliases:     []string{"cat"},
		Short:       "Manage catalogs",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Catalogs are the ordered sources nem resolves packages from. The consumption " +
			"commands edit the list in config.yaml; the maintenance commands work on a " +
			"catalog's contents, whether a local directory of package manifests or a " +
			"published OCI reference.",
		Example: "  nem catalog list                                        # what is configured\n" +
			"  nem catalog add corp registry.example/nem/catalog:v2    # add one\n" +
			"  nem catalog lint .                                      # validate a checkout",
	}
	cmd.AddGroup(
		&cobra.Group{ID: catalogGroupConsumption, Title: "Catalog consumption:"},
		&cobra.Group{ID: catalogGroupMaintenance, Title: "Catalog maintenance:"},
	)
	addGrouped(cmd, catalogGroupConsumption, newCatalogAddCmd(), newCatalogListCmd(), newCatalogRemoveCmd(), newCatalogUpdateCmd(), newCatalogReorderCmd(), newCatalogDisableCmd(), newCatalogEnableCmd())
	addGrouped(cmd, catalogGroupMaintenance, newCatalogLintCmd(), newCatalogFmtCmd(), newCatalogBuildCmd(), newCatalogTestCmd(), newCatalogBumpCmd(), newCatalogOutdatedCmd(), newCatalogDiffCmd(), newCatalogPublishCmd(), newCatalogMirrorCmd(), newCatalogFillCmd())
	return cmd
}

func newCatalogAddCmd() *cobra.Command {
	var typeFlag string
	cmd := &cobra.Command{
		Use:         "add <name> <ref>",
		Short:       "Add a catalog",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Append a catalog to config.yaml under <name>, after the ones already configured. " +
			"<ref> is an OCI reference or a local directory; the type is detected from it " +
			"unless --type says otherwise. nem use and nem lock sync a new oci catalog when " +
			"they need it; nem catalog update syncs it right away.",
		Example: "  nem catalog add corp registry.example/nem/catalog:v2             # an OCI catalog\n" +
			"  nem catalog add corp registry.example/nem/catalog@sha256:0123...  # pinned to a digest\n" +
			"  nem catalog add local ./my-catalog                                # a directory",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, ref := args[0], args[1]
			entryType := typeFlag
			if entryType == "" {
				if looksLikeDir(ref) {
					entryType = "dir"
				} else {
					entryType = "oci"
				}
			}
			release, err := fsx.Lock(nemHome.LockFile())
			if err != nil {
				return err
			}
			defer release()
			cfg, err := config.OpenConfig(nemHome)
			if err != nil {
				return err
			}
			if cfg.Find(name) != nil {
				return fmt.Errorf("catalog %s already exists", name)
			}
			entry := config.CatalogEntry{Name: name, Type: entryType}
			switch entryType {
			case "dir":
				abs, err := filepath.Abs(ref)
				if err != nil {
					return err
				}
				entry.Path = abs
			case "oci":
				if err := ocix.WithTagOrDigest(ref); err != nil {
					return err
				}
				entry.Ref = ref
			default:
				return fmt.Errorf("invalid --type %q (want oci or dir)", entryType)
			}
			cfg.Catalogs = append(cfg.Catalogs, entry)
			if err := config.SaveConfig(nemHome, cfg); err != nil {
				return err
			}
			console.Success("Added catalog %s", name)
			if entryType == "oci" {
				console.Hint("Run `nem catalog update " + name + "` to sync it")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeFlag, "type", "", "catalog type: oci or dir (default: auto-detect)")
	_ = cmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions([]string{"oci", "dir"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func newCatalogListCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Aliases:     []string{"ls"},
		Short:       "List configured catalogs",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Print every configured catalog in precedence order with its type, source, and " +
			"status.",
		Example: "  nem catalog list             # precedence order, first wins",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.OpenConfig(nemHome)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(cfg.Catalogs))
			for _, e := range cfg.Catalogs {
				source := e.Ref
				if source == "" {
					source = e.Path
				}
				status := "enabled"
				if e.Disabled {
					status = "disabled"
				}
				rows = append(rows, []string{e.Name, e.Type, source, status})
			}
			console.Table([]string{"NAME", "TYPE", "SOURCE", "STATUS"}, rows)
			return nil
		},
	}
}

func newCatalogRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "remove <name>",
		Aliases:     []string{"rm"},
		Short:       "Remove a catalog",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Delete a catalog from config.yaml. Packages already installed from it stay under " +
			"NEM_HOME, but projects whose lockfile pins packages from it cannot sync them " +
			"until the catalog is added back.",
		Example:           "  nem catalog remove corp      # forget it",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: firstArgOnly(completeCatalogNames(anyCatalog)),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			release, err := fsx.Lock(nemHome.LockFile())
			if err != nil {
				return err
			}
			defer release()
			cfg, err := config.OpenConfig(nemHome)
			if err != nil {
				return err
			}
			idx := slices.IndexFunc(cfg.Catalogs, func(e config.CatalogEntry) bool { return e.Name == name })
			if idx == -1 {
				return fmt.Errorf("catalog %s is not configured", name)
			}

			dir, err := nemHome.CatalogDir(name)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			cfg.Catalogs = slices.Delete(cfg.Catalogs, idx, idx+1)
			if err := config.SaveConfig(nemHome, cfg); err != nil {
				return err
			}
			console.Success("Removed catalog %s", name)
			return nil
		},
	}
}

func newCatalogUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "update [name]",
		Aliases:     []string{"up"},
		Short:       "Sync oci catalogs from their remote",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Pull the current index of each oci catalog from its registry into NEM_HOME so " +
			"that resolution sees new packages and versions. Without a name, every enabled " +
			"oci catalog is synced. dir catalogs are read live and need no sync.",
		Example: "  nem catalog update           # every enabled oci catalog\n" +
			"  nem catalog update official  # one catalog",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeCatalogNames(func(e config.CatalogEntry) bool { return e.Type == "oci" && !e.Disabled })),
		RunE: func(cmd *cobra.Command, args []string) error {
			release, err := fsx.Lock(nemHome.LockFile())
			if err != nil {
				return err
			}
			defer release()
			cfg, err := config.OpenConfig(nemHome)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				name := args[0]
				entry := cfg.Find(name)
				if entry == nil {
					return fmt.Errorf("catalog %s is not configured", name)
				}
				if entry.Type == "dir" {
					console.Warn("Catalog %s is a dir catalog (nothing to sync)", name)
					return nil
				}
				if entry.Disabled {
					console.Warn("Catalog %s is disabled (nothing to sync)", name)
					return nil
				}
				return syncOne(cmd.Context(), *entry)
			}
			for _, e := range cfg.Catalogs {
				if e.Disabled {
					continue
				}
				if e.Type != "oci" {
					continue
				}
				if err := syncOne(cmd.Context(), e); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func syncOne(ctx context.Context, e config.CatalogEntry) error {
	store, err := nemHome.CatalogStore(e.Name)
	if err != nil {
		return err
	}
	labels := report.TaskLabels{
		Run:     "Syncing catalog " + e.Name,
		Segment: "copying",
		Done:    "Synced catalog " + e.Name,
		Fail:    "Failed to sync catalog " + e.Name,
	}
	return report.RunTask(ctx, labels, func(count report.ProgressFunc) error {
		_, err := syncCatalog(ctx, e.Ref, store, count)
		return err
	})
}

func newCatalogReorderCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "reorder <name>...",
		Short:       "Reorder catalog precedence",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Rewrite the precedence list. Name every configured catalog exactly once, first " +
			"to last; a lookup without a <catalog>: prefix stops at the first catalog that " +
			"has the package.",
		Example:           "  nem catalog reorder corp official   # corp wins over official",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeCatalogNames(anyCatalog),
		RunE: func(cmd *cobra.Command, args []string) error {
			release, err := fsx.Lock(nemHome.LockFile())
			if err != nil {
				return err
			}
			defer release()
			cfg, err := config.OpenConfig(nemHome)
			if err != nil {
				return err
			}
			if err := cfg.Reorder(args); err != nil {
				return err
			}
			if err := config.SaveConfig(nemHome, cfg); err != nil {
				return err
			}
			console.Success("Reordered catalogs")
			return nil
		},
	}
}

func newCatalogDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "disable <name>...",
		Short:       "Disable configured catalogs",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Skip the named catalogs during lookups while keeping their place in the " +
			"precedence order. nem catalog enable reverses it.",
		Example:           "  nem catalog disable official        # stop resolving from it",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeCatalogNames(func(e config.CatalogEntry) bool { return !e.Disabled }),
		RunE:              func(cmd *cobra.Command, args []string) error { return setCatalogsDisabled(args, true) },
	}
}

func newCatalogEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "enable <name>...",
		Short:       "Enable configured catalogs",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Include the named catalogs in lookups again, in the place they kept while " +
			"disabled.",
		Example:           "  nem catalog enable official         # resolve from it again",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeCatalogNames(func(e config.CatalogEntry) bool { return e.Disabled }),
		RunE:              func(cmd *cobra.Command, args []string) error { return setCatalogsDisabled(args, false) },
	}
}

func setCatalogsDisabled(names []string, disabled bool) error {
	release, err := fsx.Lock(nemHome.LockFile())
	if err != nil {
		return err
	}
	defer release()
	cfg, err := config.OpenConfig(nemHome)
	if err != nil {
		return err
	}

	for _, name := range names {
		entry := cfg.Find(name)
		if entry == nil {
			return fmt.Errorf("catalog %s is not configured", name)
		}
		entry.Disabled = disabled
	}
	if err := config.SaveConfig(nemHome, cfg); err != nil {
		return err
	}
	verb := "Disabled"
	if !disabled {
		verb = "Enabled"
	}
	for _, name := range names {
		console.Success("%s catalog %s", verb, name)
	}
	return nil
}

func looksLikeDir(ref string) bool {
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") {
		return true
	}
	info, err := os.Stat(ref)
	return err == nil && info.IsDir()
}

func parsePackageRefs(packages []string) ([]spec.Ref, error) {
	refs := make([]spec.Ref, 0, len(packages))
	for _, p := range packages {
		ref, err := spec.ParseRef(p)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
