package build

import (
	"bytes"
	delf "debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTree(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNormalizeOutputFiles(t *testing.T) {
	out := t.TempDir()
	writeTree(t, out, "lib/libfoo.la", "libtool junk")
	writeTree(t, out, "lib/pkgconfig/foo.pc",
		"prefix=/\nexec_prefix=/\nlibdir=/lib\nincludedir=${prefix}/include\n\nName: foo\nLibs: -L${libdir} -lfoo\n")
	writeTree(t, out, "lib64/pkgconfig/bar.pc", "prefix=/x\nlibdir=/x/lib64\n")
	rel := "prefix=${pcfiledir}/../..\nlibdir=${prefix}/lib\n"
	writeTree(t, out, "share/pkgconfig/rel.pc", rel)

	if err := normalizeOutput(out); err != nil {
		t.Fatalf("normalizeOutput: %v", err)
	}

	if _, err := os.Stat(filepath.Join(out, "lib/libfoo.la")); !os.IsNotExist(err) {
		t.Fatalf("libfoo.la must be removed: %v", err)
	}
	want := "prefix=${pcfiledir}/../..\nexec_prefix=${prefix}\nlibdir=${prefix}/lib\nincludedir=${prefix}/include\n\nName: foo\nLibs: -L${libdir} -lfoo\n"
	if got := readTree(t, out, "lib/pkgconfig/foo.pc"); got != want {
		t.Fatalf("foo.pc = %q\nwant %q", got, want)
	}
	if got := readTree(t, out, "lib64/pkgconfig/bar.pc"); got != "prefix=${pcfiledir}/../..\nlibdir=${prefix}/lib64\n" {
		t.Fatalf("bar.pc = %q", got)
	}
	if got := readTree(t, out, "share/pkgconfig/rel.pc"); got != rel {
		t.Fatalf("already-relative rel.pc changed: %q", got)
	}

	before := readTree(t, out, "lib/pkgconfig/foo.pc")
	if err := normalizeOutput(out); err != nil {
		t.Fatalf("second normalizeOutput: %v", err)
	}
	if got := readTree(t, out, "lib/pkgconfig/foo.pc"); got != before {
		t.Fatalf("not idempotent: %q", got)
	}
}

func TestNormalizeMacho(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O normalization is darwin-only")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not available")
	}

	out := t.TempDir()
	src := t.TempDir()
	writeTree(t, src, "demo.c", "int demo(void) { return 42; }\n")
	writeTree(t, src, "main.c", "int demo(void); int main(void) { return demo(); }\n")
	if err := os.MkdirAll(filepath.Join(out, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(out, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(out, "lib", "libdemo.1.dylib")
	run := func(name string, args ...string) {
		t.Helper()
		if b, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, b)
		}
	}
	run("clang", "-dynamiclib", "-install_name", "/lib/libdemo.1.dylib", "-o", lib, filepath.Join(src, "demo.c"))
	run("clang", "-o", filepath.Join(out, "bin", "demo"), filepath.Join(src, "main.c"), "-L", filepath.Join(out, "lib"), "-ldemo.1")

	fixes, err := planMachoFixes(out)
	if err != nil {
		t.Fatalf("planMachoFixes: %v", err)
	}
	if len(fixes) != 2 {
		t.Fatalf("fixes = %+v, want id fix and reference fix", fixes)
	}

	if err := normalizeOutput(out); err != nil {
		t.Fatalf("normalizeOutput: %v", err)
	}
	fixes, err = planMachoFixes(out)
	if err != nil {
		t.Fatalf("re-plan: %v", err)
	}
	if len(fixes) != 0 {
		t.Fatalf("normalization not idempotent, still planned: %+v", fixes)
	}

	cmd := exec.Command(filepath.Join(out, "bin", "demo"))
	if b, err := cmd.CombinedOutput(); cmd.ProcessState.ExitCode() != 42 {
		t.Fatalf("normalized binary does not run: %v\n%s", err, b)
	}
}

func TestNormalizeMachoSelfRpath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O normalization is darwin-only")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not available")
	}

	out := t.TempDir()
	src := t.TempDir()
	writeTree(t, src, "demo.c", "int demo(void) { return 42; }\n")
	writeTree(t, src, "user.c", "int demo(void); int user(void) { return demo(); }\n")
	writeTree(t, src, "main.c", "int user(void); int main(void) { return user() - 42; }\n")
	if err := os.MkdirAll(filepath.Join(out, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(out, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(name string, args ...string) {
		t.Helper()
		if b, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, b)
		}
	}
	libDir := filepath.Join(out, "lib")
	run("clang", "-dynamiclib", "-install_name", "@rpath/libdemo.1.dylib",
		"-o", filepath.Join(libDir, "libdemo.1.dylib"), filepath.Join(src, "demo.c"))
	run("clang", "-dynamiclib", "-install_name", "@rpath/libuser.1.dylib",
		"-o", filepath.Join(libDir, "libuser.1.dylib"), filepath.Join(src, "user.c"),
		"-L", libDir, "-ldemo.1")
	run("clang", "-o", filepath.Join(out, "bin", "demo"), filepath.Join(src, "main.c"),
		"-L", libDir, "-luser.1")

	if err := normalizeOutput(out); err != nil {
		t.Fatalf("normalizeOutput: %v", err)
	}

	if b, err := exec.Command(filepath.Join(out, "bin", "demo")).CombinedOutput(); err != nil {
		t.Fatalf("normalized binary does not run: %v\n%s", err, b)
	}

	fixes, err := planMachoFixes(out)
	if err != nil {
		t.Fatalf("re-plan: %v", err)
	}
	if len(fixes) != 0 {
		t.Fatalf("normalization not idempotent, still planned: %+v", fixes)
	}
}

func TestNormalizeElfRpathAnchor(t *testing.T) {
	out := t.TempDir()
	lib, doubled := buildDoubledAnchorLib(t, out)

	if err := normalizeOutput(out); err != nil {
		t.Fatalf("normalizeOutput: %v", err)
	}

	want := strings.ReplaceAll(doubled, "$$ORIGIN", "$ORIGIN")
	if got := elfDynStrings(t, lib, delf.DT_RUNPATH); got != want {
		t.Fatalf("RUNPATH = %q, want %q", got, want)
	}
	if got := elfDynStrings(t, lib, delf.DT_SONAME); got != "libdemo.so.1" {
		t.Fatalf("SONAME changed to %q", got)
	}
	target, err := os.Readlink(filepath.Join(out, "lib", ".nem-link-dependencies"))
	if err != nil {
		t.Fatalf("dep link not planted after the rewrite: %v", err)
	}
	if target != filepath.Join("..", ".nem-link-dependencies") {
		t.Fatalf("dep link target = %q", target)
	}

	if err := normalizeOutput(out); err != nil {
		t.Fatalf("second normalizeOutput: %v", err)
	}
	if got := elfDynStrings(t, lib, delf.DT_RUNPATH); got != want {
		t.Fatalf("not idempotent: RUNPATH = %q", got)
	}
}

func TestNormalizeElfRpathAnchorKeepsReadOnlyFiles(t *testing.T) {
	out := t.TempDir()
	lib, doubled := buildDoubledAnchorLib(t, out)
	if err := os.Chmod(lib, 0o555); err != nil {
		t.Fatal(err)
	}

	if err := normalizeOutput(out); err != nil {
		t.Fatalf("normalizeOutput: %v", err)
	}

	want := strings.ReplaceAll(doubled, "$$ORIGIN", "$ORIGIN")
	if got := elfDynStrings(t, lib, delf.DT_RUNPATH); got != want {
		t.Fatalf("RUNPATH = %q, want %q", got, want)
	}
	info, err := os.Stat(lib)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Fatalf("mode = %o after the rewrite, want 555", info.Mode().Perm())
	}
}

func TestElfRpathRewriteRefusesAliasedStrings(t *testing.T) {
	out := t.TempDir()
	lib, _ := buildDoubledAnchorLib(t, out)
	pointSonameInsideRunpath(t, lib)
	before, err := os.ReadFile(lib)
	if err != nil {
		t.Fatal(err)
	}

	err = fixElfRpathAnchor(lib)
	if err == nil || !strings.Contains(err.Error(), "another dynamic string starts inside") {
		t.Fatalf("fixElfRpathAnchor error = %v, want a refusal", err)
	}
	after, err := os.ReadFile(lib)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the refused file was modified")
	}
}

func buildDoubledAnchorLib(t *testing.T, out string) (string, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("ELF rpath normalization is linux-only")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	src := t.TempDir()
	writeTree(t, src, "demo.c", "int demo(void) { return 42; }\n")
	if err := os.MkdirAll(filepath.Join(out, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(out, "lib", "libdemo.so.1")
	doubled := "$$ORIGIN/.nem-link-dependencies/zlib/lib:$$ORIGIN/.nem-link-dependencies/pcre2/lib"
	cmd := exec.Command("cc", "-shared", "-fPIC", "-o", lib, "-Wl,-soname,libdemo.so.1",
		"-Wl,--enable-new-dtags", "-Wl,-rpath,"+doubled, filepath.Join(src, "demo.c"))
	if outb, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, outb)
	}
	if got := elfDynStrings(t, lib, delf.DT_RUNPATH); got != doubled {
		t.Fatalf("precondition: RUNPATH = %q, want the doubled anchor %q", got, doubled)
	}
	return lib, doubled
}

func pointSonameInsideRunpath(t *testing.T, path string) {
	t.Helper()
	f, err := delf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Class != delf.ELFCLASS64 {
		t.Skip("the test patches 64-bit dynamic entries")
	}
	runpath, err := f.DynValue(delf.DT_RUNPATH)
	if err != nil || len(runpath) != 1 {
		t.Fatalf("DT_RUNPATH values = %v, %v", runpath, err)
	}
	dyn := f.Section(".dynamic")
	if dyn == nil {
		t.Fatal("no .dynamic section")
	}
	data, err := dyn.Data()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+16 <= len(data); i += 16 {
		if delf.DynTag(f.ByteOrder.Uint64(data[i:])) != delf.DT_SONAME {
			continue
		}
		var val [8]byte
		f.ByteOrder.PutUint64(val[:], runpath[0]+1)
		w, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		if _, err := w.WriteAt(val[:], int64(dyn.Offset)+int64(i)+8); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("no DT_SONAME entry to patch")
}

func elfDynStrings(t *testing.T, path string, tag delf.DynTag) string {
	t.Helper()
	f, err := delf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	vals, err := f.DynString(tag)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(vals, ":")
}
