package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/project"
)

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "info [<catalog>:]<pkg>",
		Short:       "Show a package's details and available versions",
		Annotations: guide("Catalogs", "/docs/using/catalogs"),
		Long: "Print a package's description, homepage, license, supported platforms, and " +
			"executables, the catalog it comes from, and every version that catalog offers. " +
			"A <catalog>: prefix restricts the lookup to one catalog; without it, the first " +
			"catalog in configured order that has the package answers.",
		Example: "  nem info kubectl             # from the first catalog that has it\n" +
			"  nem info corp:kubectl        # from one catalog",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: firstArgOnly(completeAvailablePackages),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInfo(cmd, args[0])
		},
	}
}

func runInfo(cmd *cobra.Command, arg string) error {
	key, err := project.ParseToolKey(arg)
	if err != nil {
		return err
	}
	cfg, err := config.OpenConfig(nemHome)
	if err != nil {
		return err
	}
	set, err := catalog.OpenConfigured(cfg, nemHome)
	if err != nil {
		return err
	}
	hit, err := set.Lookup(cmd.Context(), key)
	if err != nil {
		return err
	}
	pkg, catalogName := hit.Pkg, hit.Entry.Name

	platforms := "all"
	if len(pkg.Platforms) > 0 {
		ss := make([]string, len(pkg.Platforms))
		for i, p := range pkg.Platforms {
			ss[i] = p.String()
		}
		platforms = strings.Join(ss, ", ")
	}
	versions := make([]string, len(pkg.Versions))
	for i, v := range pkg.Versions {
		versions[i] = v.Version
	}

	fields := []struct{ label, value string }{
		{"name", pkg.Name},
		{"catalog", catalogName},
		{"description", pkg.Description},
		{"homepage", pkg.Homepage},
		{"license", pkg.License},
		{"platforms", platforms},
		{"bins", strings.Join(pkg.Bins, ", ")},
		{"versions", strings.Join(versions, ", ")},
	}
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		console.Data("%-12s %s\n", f.label+":", f.value)
	}
	return nil
}
