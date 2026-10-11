package spec

import (
	"fmt"
	"reflect"
	"strings"
)

const PkgFileName = "pkg.yaml"

type Recipe struct {
	Prefix    string
	Platforms []Platform
	Deps      []Dep
	Artifact  Artifact
	Install   []Action
	Bins      []string
	Libs      []string
	Env       []EnvExport
	Build     *Build
	Test      []TestStep
}

func (r *Recipe) SupportedBy() []Platform {
	if len(r.Platforms) == 0 {
		return SupportedPlatforms
	}
	var out []Platform
	for _, sup := range SupportedPlatforms {
		for _, c := range r.Platforms {
			if c.Matches(sup) {
				out = append(out, sup)
				break
			}
		}
	}
	return out
}

func (r *Recipe) Equal(o *Recipe) bool {
	a, b := *r, *o
	a.Prefix, b.Prefix = "", ""
	return reflect.DeepEqual(a, b)
}

func versionComponents(s string) []string {
	return strings.Split(strings.TrimPrefix(s, "v"), ".")
}

func MatchesPrefix(version, prefix string) bool {
	vs, ps := versionComponents(version), versionComponents(prefix)
	if len(ps) > len(vs) {
		return false
	}
	for i := range ps {
		if vs[i] != ps[i] {
			return false
		}
	}
	return true
}

func RecipeFileName(prefix string) string { return "recipe@" + prefix + ".yaml" }

func ParseRecipeFileName(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, "recipe@")
	if !ok {
		return "", false
	}
	prefix, ok := strings.CutSuffix(rest, ".yaml")
	if !ok || !CompatRE.MatchString(prefix) {
		return "", false
	}
	return prefix, true
}

func (p *Package) selectRecipe(version string) *Recipe {
	best, bestN := &p.Base, 0
	for i := range p.Frozen {
		f := &p.Frozen[i]
		if n := len(versionComponents(f.Prefix)); MatchesPrefix(version, f.Prefix) && n > bestN {
			best, bestN = f, n
		}
	}
	return best
}

func (p *Package) Recipe(version string) (*Recipe, error) {
	if !p.HasEqualVersion(version) {
		return nil, fmt.Errorf("version %s not offered by %s", version, p.Name)
	}
	return p.selectRecipe(version), nil
}

func (p *Package) LatestRecipe() *Recipe {
	if len(p.Versions) == 0 {
		return &p.Base
	}
	return p.selectRecipe(p.Versions[0].Version)
}
