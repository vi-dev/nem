package envx

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/usage"
)

type Var struct{ Name, Value, Source string }

type Result struct {
	Vars       []Var
	Path       []string
	LoaderVar  string
	LoaderPath []string
	Warnings   []string
}

func Compose(project, global *project.Manifest, projectLock, globalLock *project.Lockfile, h home.Home,
	metaLookup func(name, version string) (*install.Meta, bool),
	getenv func(string) (string, bool)) Result {

	var warnings []string

	resolvedProject, w := resolveEntries(orderedLockEntries(projectLock), metaLookup, h)
	warnings = append(warnings, w...)
	resolvedGlobal, w := resolveEntries(orderedLockEntries(globalLock), metaLookup, h)
	warnings = append(warnings, w...)
	stampUsage(h, resolvedProject, resolvedGlobal)

	path := buildPath(resolvedProject, resolvedGlobal)
	loaderPath := buildLoaderPath(resolvedProject, resolvedGlobal)

	vars := map[string]string{}
	sources := map[string]string{}
	applyPackageExports(resolvedGlobal, vars, sources, &warnings)
	applyPackageExports(resolvedProject, vars, sources, &warnings)

	lookup := savedOriginalLookup(managedKeys(vars, global.Env, project.Env), getenv)
	applyEnvSection(global.Env, vars, sources, &warnings, lookup)
	applyEnvSection(project.Env, vars, sources, &warnings, lookup)

	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	result := Result{Path: path, LoaderVar: LoaderPathVar(), LoaderPath: loaderPath, Warnings: warnings}
	for _, name := range names {
		result.Vars = append(result.Vars, Var{Name: name, Value: vars[name], Source: sources[name]})
	}
	return result
}

func ComposeScope(scope, other *project.Manifest, scopeLock, otherLock *project.Lockfile, h home.Home,
	metaLookup func(name, version string) (*install.Meta, bool),
	getenv func(string) (string, bool)) Result {

	var warnings []string

	resolvedScope, w := resolveEntries(orderedLockEntries(scopeLock), metaLookup, h)
	warnings = append(warnings, w...)
	resolvedOther, _ := resolveEntries(orderedLockEntries(otherLock), metaLookup, h)

	vars := map[string]string{}
	sources := map[string]string{}
	applyPackageExports(resolvedScope, vars, sources, &warnings)

	otherVars := map[string]string{}
	var otherWarnings []string
	applyPackageExports(resolvedOther, otherVars, map[string]string{}, &otherWarnings)

	managed := managedKeys(vars, scope.Env, other.Env)
	for name := range otherVars {
		managed[name] = true
	}
	lookup := savedOriginalLookup(managed, getenv)
	applyEnvSection(scope.Env, vars, sources, &warnings, lookup)

	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	result := Result{
		Path:       buildPath(resolvedScope, nil),
		LoaderVar:  LoaderPathVar(),
		LoaderPath: buildLoaderPath(resolvedScope, nil),
		Warnings:   warnings,
	}
	for _, name := range names {
		result.Vars = append(result.Vars, Var{Name: name, Value: vars[name], Source: sources[name]})
	}
	return result
}

func stampUsage(h home.Home, groups ...[]resolvedEntry) {
	var keys []string
	for _, g := range groups {
		for _, r := range g {
			keys = append(keys, usage.Key(r.name, r.version))
		}
	}
	usage.Stamp(h, time.Now(), keys)
}

type resolvedEntry struct {
	name, version string
	installDir    string
	meta          *install.Meta
	onPath        bool
	onLoaderPath  bool
}

func orderedLockEntries(lock *project.Lockfile) []project.LockEntry {
	current := spec.Current().String()
	var direct, indirect []project.LockEntry
	for _, e := range lock.Packages {
		if !slices.Contains(e.Platforms, current) {
			continue
		}
		if e.Direct {
			direct = append(direct, e)
		} else {
			indirect = append(indirect, e)
		}
	}
	sort.Slice(direct, func(i, j int) bool { return direct[i].Name < direct[j].Name })
	sort.Slice(indirect, func(i, j int) bool { return indirect[i].Name < indirect[j].Name })
	return append(direct, indirect...)
}

func resolveEntries(entries []project.LockEntry, metaLookup func(name, version string) (*install.Meta, bool), h home.Home) ([]resolvedEntry, []string) {
	var out []resolvedEntry
	var warnings []string
	for _, e := range entries {
		meta, ok := metaLookup(e.Name, e.Version)
		if !ok {
			if install.IsInstalled(h, e.Name, e.Version) {
				warnings = append(warnings, fmt.Sprintf("no install metadata for %s@%s", e.Name, e.Version))
			}
			continue
		}
		installDir, err := h.PackageDir(e.Name, e.Version)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("install dir for %s@%s: %v", e.Name, e.Version, err))
			continue
		}
		out = append(out, resolvedEntry{
			name: e.Name, version: e.Version, installDir: installDir, meta: meta,
			onPath: e.OnPath, onLoaderPath: e.OnLoaderPath,
		})
	}
	return out, warnings
}

func buildPath(projectEntries, globalEntries []resolvedEntry) []string {
	seen := map[string]bool{}
	var out []string
	for _, entries := range [][]resolvedEntry{projectEntries, globalEntries} {
		for _, r := range entries {
			if !r.onPath {
				continue
			}
			for _, bin := range r.meta.Bins {
				dir := filepath.Join(r.installDir, bin)
				if seen[dir] {
					continue
				}
				seen[dir] = true
				out = append(out, dir)
			}
		}
	}
	return out
}

func buildLoaderPath(projectEntries, globalEntries []resolvedEntry) []string {
	seen := map[string]bool{}
	var out []string
	for _, entries := range [][]resolvedEntry{projectEntries, globalEntries} {
		for _, r := range entries {
			if !r.onLoaderPath {
				continue
			}
			for _, lib := range r.meta.Libs {
				dir := filepath.Join(r.installDir, lib)
				if seen[dir] {
					continue
				}
				seen[dir] = true
				out = append(out, dir)
			}
		}
	}
	return out
}

func LoaderPathVar() string {
	if spec.Current().OS == "darwin" {
		return "DYLD_LIBRARY_PATH"
	}
	return "LD_LIBRARY_PATH"
}

func applyPackageExports(entries []resolvedEntry, vars, sources map[string]string, warnings *[]string) {
	current := spec.Current()
	for _, r := range entries {
		for _, export := range r.meta.Env {
			if !spec.PlatformsInclude(export.Platforms, current) {
				continue
			}
			if IsReserved(export.Name) {
				*warnings = append(*warnings, fmt.Sprintf("reserved env var %q from %s skipped", export.Name, r.name))
				continue
			}
			value, err := RenderEnvTemplate(export.Value, r.installDir, r.version)
			if err != nil {
				*warnings = append(*warnings, fmt.Sprintf("reinstall %s: %v", r.name, err))
				continue
			}
			vars[export.Name] = value
			sources[export.Name] = r.name
		}
	}
}

type envTemplateCtx struct {
	InstallDir string
	Version    string
}

var envTemplateFuncs = template.FuncMap{
	"trimPrefix": func(s, prefix string) string { return strings.TrimPrefix(s, prefix) },
	"trimSuffix": func(s, suffix string) string { return strings.TrimSuffix(s, suffix) },
	"replace":    func(s, old, new string) string { return strings.ReplaceAll(s, old, new) },
}

func RenderEnvTemplate(tmpl, installDir, version string) (string, error) {
	t, err := template.New("").Funcs(envTemplateFuncs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", tmpl, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, envTemplateCtx{InstallDir: installDir, Version: version}); err != nil {
		return "", fmt.Errorf("expand template %q: %w", tmpl, err)
	}
	return b.String(), nil
}

func managedKeys(exported map[string]string, envs ...[]project.EnvVar) map[string]bool {
	keys := make(map[string]bool, len(exported))
	for name := range exported {
		keys[name] = true
	}
	for _, list := range envs {
		for _, e := range list {
			keys[e.Name] = true
		}
	}
	return keys
}

func savedOriginalLookup(managed map[string]bool, getenv func(string) (string, bool)) func(string) (string, bool) {
	return func(name string) (string, bool) {
		if managed[name] {
			if _, ok := getenv("NEM_SAVED__" + name + "_SET"); ok {
				return getenv("NEM_SAVED__" + name)
			}
		}
		return getenv(name)
	}
}

func applyEnvSection(entries []project.EnvVar, vars, sources map[string]string, warnings *[]string, lookup func(string) (string, bool)) {
	for _, e := range entries {
		if IsReserved(e.Name) {
			*warnings = append(*warnings, fmt.Sprintf("reserved env var %q skipped", e.Name))
			continue
		}
		vars[e.Name] = Expand(e.Value, lookup)
		sources[e.Name] = "nem.toml"
	}
}
