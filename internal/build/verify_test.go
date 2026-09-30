package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func requireCC(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("loader semantics differ off darwin/linux")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}
	return cc
}

func TestVerifyConformance(t *testing.T) {
	cc := requireCC(t)
	dir := t.TempDir()
	forbidden := filepath.Join(dir, "forbidden")
	if err := os.MkdirAll(forbidden, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "m.c")
	os.WriteFile(src, []byte("int main(void){return 0;}\n"), 0o644)

	good := filepath.Join(dir, "good")
	run(t, cc, "-o", good, src)

	bad := filepath.Join(dir, "bad")
	run(t, cc, "-o", bad, src, "-Wl,-rpath,"+forbidden+"/lib")

	sibling := filepath.Join(dir, "forbidden-sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	okSibling := filepath.Join(dir, "ok-sibling")
	run(t, cc, "-o", okSibling, src, "-Wl,-rpath,"+sibling+"/lib")

	vs, err := VerifyConformance(dir, []string{forbidden})
	if err != nil {
		t.Fatalf("VerifyConformance: %v", err)
	}
	if len(vs) != 1 || filepath.Base(vs[0].File) != "bad" {
		t.Fatalf("want exactly one violation on 'bad', got %+v", vs)
	}
}

func TestVerifyConformanceForbiddenPathWithSpace(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("otool is darwin-only")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}
	dir := t.TempDir()
	forbidden := filepath.Join(dir, "for bidden")
	if err := os.MkdirAll(forbidden, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "m.c")
	os.WriteFile(src, []byte("int main(void){return 0;}\n"), 0o644)

	bad := filepath.Join(dir, "bad")
	run(t, cc, "-o", bad, src, "-Wl,-rpath,"+forbidden+"/lib")

	vs, err := VerifyConformance(dir, []string{forbidden})
	if err != nil {
		t.Fatalf("VerifyConformance: %v", err)
	}
	if len(vs) != 1 || filepath.Base(vs[0].File) != "bad" {
		t.Fatalf("want exactly one violation on 'bad', got %+v", vs)
	}
}

func TestDepsRpathViolation(t *testing.T) {
	a := loaderAnchor()
	mustBe := "rpath must be " + a + "/.nem-link-dependencies/<dep>/<lib>; link via $LDFLAGS"
	escape := "rpath escapes the package root; link via $LDFLAGS so rpaths go through .nem-link-dependencies"
	cases := []struct {
		ref   string
		depth int
		want  string
	}{
		{a + "/.nem-link-dependencies/zlib/lib", 1, ""},
		{a + "/.nem-link-dependencies/zlib/lib", 7, ""},
		{a + "/../.nem-link-dependencies/zlib/lib", 1, mustBe},
		{a + "/../../../.nem-link-dependencies/zlib/lib", 3, mustBe},
		{a + "/../../../zlib/1.3.2/lib", 1, escape},
		{a + "/../../zlib/1.3.2/lib", 1, escape},
		{a + "/../../../lib/vendor/plugins", 3, ""},
		{a + "/../lib", 1, ""},
		{"/usr/lib/libz.1.dylib", 1, ""},
		{"libz.so.1", 1, ""},
	}
	for _, tc := range cases {
		if got := depsRpathViolation(tc.ref, tc.depth); got != tc.want {
			t.Errorf("depsRpathViolation(%q, %d) = %q, want %q", tc.ref, tc.depth, got, tc.want)
		}
	}
}

func TestVerifyConformanceChecksDepLinksAndEscapes(t *testing.T) {
	cc := requireCC(t)
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "m.c")
	os.WriteFile(src, []byte("int main(void){return 0;}\n"), 0o644)
	a := loaderAnchor()
	dep := depsRpath(a) + "zlib/lib"
	mk := func(rel string, flags ...string) string {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		run(t, cc, append([]string{"-o", path, src}, flags...)...)
		return path
	}
	plant := func(rel string) {
		t.Helper()
		d := filepath.Join(dir, rel)
		up, _ := filepath.Rel(d, dir)
		if err := applyDepLinks([]depLink{{dir: d, target: filepath.Join(up, ".nem-link-dependencies")}}); err != nil {
			t.Fatal(err)
		}
	}
	mk("root", "-Wl,-rpath,"+dep)
	mk("bin/good", "-Wl,-rpath,"+dep)
	plant("bin")
	mk("a/b/c/d/e/f/g/h/i/deep", "-Wl,-rpath,"+dep)
	plant("a/b/c/d/e/f/g/h/i")
	mk("x/b/c/d/e/f/g/h/i/deepFree")
	unlinked := mk("sbin/unlinked", "-Wl,-rpath,"+dep)
	climb := mk("bin/climb", "-Wl,-rpath,"+a+"/../.nem-link-dependencies/zlib/lib")
	escape := mk("bin/escape", "-Wl,-rpath,"+a+"/../../../zlib/1.3.2/lib")

	vs, err := VerifyConformance(dir, nil)
	if err != nil {
		t.Fatalf("VerifyConformance: %v", err)
	}
	got := map[string][]string{}
	for _, v := range vs {
		got[v.File] = append(got[v.File], v.Reason)
	}
	if len(got) != 3 {
		t.Fatalf("violations = %+v, want exactly unlinked, climb, escape", vs)
	}
	hasReason := func(file, prefix string) bool {
		for _, r := range got[file] {
			if strings.HasPrefix(r, prefix) {
				return true
			}
		}
		return false
	}
	if !hasReason(unlinked, "binary directory has no .nem-link-dependencies link") {
		t.Errorf("unlinked: %v", got[unlinked])
	}
	if !hasReason(climb, "rpath must be") {
		t.Errorf("climb: %v", got[climb])
	}
	if !hasReason(escape, "rpath escapes the package root") {
		t.Errorf("escape: %v", got[escape])
	}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
