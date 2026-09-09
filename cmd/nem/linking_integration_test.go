package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func writeMetaFile(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nem-meta.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func platformString() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func buildLib(t *testing.T, cc, installDir string, ver int) {
	t.Helper()
	libDir := filepath.Join(installDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "foo.c")
	if err := os.WriteFile(src, []byte("int foo_version(void){return "+strconv.Itoa(ver)+";}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	switch runtime.GOOS {
	case "darwin":
		out := filepath.Join(libDir, "libfoo.dylib")
		run(t, cc, "-dynamiclib", "-install_name", "@rpath/libfoo.dylib", "-o", out, src)
	case "linux":
		out := filepath.Join(libDir, "libfoo.so")
		run(t, cc, "-shared", "-fPIC", "-Wl,-soname,libfoo.so", "-o", out, src)
	}
}

func buildConsumer(t *testing.T, cc, installDir, frozenLibDir string) {
	t.Helper()
	binDir := filepath.Join(installDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "use.c")
	useSrc := "#include <stdio.h>\n" +
		"int foo_version(void);\n" +
		"int main(void){printf(\"%d\\n\", foo_version());return 0;}\n"
	if err := os.WriteFile(src, []byte(useSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(binDir, "usefoo")

	rel, err := filepath.Rel(binDir, frozenLibDir)
	if err != nil {
		t.Fatal(err)
	}
	switch runtime.GOOS {
	case "darwin":
		run(t, cc, "-o", out, src, "-L", frozenLibDir, "-lfoo",
			"-Wl,-rpath,@loader_path/"+rel)
	case "linux":
		run(t, cc, "-o", out, src, "-L", frozenLibDir, "-lfoo",
			"-Wl,-rpath,$ORIGIN/"+rel, "-Wl,--enable-new-dtags")
	}
}

func TestLinkingFloatsUnderExecAndFallsBackToRpath(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("loader semantics differ off darwin/linux")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}

	nemHomeDir := t.TempDir()
	pkgDir := func(name, ver string) string {
		return filepath.Join(nemHomeDir, "packages", name, ver)
	}

	buildLib(t, cc, pkgDir("foo", "v1"), 1)
	buildLib(t, cc, pkgDir("foo", "v2"), 2)
	writeMetaFile(t, pkgDir("foo", "v2"), "package: foo\nversion: v2\ncatalog: c\nbins: [bin]\nlibs: [lib]\n")

	buildConsumer(t, cc, pkgDir("usefoo", "v1"), filepath.Join(pkgDir("foo", "v1"), "lib"))
	writeMetaFile(t, pkgDir("usefoo", "v1"), "package: usefoo\nversion: v1\ncatalog: c\nbins: [bin]\n")

	lock := "# machine-written by nem — do not edit\nversion = 2\n\n" +
		"[[package]]\nname = \"usefoo\"\nversion = \"v1\"\ncatalog = \"c\"\ndirect = true\non_path = true\nplatforms = [\"" + platformString() + "\"]\n\n" +
		"[[package]]\nname = \"foo\"\nversion = \"v2\"\ncatalog = \"c\"\non_path = false\non_loader_path = true\nplatforms = [\"" + platformString() + "\"]\n"
	if err := os.WriteFile(filepath.Join(nemHomeDir, "nem.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nemHomeDir, "nem.toml"), []byte("[tools]\nusefoo = \"v1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	chdir(t, t.TempDir())

	out, errb, err := runNem(t, nemHomeDir, "exec", "--", "usefoo")
	if err != nil {
		t.Fatalf("nem exec: %v\nstderr: %s", err, errb)
	}
	if strings.TrimSpace(out) != "2" {
		t.Fatalf("under nem exec want floated lib (2), got %q", out)
	}

	direct := exec.Command(filepath.Join(pkgDir("usefoo", "v1"), "bin", "usefoo"))
	direct.Env = []string{"PATH=/usr/bin:/bin"}
	db, err := direct.Output()
	if err != nil {
		t.Fatalf("direct run: %v", err)
	}
	if strings.TrimSpace(string(db)) != "1" {
		t.Fatalf("direct run want rpath-frozen lib (1), got %q", string(db))
	}
}
