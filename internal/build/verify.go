package build

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Violation struct{ File, Ref, Reason string }

func VerifyConformance(outputDir string, forbidden []string) ([]Violation, error) {
	var out []Violation
	err := filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
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
		if kind == notBinary {
			return nil
		}
		refs, err := loadRefs(path, kind)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			for _, bad := range forbidden {
				if ref == bad || strings.HasPrefix(ref, bad+string(filepath.Separator)) {
					out = append(out, Violation{File: path, Ref: ref,
						Reason: "absolute reference into the build tree; link via $LDFLAGS so refs are @rpath/soname"})
					break
				}
			}
		}
		return nil
	})
	return out, err
}

type binKind int

const (
	notBinary binKind = iota
	machO
	elf
)

func binaryKind(path string) (binKind, error) {
	f, err := os.Open(path)
	if err != nil {
		return notBinary, err
	}
	defer f.Close()
	var magic [4]byte
	if _, err := f.Read(magic[:]); err != nil {
		return notBinary, nil
	}
	switch magic {
	case [4]byte{0x7f, 'E', 'L', 'F'}:
		return elf, nil
	case [4]byte{0xfe, 0xed, 0xfa, 0xce}, [4]byte{0xfe, 0xed, 0xfa, 0xcf},
		[4]byte{0xce, 0xfa, 0xed, 0xfe}, [4]byte{0xcf, 0xfa, 0xed, 0xfe},
		[4]byte{0xca, 0xfe, 0xba, 0xbe}:
		return machO, nil
	}
	return notBinary, nil
}

func loadRefs(path string, kind binKind) ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		if kind != machO {
			return nil, nil
		}
		return otoolRefs(path)
	case "linux":
		if kind != elf {
			return nil, nil
		}
		return readelfRefs(path)
	}
	return nil, nil
}

func otoolRefs(path string) ([]string, error) {
	cmd := exec.Command("otool", "-l", path)
	stdout, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("otool -l %s: %w", path, err)
	}
	var refs []string
	s := bufio.NewScanner(strings.NewReader(string(stdout)))
	for s.Scan() {
		if ref, ok := parseOtoolOperand(s.Text()); ok {
			refs = append(refs, ref)
		}
	}
	return refs, s.Err()
}

func parseOtoolOperand(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	var rest string
	switch {
	case strings.HasPrefix(trimmed, "path "):
		rest = trimmed[len("path "):]
	case strings.HasPrefix(trimmed, "name "):
		rest = trimmed[len("name "):]
	default:
		return "", false
	}
	if i := strings.LastIndex(rest, " (offset "); i != -1 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	return rest, true
}

func readelfRefs(path string) ([]string, error) {
	cmd := exec.Command("readelf", "-d", path)
	stdout, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("readelf -d %s: %w", path, err)
	}
	var refs []string
	s := bufio.NewScanner(strings.NewReader(string(stdout)))
	for s.Scan() {
		line := s.Text()
		for _, tag := range []string{"(NEEDED)", "(RPATH)", "(RUNPATH)"} {
			if strings.Contains(line, tag) {
				if i := strings.LastIndex(line, "["); i != -1 {
					if j := strings.Index(line[i:], "]"); j != -1 {

						refs = append(refs, strings.Split(line[i+1:i+j], ":")...)
					}
				}
			}
		}
	}
	return refs, s.Err()
}
