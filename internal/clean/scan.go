package clean

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vi-dev/nem/internal/home"
)

const buildStagingGlob = "*" + home.BuildStagingInfix + "*"

const testInstallGlob = "*" + home.TestInstallInfix + "*"

func Scan(h home.Home, includeVersions bool) (Store, error) {
	var s Store

	tmp, err := os.ReadDir(h.Tmp())
	if err != nil && !os.IsNotExist(err) {
		return s, err
	}
	for _, e := range tmp {
		path := filepath.Join(h.Tmp(), e.Name())
		switch {
		case e.IsDir():

			if ok, _ := filepath.Match(buildStagingGlob, e.Name()); !ok {
				continue
			}
			newest, size, err := treeStat(path)
			if err != nil {
				continue
			}
			s.Staging = append(s.Staging, Item{Path: path, Newest: newest, Size: size})
		case e.Type().IsRegular() && strings.HasSuffix(e.Name(), home.TmpSuffix):

			info, err := e.Info()
			if err != nil {
				continue
			}
			s.Downloads = append(s.Downloads, Item{Path: path, Newest: info.ModTime(), Size: info.Size()})
		}
	}

	pkgRoot := h.Packages()
	names, err := os.ReadDir(pkgRoot)
	if err != nil && !os.IsNotExist(err) {
		return s, err
	}
	for _, n := range names {
		if !n.IsDir() {
			continue
		}

		if ok, _ := filepath.Match(testInstallGlob, n.Name()); ok {
			path := filepath.Join(pkgRoot, n.Name())
			newest, size, err := treeStat(path)
			if err != nil {
				continue
			}
			s.TestInstalls = append(s.TestInstalls, Item{Path: path, Newest: newest, Size: size})
			continue
		}
		versions, err := os.ReadDir(filepath.Join(pkgRoot, n.Name()))
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			path, err := h.PackageDir(n.Name(), v.Name())
			if err != nil {
				continue
			}
			if strings.HasSuffix(v.Name(), home.TmpSuffix) {
				newest, size, err := treeStat(path)
				if err != nil {
					continue
				}
				s.Partials = append(s.Partials, Item{Path: path, Newest: newest, Size: size})
				continue
			}
			if !includeVersions {
				continue
			}
			_, size, err := treeStat(path)
			if err != nil {
				continue
			}
			s.Versions = append(s.Versions, Version{
				Name: n.Name(), Version: v.Name(), Path: path, Size: size,
			})
		}
	}

	return s, nil
}

func treeStat(root string) (time.Time, int64, error) {
	var newest time.Time
	var size int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if d.Type().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return newest, size, err
}
