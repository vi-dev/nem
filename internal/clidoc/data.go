package clidoc

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type link struct {
	Text string
	Href string
}

type table struct {
	Heading string
	First   string
	Rows    [][2]string
}

type commandPage struct {
	Title       string
	Weight      int
	Short       string
	Long        string
	Usage       string
	Aliases     []string
	Example     string
	Flags       table
	Commands    []table
	GlobalFlags []string
	Root        link
	Parent      *link
	Guide       *link
}

type indexPage struct {
	Name        string
	GlobalFlags []string
	Root        link
	Tables      []table
}

func commandData(c *cobra.Command, weight int) commandPage {
	p := commandPage{
		Title:   c.CommandPath(),
		Weight:  weight,
		Short:   sentence(c.Short),
		Long:    long(c.Long),
		Usage:   c.UseLine(),
		Aliases: c.Aliases,
		Example: dedent(c.Example),
		Flags:   table{Heading: "## Flags", First: "Flag", Rows: flagRows(c.NonInheritedFlags())},
	}
	if c.HasAvailableSubCommands() {
		p.Commands = groupTables(c, "../", false, true)
	}
	if c.HasParent() {
		root := c.Root()
		p.GlobalFlags = flagNames(c.InheritedFlags())
		p.Root = link{Text: root.CommandPath(), Href: "../" + slug(root) + "/"}
		p.Parent = &link{Text: c.Parent().CommandPath(), Href: "../" + slug(c.Parent()) + "/"}
		if title, path := c.Annotations[AnnotationGuideTitle], c.Annotations[AnnotationGuidePath]; title != "" && path != "" {
			p.Guide = &link{Text: title, Href: fmt.Sprintf("{{< relref %q >}}", path)}
		}
	}
	return p
}

func indexData(root *cobra.Command) indexPage {
	p := indexPage{
		Name:        root.Name(),
		GlobalFlags: flagNames(root.PersistentFlags()),
		Root:        link{Text: root.CommandPath(), Href: slug(root) + "/"},
		Tables:      groupTables(root, "", true, false),
	}
	for _, c := range root.Commands() {
		if visible(c) && len(c.Groups()) > 0 {
			p.Tables = append(p.Tables, groupTables(c, "", true, true)...)
		}
	}
	if t, ok := otherTable(root, "", true); ok {
		p.Tables = append(p.Tables, t)
	}
	return p
}

func sentence(short string) string {
	short = escape(short)
	if strings.HasSuffix(short, ".") {
		return short
	}
	return short + "."
}

func long(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "#") {
			return "```text\n" + strings.TrimRight(text, "\n") + "\n```"
		}
	}
	return escape(text)
}

func dedent(example string) string {
	lines := strings.Split(strings.TrimRight(example, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "  ")
	}
	return strings.Join(lines, "\n")
}

func flagNames(fs *pflag.FlagSet) []string {
	var names []string
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		names = append(names, "--"+f.Name)
	})
	return names
}

func flagRows(fs *pflag.FlagSet) [][2]string {
	var rows [][2]string
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		varname, usage := pflag.UnquoteUsage(f)
		if varname == "stringArray" {
			varname = "strings"
		}
		if varname != "" {
			name += " <" + varname + ">"
		}
		desc := escape(usage)
		if d := f.DefValue; f.Value.Type() != "bool" && d != "" && d != "0" && d != "[]" {
			desc += fmt.Sprintf(" (default `%s`)", d)
		}
		rows = append(rows, [2]string{"`" + strings.ReplaceAll(name, "|", `\|`) + "`", desc})
	})
	return rows
}

func commandRow(c *cobra.Command, prefix string, fullPath bool) [2]string {
	label := c.Name()
	if fullPath {
		label = c.CommandPath()
	}
	return [2]string{fmt.Sprintf("[%s](%s%s/)", label, prefix, slug(c)), escape(c.Short)}
}

func groupTables(parent *cobra.Command, prefix string, fullPath, withOther bool) []table {
	var tables []table
	for _, g := range parent.Groups() {
		var rows [][2]string
		for _, c := range parent.Commands() {
			if visible(c) && c.GroupID == g.ID {
				rows = append(rows, commandRow(c, prefix, fullPath))
			}
		}
		if len(rows) > 0 {
			tables = append(tables, table{Heading: "### " + strings.TrimSuffix(g.Title, ":"), First: "Command", Rows: rows})
		}
	}
	if withOther {
		if t, ok := otherTable(parent, prefix, fullPath); ok {
			tables = append(tables, t)
		}
	}
	return tables
}

func otherTable(parent *cobra.Command, prefix string, fullPath bool) (table, bool) {
	groups := parent.Groups()
	var rows [][2]string
	for _, c := range parent.Commands() {
		if visible(c) && (len(groups) == 0 || c.GroupID == "") {
			rows = append(rows, commandRow(c, prefix, fullPath))
		}
	}
	if len(rows) == 0 {
		return table{}, false
	}
	heading := ""
	if len(groups) > 0 {
		heading = "### Other commands"
	}
	return table{Heading: heading, First: "Command", Rows: rows}, true
}
