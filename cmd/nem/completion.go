package main

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/project"
)

func completeCatalogNames(filter func(config.CatalogEntry) bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, err := config.OpenConfigReadOnly(nemHome)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		used := make(map[string]bool, len(args))
		for _, a := range args {
			used[a] = true
		}
		var out []string
		for _, e := range cfg.Catalogs {
			if used[e.Name] || !filter(e) || !strings.HasPrefix(e.Name, toComplete) {
				continue
			}
			out = append(out, e.Name)
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

func anyCatalog(_ config.CatalogEntry) bool { return true }

func completionSources() (*catalog.Set, bool) {
	cfg, err := config.OpenConfigReadOnly(nemHome)
	if err != nil {
		return nil, false
	}
	set, err := catalog.OpenConfigured(cfg, nemHome)
	if err != nil {
		return nil, false
	}
	return set, true
}

func firstArgOnly(fn func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return fn(cmd, args, toComplete)
	}
}

func completeDeclaredPackages(global bool, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	path, err := manifestPath(global, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	manifest, err := project.LoadManifest(path)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	used := make(map[string]bool, len(args))
	for _, a := range args {
		if key, err := project.ParseToolKey(a); err == nil {
			used[key.Name] = true
		} else {
			used[a] = true
		}
	}
	var out []string
	for _, tool := range manifest.Tools {
		if used[tool.Key.Name] || !strings.HasPrefix(tool.Key.Name, toComplete) {
			continue
		}
		out = append(out, tool.Key.Name)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeAvailablePackages(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	set, ok := completionSources()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	catalogPrefix, namePrefix := "", toComplete
	if c, rest, ok := strings.Cut(toComplete, ":"); ok {
		catalogPrefix, namePrefix = c, rest
	}

	used := make(map[string]bool, len(args))
	for _, a := range args {
		if p, err := parseUseArg(a); err == nil {
			used[p.Key.Name] = true
		}
	}

	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if seen[name] || used[name] || !strings.HasPrefix(name, namePrefix) {
			return
		}
		seen[name] = true
		s := name
		if catalogPrefix != "" {
			s = catalogPrefix + ":" + name
		}
		out = append(out, s)
	}

	if catalogPrefix != "" {
		e, ok := set.Find(catalogPrefix)
		if !ok {
			return out, cobra.ShellCompDirectiveNoFileComp
		}
		names, err := e.Catalog.PackageNames(cmd.Context())
		if err != nil {
			return out, cobra.ShellCompDirectiveNoFileComp
		}
		for _, name := range names {
			add(name)
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}

	for _, name := range set.PackageNames(cmd.Context()) {
		add(name)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeSearchQuery(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if strings.Contains(toComplete, ":") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeAvailablePackages(cmd, args, toComplete)
}

func completeUseArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if strings.Contains(toComplete, "@") {
		return completeUseVersions(cmd, toComplete)
	}
	out, directive := completeAvailablePackages(cmd, args, toComplete)
	return out, directive | cobra.ShellCompDirectiveNoSpace
}

func completeUseVersions(cmd *cobra.Command, toComplete string) ([]string, cobra.ShellCompDirective) {
	parsed, err := parseUseArg(toComplete)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	key, versionPrefix := parsed.Key, parsed.Version
	set, ok := completionSources()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	for _, n := range set.Entries() {
		if key.Catalog != "" && n.Name != key.Catalog {
			continue
		}
		versions, err := n.Catalog.Versions(cmd.Context(), key.Name)
		if err != nil {

			var nf *catalog.PackageNotFoundError
			if errors.As(err, &nf) || errors.Is(err, ocix.ErrNotSynced) {
				continue
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var out []string
		for _, v := range versions {
			if strings.HasPrefix(v, versionPrefix) {
				out = append(out, key.String()+"@"+v)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeYAMLFiles(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
}

func completeDirsOnly(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}
