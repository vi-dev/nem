package archive

import (
	"net/url"
	"path"
	"strings"
)

func SingleNameFromRef(ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if base == "." || base == "/" {
		return ""
	}
	for _, ext := range []string{".gz", ".bz2", ".xz", ".zst"} {
		if trimmed, ok := strings.CutSuffix(base, ext); ok {
			return trimmed
		}
	}
	return base
}
