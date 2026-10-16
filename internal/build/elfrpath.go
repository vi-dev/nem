package build

import (
	"bytes"
	delf "debug/elf"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vi-dev/nem/internal/fsx"
)

const doubledLoaderAnchor = "$$ORIGIN"

func fixElfRpathAnchors(outDir string) error {
	return filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		kind, err := binaryKind(path)
		if err != nil {
			return err
		}
		if kind != elf {
			return nil
		}
		return fixElfRpathAnchor(path)
	})
}

type dynstrEdit struct {
	offset   int64
	old, new string
}

func fixElfRpathAnchor(path string) error {
	f, err := delf.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	strtab := f.Section(".dynstr")
	if strtab == nil || strtab.Type != delf.SHT_STRTAB {
		return nil
	}
	table, err := strtab.Data()
	if err != nil {
		return fmt.Errorf("read .dynstr of %s: %w", path, err)
	}
	edits := map[uint64]dynstrEdit{}
	for _, tag := range []delf.DynTag{delf.DT_RPATH, delf.DT_RUNPATH} {
		vals, err := f.DynValue(tag)
		if err != nil {
			return nil
		}
		for _, v := range vals {
			old, ok := cstring(table, v)
			if !ok || !strings.Contains(old, doubledLoaderAnchor) {
				continue
			}
			edits[v] = dynstrEdit{
				offset: int64(strtab.Offset) + int64(v),
				old:    old,
				new:    strings.ReplaceAll(old, doubledLoaderAnchor, "$ORIGIN"),
			}
		}
	}
	if len(edits) == 0 {
		return nil
	}
	refs, err := dynstrReferences(f)
	if err != nil {
		return fmt.Errorf("scan dynamic strings of %s: %w", path, err)
	}
	for v, e := range edits {
		for _, r := range refs {
			if r > v && r <= v+uint64(len(e.old)) {
				return fmt.Errorf("rewrite rpath of %s: another dynamic string starts inside %q", path, e.old)
			}
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("rewrite rpath of %s: %w", path, err)
	}
	for _, e := range edits {
		buf := make([]byte, len(e.old)+1)
		copy(buf, e.new)
		copy(data[e.offset:], buf)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("rewrite rpath of %s: %w", path, err)
	}
	if err := fsx.WriteAtomic(path, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("rewrite rpath of %s: %w", path, err)
	}
	return nil
}

func cstring(table []byte, off uint64) (string, bool) {
	if off >= uint64(len(table)) {
		return "", false
	}
	rest := table[off:]
	if i := bytes.IndexByte(rest, 0); i >= 0 {
		rest = rest[:i]
	}
	return string(rest), true
}

func dynstrReferences(f *delf.File) ([]uint64, error) {
	var refs []uint64
	for _, tag := range []delf.DynTag{
		delf.DT_NEEDED, delf.DT_SONAME, delf.DT_RPATH, delf.DT_RUNPATH,
		delf.DT_AUXILIARY, delf.DT_FILTER, delf.DT_CONFIG, delf.DT_DEPAUDIT, delf.DT_AUDIT,
	} {
		vals, err := f.DynValue(tag)
		if err != nil {
			return nil, err
		}
		refs = append(refs, vals...)
	}
	bo := f.ByteOrder
	if s := f.Section(".dynsym"); s != nil {
		data, err := s.Data()
		if err != nil {
			return nil, err
		}
		size := int(s.Entsize)
		if size == 0 {
			size = 16
			if f.Class == delf.ELFCLASS64 {
				size = 24
			}
		}
		for i := 0; i+4 <= len(data); i += size {
			refs = append(refs, uint64(bo.Uint32(data[i:])))
		}
	}
	if s := f.Section(".gnu.version_r"); s != nil {
		data, err := s.Data()
		if err != nil {
			return nil, err
		}
		for off := 0; off+16 <= len(data); {
			cnt := int(bo.Uint16(data[off+2:]))
			refs = append(refs, uint64(bo.Uint32(data[off+4:])))
			for a, j := off+int(bo.Uint32(data[off+8:])), 0; j < cnt && a+16 <= len(data); j++ {
				refs = append(refs, uint64(bo.Uint32(data[a+8:])))
				next := int(bo.Uint32(data[a+12:]))
				if next == 0 {
					break
				}
				a += next
			}
			next := int(bo.Uint32(data[off+12:]))
			if next == 0 {
				break
			}
			off += next
		}
	}
	if s := f.Section(".gnu.version_d"); s != nil {
		data, err := s.Data()
		if err != nil {
			return nil, err
		}
		for off := 0; off+20 <= len(data); {
			cnt := int(bo.Uint16(data[off+6:]))
			for a, j := off+int(bo.Uint32(data[off+12:])), 0; j < cnt && a+8 <= len(data); j++ {
				refs = append(refs, uint64(bo.Uint32(data[a:])))
				next := int(bo.Uint32(data[a+4:]))
				if next == 0 {
					break
				}
				a += next
			}
			next := int(bo.Uint32(data[off+16:]))
			if next == 0 {
				break
			}
			off += next
		}
	}
	return refs, nil
}
