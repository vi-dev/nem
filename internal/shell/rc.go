package shell

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vi-dev/nem/internal/fsx"
)

const rcFileMode = 0o644

func InstallBlock(rcPath string, d Dialect) error {
	existing, mode, err := readRCFile(rcPath)
	if err != nil {
		return err
	}

	prefix, suffix, _ := splitAroundBlock(existing)
	updated := prefix + blockFor(prefix, HookBlock(d)) + suffix

	if err := fsx.WriteAtomic(rcPath, []byte(updated), mode); err != nil {
		return fmt.Errorf("write %s: %w", rcPath, err)
	}
	return nil
}

func RemoveBlock(rcPath string) error {
	existing, mode, err := readRCFile(rcPath)
	if err != nil {
		return err
	}

	prefix, suffix, ok := splitAroundBlock(existing)
	if !ok {
		return nil
	}

	if err := fsx.WriteAtomic(rcPath, []byte(prefix+suffix), mode); err != nil {
		return fmt.Errorf("write %s: %w", rcPath, err)
	}
	return nil
}

func readRCFile(rcPath string) (content string, mode os.FileMode, err error) {
	f, err := os.Open(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", rcFileMode, nil
		}
		return "", 0, fmt.Errorf("open %s: %w", rcPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat %s: %w", rcPath, err)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return "", 0, fmt.Errorf("read %s: %w", rcPath, err)
	}

	return string(data), info.Mode().Perm(), nil
}

func blockFor(prefix, body string) string {
	sep := ""
	if prefix != "" {
		sep = "\n"
	}
	return sep + BeginMarker + "\n" + body + "\n" + EndMarker + "\n"
}

func splitAroundBlock(s string) (prefix, suffix string, ok bool) {
	start, end, ok := findBlock(s)
	if !ok {
		return s, "", false
	}
	return s[:start], s[end:], true
}

func findBlock(s string) (start, end int, ok bool) {
	beginIdx := strings.Index(s, BeginMarker)
	if beginIdx < 0 {
		return 0, 0, false
	}
	afterBegin := beginIdx + len(BeginMarker)
	rel := strings.Index(s[afterBegin:], EndMarker)
	if rel < 0 {
		return 0, 0, false
	}
	endIdx := afterBegin + rel + len(EndMarker)

	start = beginIdx
	if beginIdx > 0 && s[beginIdx-1] == '\n' {
		start--
	}

	end = endIdx
	if end < len(s) && s[end] == '\n' {
		end++
	}

	return start, end, true
}
