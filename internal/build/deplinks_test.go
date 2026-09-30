package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func compileDepFixture(t *testing.T, out, depRoot string, bins ...string) {
	t.Helper()
	cc := requireCC(t)
	src := t.TempDir()
	writeTree(t, src, "demo.c", "int demo(void) { return 42; }\n")
	writeTree(t, src, "main.c", "int demo(void); int main(void) { return demo(); }\n")
	if err := os.MkdirAll(filepath.Join(depRoot, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := "-ldemo.1"
	if runtime.GOOS == "linux" {
		run(t, cc, "-shared", "-fPIC", "-Wl,-soname,libdemo.so.1", "-o", filepath.Join(depRoot, "lib", "libdemo.so.1"), filepath.Join(src, "demo.c"))
		link = "-l:libdemo.so.1"
	} else {
		run(t, cc, "-dynamiclib", "-install_name", "@rpath/libdemo.1.dylib", "-o", filepath.Join(depRoot, "lib", "libdemo.1.dylib"), filepath.Join(src, "demo.c"))
	}
	for _, bin := range bins {
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatal(err)
		}
		args := []string{"-o", bin, filepath.Join(src, "main.c"), "-L", filepath.Join(depRoot, "lib"), link,
			"-Wl,-rpath," + depsRpath(loaderAnchor()) + "demo/lib"}
		if runtime.GOOS == "darwin" {
			args = append(args, "-Wl,-headerpad_max_install_names")
		} else {
			args = append(args, "-Wl,--enable-new-dtags")
		}
		run(t, cc, args...)
	}
	if err := os.MkdirAll(filepath.Join(out, ".nem-link-dependencies"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(depRoot, filepath.Join(out, ".nem-link-dependencies", "demo")); err != nil {
		t.Fatal(err)
	}
}

func exits42(t *testing.T, bin string) bool {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	_ = cmd.Run()
	return cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 42
}

func TestNormalizePlantsDepLinksAtEveryDepth(t *testing.T) {
	out, depRoot := t.TempDir(), t.TempDir()
	bins := []string{
		filepath.Join(out, "root"),
		filepath.Join(out, "bin", "demo"),
		filepath.Join(out, "libexec", "demo", "demo"),
		filepath.Join(out, "lib", "a", "b", "c", "d", "e", "f", "deep"),
	}
	compileDepFixture(t, out, depRoot, bins...)
	if exits42(t, bins[1]) {
		t.Fatal("bin/demo must not resolve before normalization")
	}
	if !exits42(t, bins[0]) {
		t.Fatal("a root-level binary resolves through the real directory without any link")
	}

	if err := normalizeOutput(out); err != nil {
		t.Fatalf("normalizeOutput: %v", err)
	}
	for _, bin := range bins {
		if !exits42(t, bin) {
			t.Fatalf("%s does not load libdemo after normalization", bin)
		}
	}
	for dir, want := range map[string]string{
		filepath.Join(out, "bin"):                               filepath.Join("..", ".nem-link-dependencies"),
		filepath.Join(out, "libexec", "demo"):                   filepath.Join("..", "..", ".nem-link-dependencies"),
		filepath.Join(out, "lib", "a", "b", "c", "d", "e", "f"): filepath.Join("..", "..", "..", "..", "..", "..", "..", ".nem-link-dependencies"),
	} {
		got, err := os.Readlink(filepath.Join(dir, ".nem-link-dependencies"))
		if err != nil || got != want {
			t.Fatalf("%s/.nem-link-dependencies -> %q, %v; want %q", dir, got, err, want)
		}
	}
	if info, err := os.Lstat(filepath.Join(out, ".nem-link-dependencies")); err != nil || !info.IsDir() {
		t.Fatalf("root .nem-link-dependencies must stay the real directory: %v %v", info, err)
	}
}

func TestPlanDepLinksSkipsBinariesWithoutDepRpaths(t *testing.T) {
	cc := requireCC(t)
	out := t.TempDir()
	src := t.TempDir()
	writeTree(t, src, "main.c", "int main(void) { return 0; }\n")
	if err := os.MkdirAll(filepath.Join(out, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, cc, "-o", filepath.Join(out, "bin", "plain"), filepath.Join(src, "main.c"), "-Wl,-rpath,"+loaderAnchor()+"/../lib")
	links, err := planDepLinks(out)
	if err != nil || len(links) != 0 {
		t.Fatalf("links = %+v, %v; want none for an in-tree-only rpath", links, err)
	}
}

func TestApplyDepLinksRefusesToClobberPackageContent(t *testing.T) {
	out := t.TempDir()
	dir := filepath.Join(out, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTree(t, out, "bin/.nem-link-dependencies", "shipped\n")
	err := applyDepLinks([]depLink{{dir: dir, target: filepath.Join("..", ".nem-link-dependencies")}})
	if err == nil {
		t.Fatal("want an error when .nem-link-dependencies is a regular file")
	}
	if got := readTree(t, out, "bin/.nem-link-dependencies"); got != "shipped\n" {
		t.Fatalf("package content clobbered: %q", got)
	}
}
