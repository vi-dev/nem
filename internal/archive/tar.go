package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractTar(tr *tar.Reader, root *os.Root, opts Options) (Result, error) {
	var prefix prefixTracker
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return Result{CommonPrefix: prefix.value()}, nil
		}
		if err != nil {
			return Result{}, fmt.Errorf("read tar entry: %w", err)
		}

		if hdr.Typeflag == tar.TypeXHeader || hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		parts, relPath, ok := splitEntryPath(hdr.Name, opts.Strip)
		if len(parts) > 0 {
			prefix.add(parts[0])
		}
		if !ok {
			continue
		}
		if !filepath.IsLocal(relPath) {
			return Result{}, fmt.Errorf("entry %q escapes extraction root", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(relPath, 0o755); err != nil {
				return Result{}, fmt.Errorf("create dir %s: %w", relPath, err)
			}
		case tar.TypeReg:
			if err := mkdirParent(root, relPath); err != nil {
				return Result{}, err
			}
			if err := writeFile(root, relPath, tr, os.FileMode(hdr.Mode)&0o777); err != nil {
				return Result{}, err
			}
		case tar.TypeSymlink:
			if err := checkSymlinkContainment(relPath, hdr.Linkname); err != nil {
				return Result{}, err
			}
			if err := mkdirParent(root, relPath); err != nil {
				return Result{}, err
			}
			if err := root.Symlink(hdr.Linkname, relPath); err != nil {
				return Result{}, fmt.Errorf("create symlink %s: %w", relPath, err)
			}
		case tar.TypeLink:
			_, linkRel, ok := splitEntryPath(hdr.Linkname, opts.Strip)
			if !ok || !filepath.IsLocal(linkRel) {
				return Result{}, fmt.Errorf("hardlink %q target %q escapes extraction root", hdr.Name, hdr.Linkname)
			}
			if err := mkdirParent(root, relPath); err != nil {
				return Result{}, err
			}
			if err := root.Link(linkRel, relPath); err != nil {
				return Result{}, fmt.Errorf("create hardlink %s: %w", relPath, err)
			}
		default:
			return Result{}, fmt.Errorf("entry %q: unsupported tar type %v", hdr.Name, hdr.Typeflag)
		}
	}
}

func splitEntryPath(name string, strip int) (parts []string, relPath string, ok bool) {
	if strip < 0 {
		strip = 0
	}
	for p := range strings.SplitSeq(name, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if strip >= len(parts) {
		return parts, "", false
	}
	return parts, filepath.FromSlash(strings.Join(parts[strip:], "/")), true
}

type prefixTracker struct {
	first    string
	saw      bool
	multiple bool
}

func (p *prefixTracker) add(seg string) {
	switch {
	case !p.saw:
		p.first, p.saw = seg, true
	case seg != p.first:
		p.multiple = true
	}
}

func (p *prefixTracker) value() string {
	if p.saw && !p.multiple {
		return p.first
	}
	return ""
}
