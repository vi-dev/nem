package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/envx"
	"github.com/vi-dev/nem/internal/spec"
)

type Finding struct {
	Pkg string
	Msg string
}

func (f Finding) String() string {
	if f.Pkg == "" {
		return f.Msg
	}
	return f.Pkg + ": " + f.Msg
}

func Lint(ctx context.Context, cat string, packages ...string) ([]Finding, error) {
	entry, err := catalog.Open(ctx, cat)
	if err != nil {
		return nil, err
	}
	if _, ok := entry.Catalog.(*catalog.File); ok {
		return lintFile(ctx, cat, packages)
	}

	names, err := entry.Catalog.PackageNames(ctx)
	if err != nil {
		return nil, err
	}
	if len(packages) > 0 {
		var selected []string
		for _, name := range packages {
			if slices.Contains(selected, name) {
				continue
			}
			if !slices.Contains(names, name) {
				return nil, &catalog.PackageNotFoundError{Name: name, Catalogs: []string{cat}}
			}
			selected = append(selected, name)
		}
		var findings []Finding
		for _, name := range selected {
			files, err := entry.Catalog.ReadFiles(ctx, name)
			if err != nil {
				return nil, err
			}
			findings = append(findings, lintPackage(name, files)...)
		}
		findings = append(findings, lintExcludedDeps(ctx, entry.Catalog, selected)...)
		sortFindings(findings)
		return findings, nil
	}

	if len(names) == 0 {
		if _, ok := entry.Catalog.(*catalog.Dir); ok {
			if _, err := os.Stat(filepath.Join(cat, "pkgs")); err != nil {
				return []Finding{{Msg: "no pkgs directory found"}}, nil
			}
			return []Finding{{Msg: "pkgs directory contains no packages"}}, nil
		}
		return []Finding{{Msg: "catalog contains no packages"}}, nil
	}
	var findings []Finding
	for _, name := range names {
		files, err := entry.Catalog.ReadFiles(ctx, name)
		if err != nil {
			findings = append(findings, Finding{Pkg: name, Msg: err.Error()})
			continue
		}
		findings = append(findings, lintPackage(name, files)...)
	}
	findings = append(findings, lintExcludedDeps(ctx, entry.Catalog, names)...)
	sortFindings(findings)
	return findings, nil
}

func lintFile(ctx context.Context, path string, packages []string) ([]Finding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	declared := ""
	if pkg, err := spec.Parse(data); err == nil {
		declared = pkg.Name
	}
	if len(packages) > 0 {
		for _, name := range packages {
			if name != declared {
				return nil, &catalog.PackageNotFoundError{Name: name}
			}
		}
	}
	files, err := catalog.NewFile(path).ReadFiles(ctx, declared)
	if err != nil {
		return nil, err
	}
	findings := lintPackage("", files)
	sortFindings(findings)
	return findings, nil
}

func lintPackage(id string, files spec.Files) []Finding {
	pkg, err := spec.ParseFiles(files)
	if err != nil {
		return []Finding{{Pkg: id, Msg: err.Error()}}
	}

	var findings []Finding
	if err := pkg.Validate(); err != nil {
		findings = append(findings, Finding{Pkg: id, Msg: err.Error()})
	}
	if id != "" && pkg.Name != "" && pkg.Name != id {
		findings = append(findings, Finding{
			Pkg: id,
			Msg: fmt.Sprintf("name %q does not match its directory %q", pkg.Name, id),
		})
	}
	findings = append(findings, lintTemplates(id, pkg)...)
	findings = append(findings, lintReservedEnv(id, pkg)...)
	findings = append(findings, lintVersionOrder(id, pkg)...)
	findings = append(findings, lintTestSteps(id, pkg)...)
	findings = append(findings, lintRecipes(id, pkg)...)
	findings = append(findings, lintSchema(id, pkg)...)
	return findings
}

func lintSchema(id string, pkg *spec.Package) []Finding {
	if pkg.Schema < spec.SchemaExcluded || len(pkg.Offered()) < len(pkg.Versions) {
		return nil
	}
	return []Finding{{Pkg: id, Msg: fmt.Sprintf("schema %d is only needed for excluded versions; lower it to 3 to keep the package digest stable", pkg.Schema)}}
}

// lintExcludedDeps reports exact dependencies, from the named packages, on
// versions their own catalog excludes: every install of the dependent would
// fail at resolve time.
func lintExcludedDeps(ctx context.Context, cat catalog.Catalog, names []string) []Finding {
	pkgs := map[string]*spec.Package{}
	load := func(name string) *spec.Package {
		if pkg, ok := pkgs[name]; ok {
			return pkg
		}
		var pkg *spec.Package
		if files, err := cat.ReadFiles(ctx, name); err == nil {
			pkg, _ = spec.ParseFiles(files)
		}
		pkgs[name] = pkg
		return pkg
	}
	var findings []Finding
	for _, name := range names {
		pkg := load(name)
		if pkg == nil {
			continue
		}
		seen := map[*spec.Recipe]bool{}
		for _, v := range pkg.Offered() {
			r, err := pkg.Recipe(v.Version)
			if err != nil || seen[r] {
				continue
			}
			seen[r] = true
			deps := slices.Clone(r.Deps)
			if r.Build != nil {
				deps = append(deps, r.Build.Deps...)
			}
			for _, d := range deps {
				if d.Version == "" {
					continue
				}
				dep := load(d.Name)
				if dep == nil {
					continue
				}
				if ex := dep.Exclusion(d.Version); ex != nil {
					findings = append(findings, Finding{Pkg: name,
						Msg: fmt.Sprintf("%sdepends on %s@%s, which is excluded: %s", recipeLabel(r), d.Name, d.Version, ex.Reason)})
				}
			}
		}
	}
	return findings
}

func lintRecipes(id string, pkg *spec.Package) []Finding {
	var findings []Finding
	covered := map[string]int{}
	for _, v := range pkg.Versions {
		r, err := pkg.Recipe(v.Version)
		if err != nil {
			continue
		}
		covered[r.Prefix]++
	}
	for _, f := range pkg.Frozen {
		if covered[f.Prefix] == 0 {
			findings = append(findings, Finding{Pkg: id,
				Msg: fmt.Sprintf("%s covers no listed version; delete it", spec.RecipeFileName(f.Prefix))})
		}
	}
	if latest := pkg.Latest(); latest != "" {
		if r := pkg.LatestRecipe(); r.Prefix != "" {
			findings = append(findings, Finding{Pkg: id,
				Msg: fmt.Sprintf("%s does not cover the latest version %s (%s does)",
					spec.PkgFileName, latest, spec.RecipeFileName(r.Prefix))})
		}
	}
	return findings
}

func recipeLabel(r *spec.Recipe) string {
	if r.Prefix == "" {
		return ""
	}
	return spec.RecipeFileName(r.Prefix) + ": "
}

func allRecipes(pkg *spec.Package) []*spec.Recipe {
	recipes := []*spec.Recipe{&pkg.Base}
	for i := range pkg.Frozen {
		recipes = append(recipes, &pkg.Frozen[i])
	}
	return recipes
}

func lintVersionOrder(id string, pkg *spec.Package) []Finding {
	var findings []Finding
	for i := 1; i < len(pkg.Versions); i++ {
		prev, cur := pkg.Versions[i-1].Version, pkg.Versions[i].Version
		if spec.CompareVersions(prev, cur) <= 0 {
			findings = append(findings, Finding{
				Pkg: id,
				Msg: fmt.Sprintf("versions are not newest-first: %s precedes %s", prev, cur),
			})
		}
	}
	return findings
}

func lintTestSteps(id string, pkg *spec.Package) []Finding {
	var findings []Finding
	for _, r := range allRecipes(pkg) {
		supported := r.SupportedBy()
		for i, s := range r.Test {
			if len(s.Platforms) == 0 {
				continue
			}
			dead := true
			for _, plat := range supported {
				if spec.PlatformsInclude(s.Platforms, plat) {
					dead = false
					break
				}
			}
			if dead {
				findings = append(findings, Finding{
					Pkg: id,
					Msg: fmt.Sprintf("%stest[%d] applies to none of the package's platforms", recipeLabel(r), i),
				})
			}
		}
	}
	return findings
}

func lintTemplates(id string, pkg *spec.Package) []Finding {
	var findings []Finding
	for _, v := range pkg.Versions {
		r, err := pkg.Recipe(v.Version)
		if err != nil {
			continue
		}
		for _, plat := range r.SupportedBy() {
			if _, err := pkg.ArtifactURL(v.Version, plat); err != nil {
				findings = append(findings, Finding{
					Pkg: id,
					Msg: fmt.Sprintf("artifact template for %s on %s: %v", v.Version, plat, err),
				})
			}
			if r.Artifact.GitHub != nil {
				if _, err := pkg.AssetName(v.Version, plat); err != nil {
					findings = append(findings, Finding{
						Pkg: id,
						Msg: fmt.Sprintf("asset template for %s on %s: %v", v.Version, plat, err),
					})
				}
			}
		}
	}
	return findings
}

func lintReservedEnv(id string, pkg *spec.Package) []Finding {
	var findings []Finding
	for _, r := range allRecipes(pkg) {
		for _, e := range r.Env {
			if envx.IsReserved(e.Name) {
				findings = append(findings, Finding{
					Pkg: id,
					Msg: fmt.Sprintf("%senv %q is a reserved variable name", recipeLabel(r), e.Name),
				})
			}
		}
	}
	return findings
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Pkg != findings[j].Pkg {
			return findings[i].Pkg < findings[j].Pkg
		}
		return findings[i].Msg < findings[j].Msg
	})
}
