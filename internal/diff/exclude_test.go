package diff

import (
	"slices"
	"strings"
	"testing"
)

const excludeBase = `schema: 4
name: tool
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
versions:
  - 1.1.0
  - version: 1.0.0
    excluded: "broken"
`

const excludeTarget = `schema: 3
name: tool
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
versions: [1.1.0, 1.0.0]
`

func TestCompareReportsNewExclusion(t *testing.T) {
	local := writeFixture(t, map[string]string{"tool": excludeBase})
	remote := writeFixture(t, map[string]string{"tool": excludeTarget})
	res := compare(t, local, remote)
	if len(res.Changes) != 1 {
		t.Fatalf("changes: %+v", res.Changes)
	}
	c := res.Changes[0]
	if c.Status != StatusUpdated || !slices.Equal(c.Excluded, []string{"1.0.0"}) {
		t.Fatalf("change: %+v", c)
	}
	if slices.Contains(c.Diff, "1.0.0") {
		t.Fatalf("an excluded version is never in the build diff: %+v", c.Diff)
	}
}

func TestCompareNewPackageListsItsExclusions(t *testing.T) {
	local := writeFixture(t, map[string]string{"tool": excludeBase})
	remote := writeFixture(t, map[string]string{"other": strings.Replace(excludeTarget, "name: tool", "name: other", 1)})
	res := compare(t, local, remote)
	for _, c := range res.Changes {
		if c.Name == "tool" && (c.Status != StatusNew || !slices.Equal(c.Excluded, []string{"1.0.0"}) || !slices.Equal(c.Diff, []string{"1.1.0"})) {
			t.Fatalf("change: %+v", c)
		}
	}
}

func TestCompareReportsLiftedExclusion(t *testing.T) {
	local := writeFixture(t, map[string]string{"tool": strings.Replace(excludeTarget, "schema: 3", "schema: 4", 1)})
	remote := writeFixture(t, map[string]string{"tool": excludeBase})
	res := compare(t, local, remote)
	if len(res.Changes) != 1 {
		t.Fatalf("changes: %+v", res.Changes)
	}
	c := res.Changes[0]
	if !slices.Equal(c.Unexcluded, []string{"1.0.0"}) || len(c.Excluded) != 0 || !slices.Contains(c.Diff, "1.0.0") {
		t.Fatalf("a lifted exclusion must be reported and enter the build diff: %+v", c)
	}
}
