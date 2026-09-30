package clidoc

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func nop(*cobra.Command, []string) error { return nil }

func testTree() *cobra.Command {
	root := &cobra.Command{Use: "nem", Short: "Root short", Long: "Root long."}
	root.PersistentFlags().String("color", "auto", "colorize output: auto, always, or never")
	root.AddGroup(&cobra.Group{ID: "env", Title: "Environment:"})

	use := &cobra.Command{
		Use:     "use <pkg>...",
		Short:   "Declare packages",
		Long:    "Long text with <angle> brackets.",
		Example: "  nem use kubectl   # newest\n  nem use go@1.27.0 # exact",
		GroupID: "env",
		Aliases: []string{"u"},
		RunE:    nop,
		Annotations: map[string]string{
			AnnotationGuideTitle: "Packages",
			AnnotationGuidePath:  "/docs/using/packages",
		},
	}
	use.Flags().BoolP("global", "g", false, "target the global manifest")
	use.Flags().StringArray("package", nil, "select <name> (repeatable; a|b)")
	use.Flags().String("output", "text", "output format: text or json")

	hidden := &cobra.Command{Use: "secret", Short: "Hidden", Hidden: true, RunE: nop}

	cat := &cobra.Command{Use: "catalog", Short: "Manage catalogs"}
	cat.AddGroup(&cobra.Group{ID: "c", Title: "Catalog consumption:"})
	add := &cobra.Command{Use: "add <name> <ref>", Short: "Add a catalog", GroupID: "c", RunE: nop}
	cat.AddCommand(add)

	ver := &cobra.Command{Use: "version", Short: "Print the version", RunE: nop}
	root.AddCommand(use, hidden, cat, ver)
	return root
}

func find(t *testing.T, root *cobra.Command, path string) *cobra.Command {
	t.Helper()
	c, _, err := root.Find(strings.Fields(path))
	if err != nil {
		t.Fatalf("find %q: %v", path, err)
	}
	return c
}

func mustContain(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestCommandPageFrontMatterAndSections(t *testing.T) {
	root := testTree()
	got := string(renderCommand(find(t, root, "use"), 4))
	mustContain(t, got,
		"---\ntitle: nem use\nweight: 4\n---\n",
		"Declare packages.\n",
		"Long text with &lt;angle&gt; brackets.\n",
		"## Usage\n\n```shell\nnem use <pkg>... [flags]\n```\n",
		"Aliases: `u`\n",
		"## Examples\n\n```shell\nnem use kubectl   # newest\nnem use go@1.27.0 # exact\n```\n",
		"## Flags\n\n| Flag | Description |\n|------|-------------|\n",
		"| `-g, --global` | target the global manifest |\n",
		"| `--output <string>` | output format: text or json (default `text`) |\n",
		"## Global flags\n\n`--color` applies to every command; see [nem](../nem/).\n",
		"## See also\n\n- [nem](../nem/)\n- Guide: [Packages]({{< relref \"/docs/using/packages\" >}})\n",
	)
}

func TestCommandPageEscapesTableCells(t *testing.T) {
	root := testTree()
	got := string(renderCommand(find(t, root, "use"), 4))
	mustContain(t, got, "| `--package <strings>` | select &lt;name&gt; (repeatable; a\\|b) |\n")
	if strings.Contains(got, "<name>") {
		t.Errorf("raw angle brackets leaked into a table cell:\n%s", got)
	}
}

func TestRootPageListsGroupedCommandsAndFlags(t *testing.T) {
	root := testTree()
	got := string(renderCommand(root, 1))
	mustContain(t, got,
		"---\ntitle: nem\nweight: 1\n---\n",
		"Root short.\n\nRoot long.\n",
		"## Flags\n\n| Flag | Description |\n|------|-------------|\n| `--color <string>` | colorize output: auto, always, or never (default `auto`) |\n",
		"## Commands\n\n### Environment\n\n| Command | Description |\n|---------|-------------|\n| [use](../nem-use/) | Declare packages |\n",
		"### Other commands\n\n| Command | Description |\n|---------|-------------|\n| [catalog](../nem-catalog/) | Manage catalogs |\n| [version](../nem-version/) | Print the version |\n",
	)
	for _, absent := range []string{"secret", "## Global flags", "## See also"} {
		if strings.Contains(got, absent) {
			t.Errorf("root page must not contain %q:\n%s", absent, got)
		}
	}
}

func TestParentWithoutGroupsListsFlatTable(t *testing.T) {
	root := &cobra.Command{Use: "nem"}
	cat := &cobra.Command{Use: "catalog", Short: "Manage catalogs"}
	cat.AddCommand(&cobra.Command{Use: "add <name> <ref>", Short: "Add a catalog", RunE: nop})
	root.AddCommand(cat)
	got := string(renderCommand(cat, 2))
	mustContain(t, got, "## Commands\n\n| Command | Description |\n|---------|-------------|\n| [add](../nem-catalog-add/) | Add a catalog |\n")
	if strings.Contains(got, "### ") {
		t.Errorf("flat table must have no group heading:\n%s", got)
	}
}

func TestPagesSkipsHiddenAndHelp(t *testing.T) {
	root := testTree()
	root.InitDefaultHelpCmd()
	var paths []string
	for _, p := range Pages(root) {
		paths = append(paths, p.Path)
	}
	want := []string{"_index.md", "nem.md", "nem-catalog.md", "nem-catalog-add.md", "nem-use.md", "nem-version.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("pages = %v, want %v", paths, want)
	}
}

func TestPagesAssignDepthFirstWeights(t *testing.T) {
	root := testTree()
	pages := Pages(root)
	mustContain(t, string(pages[1].Content), "title: nem\nweight: 1\n")
	mustContain(t, string(pages[3].Content), "title: nem catalog add\nweight: 3\n")
	mustContain(t, string(pages[5].Content), "title: nem version\nweight: 5\n")
}

func TestIndexPageGroupsEveryCommand(t *testing.T) {
	root := testTree()
	got := string(Pages(root)[0].Content)
	mustContain(t, got,
		"---\ntitle: Commands\nweight: 1\n---\n",
		"see [nem](nem/)",
		"### Environment\n\n| Command | Description |\n|---------|-------------|\n| [nem use](nem-use/) | Declare packages |\n",
		"### Other commands\n\n| Command | Description |\n|---------|-------------|\n| [nem catalog](nem-catalog/) | Manage catalogs |\n| [nem version](nem-version/) | Print the version |\n",
		"### Catalog consumption\n\n| Command | Description |\n|---------|-------------|\n| [nem catalog add](nem-catalog-add/) | Add a catalog |\n",
	)
	if strings.Contains(got, "secret") {
		t.Errorf("hidden command listed in index:\n%s", got)
	}
}

func TestWriteRemovesStalePages(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "nem-gone.md")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, Pages(testTree())); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale page still present: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("non-markdown file removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nem-use.md")); err != nil {
		t.Errorf("page not written: %v", err)
	}
}

func TestWriteCreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "cli")
	if err := Write(dir, Pages(testTree())); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_index.md")); err != nil {
		t.Errorf("index not written: %v", err)
	}
}

type pipeValue string

func (p *pipeValue) String() string     { return string(*p) }
func (p *pipeValue) Set(s string) error { *p = pipeValue(s); return nil }
func (p *pipeValue) Type() string       { return "days|hours" }

func TestFlagNameCellEscapesPipeInType(t *testing.T) {
	root := &cobra.Command{Use: "nem"}
	c := &cobra.Command{Use: "clean", Short: "Reclaim", RunE: nop}
	var v pipeValue
	c.Flags().Var(&v, "unused", "evict old versions")
	root.AddCommand(c)
	got := string(renderCommand(c, 2))
	mustContain(t, got, "| `--unused <days\\|hours>` | evict old versions |\n")
}

func TestIndentedLongRendersAsCodeBlock(t *testing.T) {
	root := &cobra.Command{Use: "nem"}
	c := &cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completions",
		Long:  "Load completions in your session:\n\n\tsource <(nem completion zsh)\n\n#### Linux:\n\n\tnem completion zsh > \"${fpath[1]}/_nem\"\n",
		RunE:  nop,
	}
	root.AddCommand(c)
	got := string(renderCommand(c, 2))
	mustContain(t, got, "```text\nLoad completions in your session:\n\n\tsource <(nem completion zsh)\n\n#### Linux:\n\n\tnem completion zsh > \"${fpath[1]}/_nem\"\n```\n")
	fence := strings.Index(got, "```text\n")
	if strings.Contains(got, "&lt;") || strings.Index(got, "#### Linux") < fence {
		t.Errorf("indented Long leaked entities or a heading outside the fence:\n%s", got)
	}
}

func TestShortEndingInPeriodIsNotDoubled(t *testing.T) {
	root := &cobra.Command{Use: "nem", Short: "Dev environments. For everyone."}
	got := string(renderCommand(root, 1))
	if strings.Contains(got, "everyone..") {
		t.Errorf("double period:\n%s", got)
	}
	mustContain(t, got, "Dev environments. For everyone.\n")
}

func TestIndexListsOtherCommandsLast(t *testing.T) {
	got := string(Pages(testTree())[0].Content)
	other := strings.Index(got, "### Other commands")
	consumption := strings.Index(got, "### Catalog consumption")
	if other < 0 || consumption < 0 || other < consumption {
		t.Errorf("Other commands (%d) must come after child groups (%d):\n%s", other, consumption, got)
	}
}
