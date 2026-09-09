package install

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vi-dev/nem/internal/spec"
)

const artifactToken = spec.ArtifactToken

func RunActions(pkg *spec.Package, stagingDir, artifactPath, version string, platform spec.Platform) error {
	root, err := os.OpenRoot(stagingDir)
	if err != nil {
		return fmt.Errorf("open staging dir: %w", err)
	}
	defer root.Close()

	singleName := singleFileName(pkg, version, platform)
	ran := 0
	for i, a := range pkg.Install {
		if !spec.PlatformsInclude(a.Platforms, platform) {
			continue
		}
		a, err := spec.ExpandActionPaths(a, version, platform)
		if err != nil {
			return fmt.Errorf("install[%d]: %w", i, err)
		}
		if err := runAction(a, root, artifactPath, singleName); err != nil {
			return fmt.Errorf("install[%d]: %w", i, err)
		}
		ran++
	}

	if ran == 0 {
		return fmt.Errorf("no install action applies to %s", platform)
	}
	return nil
}

func runAction(a spec.Action, root *os.Root, artifactPath, singleName string) error {
	switch {
	case a.Extract != nil:
		return extract(artifactPath, root, a.Extract.Strip, singleName)
	case a.Copy != nil:
		return runCopy(a.Copy, root, artifactPath)
	case a.Move != nil:
		return runMove(a.Move, root)
	case a.Mkdir != "":
		return runMkdir(a.Mkdir, root)
	default:
		return errors.New("empty action")
	}
}

func runCopy(a *spec.CopyAction, root *os.Root, artifactPath string) error {
	if err := checkContained(a.Dst); err != nil {
		return err
	}

	var in *os.File
	var err error
	if a.Src == artifactToken {
		in, err = os.Open(artifactPath)
	} else {
		if err := checkContained(a.Src); err != nil {
			return err
		}
		in, err = root.Open(a.Src)
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", a.Src, err)
	}
	defer in.Close()

	if err := mkdirParent(root, a.Dst); err != nil {
		return err
	}
	mode := os.FileMode(a.Mode)
	if mode == 0 {
		mode = 0o644
	}
	out, err := root.OpenFile(a.Dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("create %s: %w", a.Dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy to %s: %w", a.Dst, err)
	}
	return out.Close()
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

func runMove(a *spec.MoveAction, root *os.Root) error {
	if err := checkContained(a.Src); err != nil {
		return err
	}
	if err := checkContained(a.Dst); err != nil {
		return err
	}
	if err := root.Rename(a.Src, a.Dst); err != nil {
		return fmt.Errorf("rename %s to %s: %w", a.Src, a.Dst, err)
	}
	return nil
}

func runMkdir(dir string, root *os.Root) error {
	if err := checkContained(dir); err != nil {
		return err
	}
	if err := root.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return nil
}

func checkContained(rel string) error {
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("path %q escapes staging dir", rel)
	}
	return nil
}
