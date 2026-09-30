package clidoc

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	AnnotationGuideTitle = "docs.guide.title"
	AnnotationGuidePath  = "docs.guide.path"
)

type Page struct {
	Path    string
	Content []byte
}

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", `\|`)

func escape(s string) string { return escaper.Replace(s) }

func slug(c *cobra.Command) string { return strings.ReplaceAll(c.CommandPath(), " ", "-") }

func visible(c *cobra.Command) bool { return c.IsAvailableCommand() }

func Pages(root *cobra.Command) []Page {
	var cmds []*cobra.Command
	collect(root, &cmds)
	pages := make([]Page, 0, len(cmds)+1)
	pages = append(pages, Page{Path: "_index.md", Content: renderIndex(root)})
	for i, c := range cmds {
		pages = append(pages, Page{Path: slug(c) + ".md", Content: renderCommand(c, i+1)})
	}
	return pages
}

func collect(c *cobra.Command, out *[]*cobra.Command) {
	*out = append(*out, c)
	for _, sub := range c.Commands() {
		if visible(sub) {
			collect(sub, out)
		}
	}
}

func Write(dir string, pages []Page) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	keep := make(map[string]bool, len(pages))
	for _, p := range pages {
		keep[p.Path] = true
		if err := os.WriteFile(filepath.Join(dir, p.Path), p.Content, 0o644); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || keep[name] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func renderIndex(root *cobra.Command) []byte { return render("index", indexData(root)) }

func renderCommand(c *cobra.Command, weight int) []byte {
	return render("command", commandData(c, weight))
}
