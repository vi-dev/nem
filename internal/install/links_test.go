package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func linkDir(t *testing.T) string {
	t.Helper()
	dir, err := testx.Home(t).PackageDir("gnupg", "2.5.22")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readLink(t *testing.T, dir, dep string) string {
	t.Helper()
	target, err := os.Readlink(filepath.Join(dir, DepsDir, dep))
	if err != nil {
		t.Fatalf("readlink %s: %v", dep, err)
	}
	return target
}

func TestWriteLinksCreatesRelativeSymlinks(t *testing.T) {
	dir := linkDir(t)
	if _, err := WriteLinks(dir, map[string]string{"libgcrypt": "1.12.3", "zlib": "1.3.2"}); err != nil {
		t.Fatalf("WriteLinks: %v", err)
	}
	if got := readLink(t, dir, "libgcrypt"); got != filepath.Join("..", "..", "..", "libgcrypt", "1.12.3") {
		t.Fatalf("libgcrypt -> %q", got)
	}
	if got := readLink(t, dir, "zlib"); got != filepath.Join("..", "..", "..", "zlib", "1.3.2") {
		t.Fatalf("zlib -> %q", got)
	}
	got, err := ReadLinks(dir)
	if err != nil || got["libgcrypt"] != "1.12.3" || got["zlib"] != "1.3.2" || len(got) != 2 {
		t.Fatalf("ReadLinks = %v, %v", got, err)
	}
}

func TestWriteLinksRetargetsAndPrunes(t *testing.T) {
	dir := linkDir(t)
	if _, err := WriteLinks(dir, map[string]string{"libgcrypt": "1.12.2", "zlib": "1.3.2"}); err != nil {
		t.Fatal(err)
	}
	retargeted, err := WriteLinks(dir, map[string]string{"libgcrypt": "1.12.3"})
	if err != nil {
		t.Fatalf("second WriteLinks: %v", err)
	}
	if len(retargeted) != 1 || retargeted["libgcrypt"] != "1.12.2" {
		t.Fatalf("retargeted = %v, want the old libgcrypt version only", retargeted)
	}
	if got := readLink(t, dir, "libgcrypt"); got != filepath.Join("..", "..", "..", "libgcrypt", "1.12.3") {
		t.Fatalf("libgcrypt not retargeted: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, DepsDir, "zlib")); !os.IsNotExist(err) {
		t.Fatalf("zlib link not pruned: %v", err)
	}
}

func TestWriteLinksEmptyRemovesDirectory(t *testing.T) {
	dir := linkDir(t)
	if _, err := WriteLinks(dir, map[string]string{"zlib": "1.3.2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteLinks(dir, nil); err != nil {
		t.Fatalf("WriteLinks(nil): %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, DepsDir)); !os.IsNotExist(err) {
		t.Fatalf(".nem-link-dependencies must be gone: %v", err)
	}
	got, err := ReadLinks(dir)
	if err != nil || len(got) != 0 {
		t.Fatalf("ReadLinks on missing dir = %v, %v; want empty, nil", got, err)
	}
}

func TestWriteLinksRemovesStaleTempLink(t *testing.T) {
	dir := linkDir(t)
	if err := os.MkdirAll(filepath.Join(dir, DepsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../zlib/1.0.0", filepath.Join(dir, DepsDir, "zlib.tmp")); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteLinks(dir, map[string]string{"zlib": "1.3.2"}); err != nil {
		t.Fatalf("WriteLinks: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, DepsDir))
	if len(entries) != 1 || entries[0].Name() != "zlib" {
		t.Fatalf("entries = %v, want only zlib", entries)
	}
}

func TestWriteLinksKeepsEveryLinkWhenValidationFails(t *testing.T) {
	dir := linkDir(t)
	depsPath := filepath.Join(dir, DepsDir)
	if err := os.MkdirAll(depsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../libassuan/3.0.2", filepath.Join(depsPath, "libassuan")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../libgcrypt/1.12.2", filepath.Join(depsPath, "libgcrypt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depsPath, "notes"), []byte("shipped"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := WriteLinks(dir, map[string]string{"libgcrypt": "1.12.3"})
	if err == nil || !strings.Contains(err.Error(), "is not a symlink") {
		t.Fatalf("WriteLinks = %v, want an is-not-a-symlink error", err)
	}
	if got := readLink(t, dir, "libassuan"); got != filepath.Join("..", "..", "..", "libassuan", "3.0.2") {
		t.Fatalf("libassuan -> %q, want the unwanted link left in place", got)
	}
	if got := readLink(t, dir, "libgcrypt"); got != filepath.Join("..", "..", "..", "libgcrypt", "1.12.2") {
		t.Fatalf("libgcrypt -> %q, want the stale target untouched", got)
	}
	if got, _ := os.ReadFile(filepath.Join(depsPath, "notes")); string(got) != "shipped" {
		t.Fatalf("package content = %q, want it intact", got)
	}
}

func TestLinksFiltersKindAndPlatform(t *testing.T) {
	other := spec.Platform{OS: "plan9", Arch: "mips"}
	pkg := &spec.Package{Name: "gnupg", Deps: []spec.Dep{
		{Name: "libgcrypt", Kind: spec.DepKindLink, Compat: "1"},
		{Name: "pinentry", Kind: spec.DepKindRun},
		{Name: "ncurses", Kind: spec.DepKindLink, Platforms: []spec.Platform{other}},
		{Name: "zlib", Kind: spec.DepKindLink},
	}}
	versions := map[string]string{"libgcrypt": "1.12.3", "pinentry": "1.3.1", "ncurses": "6.6"}
	got := Links(pkg, versions)
	if len(got) != 1 || got["libgcrypt"] != "1.12.3" {
		t.Fatalf("Links = %v, want only libgcrypt 1.12.3 (run dep, other-platform dep, unresolved dep excluded)", got)
	}
}
