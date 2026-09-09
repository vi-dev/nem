package publish

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"

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

func Lint(dir string) ([]Finding, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", dir, err)
	}

	if !info.IsDir() {
		findings := lintPackage("", dir, false)
		sortFindings(findings)
		return findings, nil
	}

	manifests, err := Manifests(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Finding{{Msg: "no pkgs directory found"}}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(manifests) == 0 {
		return []Finding{{Msg: "pkgs directory contains no packages"}}, nil
	}

	var findings []Finding
	for _, m := range manifests {
		findings = append(findings, lintPackage(m.Pkg, m.Path, true)...)
	}

	sortFindings(findings)
	return findings, nil
}

func lintPackage(id, path string, checkDirName bool) []Finding {
	data, err := os.ReadFile(path)
	if err != nil {
		return []Finding{{Pkg: id, Msg: fmt.Sprintf("read %s: %v", path, err)}}
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return []Finding{{Pkg: id, Msg: err.Error()}}
	}

	var findings []Finding
	if err := pkg.Validate(); err != nil {
		findings = append(findings, Finding{Pkg: id, Msg: err.Error()})
	}
	if checkDirName && pkg.Name != "" && pkg.Name != id {
		findings = append(findings, Finding{
			Pkg: id,
			Msg: fmt.Sprintf("name %q does not match its directory %q", pkg.Name, id),
		})
	}
	findings = append(findings, lintTemplates(id, pkg)...)
	findings = append(findings, lintReservedEnv(id, pkg)...)
	findings = append(findings, lintVersionOrder(id, pkg)...)
	findings = append(findings, lintTestSteps(id, pkg)...)
	return findings
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
	if len(pkg.Test) == 0 {
		return nil
	}
	supported := pkg.SupportedBy()
	var findings []Finding
	for i, s := range pkg.Test {
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
				Msg: fmt.Sprintf("test[%d] applies to none of the package's platforms", i),
			})
		}
	}
	return findings
}

func lintTemplates(id string, pkg *spec.Package) []Finding {
	var findings []Finding
	for _, v := range pkg.Versions {
		for _, plat := range pkg.SupportedBy() {
			if _, err := pkg.ArtifactURL(v.Version, plat); err != nil {
				findings = append(findings, Finding{
					Pkg: id,
					Msg: fmt.Sprintf("artifact template for %s on %s: %v", v.Version, plat, err),
				})
			}
			if pkg.Artifact.GitHub != nil {
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
	for _, e := range pkg.Env {
		if envx.IsReserved(e.Name) {
			findings = append(findings, Finding{
				Pkg: id,
				Msg: fmt.Sprintf("env %q is a reserved variable name", e.Name),
			})
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
