package build

import (
	"bytes"
	"debug/macho"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func normalizeOutput(outDir string) error {
	if err := dropLibtoolArchives(outDir); err != nil {
		return err
	}
	if err := relocatePkgconfig(outDir); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return nil
	}
	fixes, err := planMachoFixes(outDir)
	if err != nil {
		return err
	}
	return applyMachoFixes(fixes)
}

func dropLibtoolArchives(outDir string) error {
	return filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".la") {
			return os.Remove(path)
		}
		return nil
	})
}

func relocatePkgconfig(outDir string) error {
	return filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".pc") ||
			filepath.Base(filepath.Dir(path)) != "pkgconfig" {
			return nil
		}
		libSeg := filepath.Base(filepath.Dir(filepath.Dir(path)))
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			lines[i] = relocatePCLine(line, libSeg)
		}
		out := strings.Join(lines, "\n")
		if out == string(data) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(out), info.Mode().Perm())
	})
}

func relocatePCLine(line, libSeg string) string {
	key, val, ok := strings.Cut(line, "=")
	if !ok || !strings.HasPrefix(val, "/") {
		return line
	}
	switch key {
	case "prefix":
		return "prefix=${pcfiledir}/../.."
	case "exec_prefix":
		return "exec_prefix=${prefix}"
	case "libdir":
		return "libdir=${prefix}/" + libSeg
	case "includedir":
		return "includedir=${prefix}/include"
	}
	return line
}

type machoFix struct {
	path    string
	id      string
	changes [][2]string
	rpaths  []string
}

func planMachoFixes(outDir string) ([]machoFix, error) {
	type machoInfo struct {
		path   string
		dylib  bool
		id     string
		refs   []string
		rpaths []string
	}
	var files []machoInfo
	shipped := map[string]string{}
	err := filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 && strings.Contains(d.Name(), ".dylib") {
			shipped[d.Name()] = filepath.Dir(path)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		f, err := macho.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		info := machoInfo{path: path, dylib: f.Type == macho.TypeDylib}
		if info.dylib {
			if info.id, err = dylibID(f); err != nil {
				return fmt.Errorf("read dylib id of %s: %w", path, err)
			}
			shipped[filepath.Base(path)] = filepath.Dir(path)
		}
		if info.refs, err = f.ImportedLibraries(); err != nil {
			return fmt.Errorf("read imports of %s: %w", path, err)
		}
		for _, l := range f.Loads {
			if rp, ok := l.(*macho.Rpath); ok {
				info.rpaths = append(info.rpaths, rp.Path)
			}
		}
		files = append(files, info)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var fixes []machoFix
	for _, mf := range files {
		fix := machoFix{path: mf.path}
		if mf.dylib && strings.HasPrefix(mf.id, "/") && !systemLib(mf.id) {
			fix.id = "@rpath/" + filepath.Base(mf.id)
		}
		var inTree []string
		for _, ref := range mf.refs {
			base := filepath.Base(ref)
			switch {
			case strings.HasPrefix(ref, "@rpath/"):
				inTree = append(inTree, base)
			case strings.HasPrefix(ref, "/") && !systemLib(ref) && shipped[base] != "":
				fix.changes = append(fix.changes, [2]string{ref, "@rpath/" + base})
				inTree = append(inTree, base)
			}
		}
		have := map[string]bool{}
		for _, rp := range mf.rpaths {
			have[rp] = true
		}
		for _, base := range inTree {
			dir, ok := shipped[base]
			if !ok {
				continue
			}
			rel, err := filepath.Rel(filepath.Dir(mf.path), dir)
			if err != nil {
				return nil, fmt.Errorf("relate %s to %s: %w", mf.path, dir, err)
			}
			entry := "@loader_path"
			if rel != "." {
				entry += "/" + rel
			}
			if !have[entry] {
				have[entry] = true
				fix.rpaths = append(fix.rpaths, entry)
			}
		}
		if fix.id != "" || len(fix.changes) > 0 || len(fix.rpaths) > 0 {
			fixes = append(fixes, fix)
		}
	}
	sort.Slice(fixes, func(i, j int) bool { return fixes[i].path < fixes[j].path })
	return fixes, nil
}

func systemLib(path string) bool {
	return strings.HasPrefix(path, "/usr/lib/") || strings.HasPrefix(path, "/System/")
}

func dylibID(f *macho.File) (string, error) {
	const lcIDDylib = 0xd
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) < 12 || f.ByteOrder.Uint32(raw[0:4]) != lcIDDylib {
			continue
		}
		off := f.ByteOrder.Uint32(raw[8:12])
		if int(off) >= len(raw) {
			return "", fmt.Errorf("name offset %d out of range", off)
		}
		name := raw[off:]
		if i := bytes.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		return string(name), nil
	}
	return "", nil
}

func applyMachoFixes(fixes []machoFix) error {
	for _, fix := range fixes {
		var args []string
		if fix.id != "" {
			args = append(args, "-id", fix.id)
		}
		for _, c := range fix.changes {
			args = append(args, "-change", c[0], c[1])
		}
		for _, rp := range fix.rpaths {
			args = append(args, "-add_rpath", rp)
		}
		args = append(args, fix.path)
		if out, err := exec.Command("install_name_tool", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("install_name_tool %s: %v\n%s", fix.path, err, out)
		}
		if out, err := exec.Command("codesign", "-f", "-s", "-", fix.path).CombinedOutput(); err != nil {
			return fmt.Errorf("codesign %s: %v\n%s", fix.path, err, out)
		}
	}
	return nil
}
