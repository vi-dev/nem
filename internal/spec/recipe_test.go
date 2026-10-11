package spec

import "testing"

func TestMatchesPrefix(t *testing.T) {
	cases := []struct {
		version, prefix string
		want            bool
	}{
		{"1.24.3", "1", true},
		{"v1.24.3", "1", true},
		{"1.24.3", "1.24", true},
		{"1.24.3", "1.24.3", true},
		{"1.24.3", "1.2", false},
		{"10.0.0", "1", false},
		{"2.1.3-rc1", "2.1", true},
		{"2.1.3-rc1", "2.1.3", false},
		{"2.0.0", "2.1", false},
		{"1.0.0", "01", false},
	}
	for _, c := range cases {
		if got := MatchesPrefix(c.version, c.prefix); got != c.want {
			t.Errorf("MatchesPrefix(%q, %q) = %v, want %v", c.version, c.prefix, got, c.want)
		}
	}
}

func TestRecipeFileNames(t *testing.T) {
	if got := RecipeFileName("1.24"); got != "recipe@1.24.yaml" {
		t.Fatalf("RecipeFileName = %q", got)
	}
	good := map[string]string{"recipe@1.yaml": "1", "recipe@1.24.yaml": "1.24", "recipe@20.yaml": "20", "recipe@01.yaml": "01"}
	for name, want := range good {
		got, ok := ParseRecipeFileName(name)
		if !ok || got != want {
			t.Errorf("ParseRecipeFileName(%q) = %q, %v; want %q, true", name, got, ok, want)
		}
	}
	for _, bad := range []string{"recipe@v1.yaml", "recipe@1..yaml", "recipe@.yaml", "recipe@1.yml", "recipe.yaml", "pkg.yaml", "recipe@1.0-rc1.yaml"} {
		if _, ok := ParseRecipeFileName(bad); ok {
			t.Errorf("ParseRecipeFileName(%q) accepted", bad)
		}
	}
}

func twoRecipePackage() *Package {
	return &Package{
		Name: "gpgme",
		Versions: []VersionEntry{
			{Version: "2.2.0"}, {Version: "2.1.2"}, {Version: "2.1.0"}, {Version: "2.0.1"},
			{Version: "1.24.3"}, {Version: "1.24.2"}, {Version: "0.9.0"},
		},
		Base:   Recipe{Bins: []string{"base"}},
		Frozen: []Recipe{{Prefix: "1", Bins: []string{"one"}}, {Prefix: "2.1", Bins: []string{"two-one"}}},
	}
}

func TestRecipeSelection(t *testing.T) {
	p := twoRecipePackage()
	cases := map[string]string{
		"2.2.0": "", "2.0.1": "", "0.9.0": "",
		"1.24.3": "1", "1.24.2": "1",
		"2.1.2": "2.1", "2.1.0": "2.1",
	}
	for version, prefix := range cases {
		r, err := p.Recipe(version)
		if err != nil {
			t.Fatalf("Recipe(%s): %v", version, err)
		}
		if r.Prefix != prefix {
			t.Errorf("Recipe(%s).Prefix = %q, want %q", version, r.Prefix, prefix)
		}
	}
	if _, err := p.Recipe("4.0.0"); err == nil {
		t.Fatal("Recipe(4.0.0) must fail: version not offered")
	}
	if got := p.LatestRecipe().Prefix; got != "" {
		t.Fatalf("LatestRecipe().Prefix = %q, want base", got)
	}
}

func TestRecipeSelectionLongestPrefixWins(t *testing.T) {
	p := &Package{
		Name:     "x",
		Versions: []VersionEntry{{Version: "1.24.3"}, {Version: "1.23.0"}},
		Frozen:   []Recipe{{Prefix: "1"}, {Prefix: "1.24"}},
	}
	r, _ := p.Recipe("1.24.3")
	if r.Prefix != "1.24" {
		t.Fatalf("got %q, want 1.24", r.Prefix)
	}
	r, _ = p.Recipe("1.23.0")
	if r.Prefix != "1" {
		t.Fatalf("got %q, want 1", r.Prefix)
	}
}

func TestRecipeSupportedByAndEqual(t *testing.T) {
	r := &Recipe{Platforms: []Platform{{OS: "linux"}}}
	if got := r.SupportedBy(); len(got) != 2 || got[0].OS != "linux" {
		t.Fatalf("SupportedBy = %v", got)
	}
	if got := (&Recipe{}).SupportedBy(); len(got) != len(SupportedPlatforms) {
		t.Fatalf("empty platforms must mean all, got %v", got)
	}
	a := &Recipe{Prefix: "1", Bins: []string{"bin"}}
	b := &Recipe{Prefix: "1.24", Bins: []string{"bin"}}
	if !a.Equal(b) {
		t.Fatal("Equal must ignore Prefix")
	}
	b.Bins = []string{"sbin"}
	if a.Equal(b) {
		t.Fatal("Equal must compare the body")
	}
}
