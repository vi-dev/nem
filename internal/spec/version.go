package spec

import "strings"

func CompareVersions(a, b string) int {
	ar, apre := splitVersion(a)
	br, bpre := splitVersion(b)
	if c := compareRuns(ar, br); c != 0 {
		return c
	}
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	}
	return compareRuns(apre, bpre)
}

func splitVersion(v string) (release, pre string) {
	if len(v) > 1 && v[0] == 'v' && versionDigit(v[1]) {
		v = v[1:]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func compareRuns(a, b string) int {
	as, bs := runs(a), runs(b)
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		if x == y {
			continue
		}
		if versionDigit(x[0]) && versionDigit(y[0]) {
			xt, yt := strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
			if len(xt) != len(yt) {
				return len(xt) - len(yt)
			}
			if c := strings.Compare(xt, yt); c != 0 {
				return c
			}
			continue
		}
		return strings.Compare(x, y)
	}
	return len(as) - len(bs)
}

func runs(v string) []string {
	var segs []string
	for i := 0; i < len(v); {
		j := i
		for j < len(v) && versionDigit(v[j]) == versionDigit(v[i]) {
			j++
		}
		segs = append(segs, v[i:j])
		i = j
	}
	return segs
}

func versionDigit(c byte) bool { return c >= '0' && c <= '9' }

func (p *Package) HasEqualVersion(v string) bool {
	for _, e := range p.Versions {
		if CompareVersions(v, e.Version) == 0 {
			return true
		}
	}
	return false
}
