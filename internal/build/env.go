package build

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vi-dev/nem/internal/spec"
)

type ResolvedDep struct {
	Name, Version, Prefix string
	OnPath                bool
	OnLoaderPath          bool
	Bins, Libs            []string
}

type EnvContext struct {
	Version    string
	Platform   spec.Platform
	Prefix     string
	StagingDir string
	OutputDir  string

	AbsRpath bool
}

func ComposeEnv(base []string, deps []ResolvedDep, ectx EnvContext) []string {
	vars := envToMap(base)

	var pathDirs []string
	var cppflags, cflags, ldflags, cgoLdflags, pkgcfg []string
	newDTags := false
	for _, d := range deps {
		vars[depPrefixVar(d.Name)] = d.Prefix
		if d.OnPath || d.OnLoaderPath {
			for _, b := range d.Bins {
				pathDirs = append(pathDirs, filepath.Join(d.Prefix, b))
			}
		}
		if !d.OnLoaderPath {
			continue
		}
		cppflags = append(cppflags, "-I"+filepath.Join(d.Prefix, "include"))
		cflags = append(cflags, "-I"+filepath.Join(d.Prefix, "include"))
		pkgcfg = append(pkgcfg, filepath.Join(d.Prefix, "lib", "pkgconfig"))
		libs := d.Libs
		if len(libs) == 0 {
			libs = []string{"lib"}
		}
		for _, lib := range libs {
			dir := filepath.Join(d.Prefix, lib)
			ldflags = append(ldflags, "-L"+dir)
			cgoLdflags = append(cgoLdflags, "-L"+dir)
			if runtime.GOOS == "linux" {

				ldflags = append(ldflags, "-Wl,-rpath-link,"+dir)
				cgoLdflags = append(cgoLdflags, "-Wl,-rpath-link,"+dir)
				newDTags = true
			}
			if ectx.AbsRpath {
				ldflags = append(ldflags, "-Wl,-rpath,"+dir)
				cgoLdflags = append(cgoLdflags, "-Wl,-rpath,"+dir)
			} else {
				ldflags = append(ldflags, rpathFlag(d.Name, d.Version, lib))
				cgoLdflags = append(cgoLdflags, cgoRpathFlag(d.Name, d.Version, lib))
			}
		}
	}
	if newDTags {
		ldflags = append(ldflags, "-Wl,--enable-new-dtags")
		cgoLdflags = append(cgoLdflags, "-Wl,--enable-new-dtags")
	}
	if runtime.GOOS == "darwin" {
		const headerpad = "-Wl,-headerpad_max_install_names"
		ldflags = append(ldflags, headerpad)
		cgoLdflags = append(cgoLdflags, headerpad)
	}

	prependPathVar(vars, "PATH", pathDirs)
	appendVar(vars, "CPPFLAGS", cppflags, " ")
	appendVar(vars, "CFLAGS", cflags, " ")
	appendVar(vars, "LDFLAGS", ldflags, " ")
	appendVar(vars, "CGO_CFLAGS", cflags, " ")
	appendVar(vars, "CGO_LDFLAGS", cgoLdflags, " ")
	appendVar(vars, "PKG_CONFIG_PATH", pkgcfg, string(filepath.ListSeparator))

	vars["NEM_VERSION"] = ectx.Version
	vars["NEM_OS"] = ectx.Platform.OS
	vars["NEM_ARCH"] = ectx.Platform.Arch
	vars["NEM_PREFIX"] = ectx.Prefix
	if ectx.StagingDir != "" {
		vars["NEM_STAGING_DIR"] = ectx.StagingDir
	}
	if ectx.OutputDir != "" {
		vars["NEM_OUTPUT"] = ectx.OutputDir
	}

	return mapToEnv(vars)
}

func rpathFlag(dep, version, lib string) string {
	rel := rpathRel(dep, version, lib)
	if runtime.GOOS == "linux" {
		return "-Wl,-rpath,'$$ORIGIN" + rel + "'"
	}
	return "-Wl,-rpath,@loader_path" + rel
}

func cgoRpathFlag(dep, version, lib string) string {
	rel := rpathRel(dep, version, lib)
	if runtime.GOOS == "linux" {
		return "-Wl,-rpath,$ORIGIN" + rel
	}
	return "-Wl,-rpath,@loader_path" + rel
}

func rpathRel(dep, version, lib string) string {
	return "/../../../" + dep + "/" + version + "/" + lib
}

func depPrefixVar(name string) string {
	mangled := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - ('a' - 'A')
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, name)
	return "NEM_DEP_" + mangled + "_PREFIX"
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

func mapToEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func prependPathVar(m map[string]string, key string, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	prefix := strings.Join(dirs, string(filepath.ListSeparator))
	if cur := m[key]; cur != "" {
		m[key] = prefix + string(filepath.ListSeparator) + cur
	} else {
		m[key] = prefix
	}
}

func appendVar(m map[string]string, key string, parts []string, sep string) {
	if len(parts) == 0 {
		return
	}
	add := strings.Join(parts, sep)
	if cur := m[key]; cur != "" {
		m[key] = cur + sep + add
	} else {
		m[key] = add
	}
}

func ScrubEnv(env []string, drop []string, overrides ...string) []string {
	dropped := make(map[string]bool, len(drop))
	for _, k := range drop {
		dropped[k] = true
	}
	out := make([]string, 0, len(env)+len(overrides))
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && dropped[k] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, overrides...)
}
