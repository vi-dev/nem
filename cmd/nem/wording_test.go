package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/vi-dev/nem/internal/resolve"
)

var toolWord = regexp.MustCompile(`(?i)\btools?\b`)

func TestHelpTextSaysPackageNotTool(t *testing.T) {
	root := newRoot()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Name() == "completion" {
			return
		}
		for _, field := range []struct{ name, text string }{
			{"Use", c.Use}, {"Short", c.Short}, {"Long", c.Long}, {"Example", c.Example},
		} {
			if toolWord.MatchString(field.text) {
				t.Errorf("%s: %s says tool: %q", c.CommandPath(), field.name, field.text)
			}
		}
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if toolWord.MatchString(f.Usage) {
				t.Errorf("%s --%s: usage says tool: %q", c.CommandPath(), f.Name, f.Usage)
			}
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func TestNarrationSaysPackageNotTool(t *testing.T) {
	for _, text := range []string{
		hintFor(&resolve.PinConflictError{Name: "a", Pinned: "1", Required: "2"}),
		hintFor(&resolve.CompatConflictError{Name: "a", Compats: []string{"1"}}),
		(&UnpinnedToolsError{Path: "nem.toml", Names: []string{"a"}}).Error(),
	} {
		if toolWord.MatchString(text) {
			t.Errorf("says tool: %q", text)
		}
	}
}

func TestEveryVisibleCommandHasLongAndExample(t *testing.T) {
	root := newRoot()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			return
		}
		if strings.TrimSpace(c.Long) == "" {
			t.Errorf("%s: no Long", c.CommandPath())
		}
		if strings.TrimSpace(c.Example) == "" {
			t.Errorf("%s: no Example", c.CommandPath())
		}
		for _, line := range strings.Split(c.Example, "\n") {
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "  ") || !strings.Contains(line, "  # ") {
				t.Errorf("%s: example line %q must be indented two spaces and end in an aligned # comment", c.CommandPath(), line)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
