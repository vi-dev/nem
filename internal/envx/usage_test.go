package envx

import (
	"testing"
	"time"

	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/testx"
	"github.com/vi-dev/nem/internal/usage"
)

func TestStampUsageWithNoEntriesWritesNothing(t *testing.T) {
	root := t.TempDir()
	h := testx.HomeAt(root)
	stampUsage(h, nil, nil)
	if len(usage.Load(h)) != 0 {
		t.Fatal("empty composition should not write an index")
	}
}

func TestComposeStampsResolvedVersions(t *testing.T) {
	root := t.TempDir()
	h := testx.HomeAt(root)

	projectLock := &project.Lockfile{Packages: []project.LockEntry{
		{Name: "proj-tool", Version: "1.0.0", Direct: true, Platforms: []string{currentPlatform()}},
	}}
	globalLock := &project.Lockfile{Packages: []project.LockEntry{
		{Name: "global-tool", Version: "2.0.0", Direct: true, Platforms: []string{currentPlatform()}},
	}}
	metaLookup := mapMetaLookup(map[string]*install.Meta{
		metaKey("proj-tool", "1.0.0"):   {},
		metaKey("global-tool", "2.0.0"): {},
	})

	Compose(&project.Manifest{}, &project.Manifest{}, projectLock, globalLock, h, metaLookup, mapGetenv(nil))

	idx := usage.Load(h)
	if _, ok := idx.LastUsed("proj-tool", "1.0.0"); !ok {
		t.Error("Compose must stamp the project entry it resolved")
	}
	if _, ok := idx.LastUsed("global-tool", "2.0.0"); !ok {
		t.Error("Compose must stamp the global entry it resolved")
	}
}

func TestComposeScopeLeavesUsageIndexUntouched(t *testing.T) {
	root := t.TempDir()
	h := testx.HomeAt(root)

	seeded := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	usage.Stamp(h, seeded, []string{usage.Key("preexisting", "1.0.0")})

	scopeLock := &project.Lockfile{Packages: []project.LockEntry{
		{Name: "scope-tool", Version: "1.0.0", Direct: true, Platforms: []string{currentPlatform()}},
	}}
	otherLock := &project.Lockfile{Packages: []project.LockEntry{
		{Name: "other-tool", Version: "1.0.0", Direct: true, Platforms: []string{currentPlatform()}},
	}}
	metaLookup := mapMetaLookup(map[string]*install.Meta{
		metaKey("scope-tool", "1.0.0"): {},
		metaKey("other-tool", "1.0.0"): {},
	})

	ComposeScope(&project.Manifest{}, &project.Manifest{}, scopeLock, otherLock, h, metaLookup, mapGetenv(nil))

	idx := usage.Load(h)
	if len(idx) != 1 {
		t.Fatalf("usage index changed by ComposeScope inspection: %v", idx)
	}
	got, ok := idx.LastUsed("preexisting", "1.0.0")
	if !ok || !got.Equal(seeded) {
		t.Fatalf("preexisting stamp changed to %v, ok=%v, want %v", got, ok, seeded)
	}
	if _, ok := idx.LastUsed("scope-tool", "1.0.0"); ok {
		t.Fatal("ComposeScope must not stamp the scope versions it resolved")
	}
	if _, ok := idx.LastUsed("other-tool", "1.0.0"); ok {
		t.Fatal("ComposeScope must not stamp the other-scope versions it resolved")
	}
}
