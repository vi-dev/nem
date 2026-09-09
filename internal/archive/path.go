package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func checkSymlinkContainment(relEntryPath, target string) error {
	if filepath.IsAbs(target) {
		return fmt.Errorf("symlink %s: target %q is absolute", relEntryPath, target)
	}
	resolved := filepath.Join(filepath.Dir(relEntryPath), target)
	if !filepath.IsLocal(resolved) {
		return fmt.Errorf("symlink %s: target %q escapes extraction root", relEntryPath, target)
	}
	return nil
}

func mkdirParent(root *os.Root, relPath string) error {
	dir := filepath.Dir(relPath)
	if dir == "." {
		return nil
	}
	if err := root.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	return nil
}

func writeFile(root *os.Root, relPath string, r io.Reader, mode os.FileMode) error {
	f, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create file %s: %w", relPath, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return fmt.Errorf("write file %s: %w", relPath, err)
	}
	return f.Close()
}
