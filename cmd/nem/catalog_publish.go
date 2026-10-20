package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vi-dev/clix"

	"github.com/vi-dev/nem/internal/publish"
)

func newCatalogPublishCmd() *cobra.Command {
	var tags []string
	var dryRun, force bool
	var output string
	cmd := &cobra.Command{
		Use:         "publish <ref> [catalog]",
		Aliases:     []string{"pub"},
		Short:       "Publish a catalog to an OCI registry",
		Annotations: guide("Publishing your own catalog", "/docs/managing-catalogs/publish-your-own"),
		Long: "Lint the catalog, then push its package manifests to <ref> as an OCI index and " +
			"move the given tags to it. Packages are compared with the index the tags point " +
			"at: unchanged manifests are not pushed again unless --force, and --dry-run lists " +
			"what would change. --output json prints the ref, tags, index digest and changed " +
			"packages on stdout. Archives built with nem catalog build --push and nem " +
			"catalog fill live beside the index and are not touched.",
		Example: "  nem catalog publish registry.example/nem/catalog .                        # publish a checkout under tag v3\n" +
			"  nem catalog publish registry.example/nem/catalog --tag v3 --tag 2026.09   # several tags\n" +
			"  nem catalog publish registry.example/nem/catalog --dry-run                # what would change",
		Args: cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return completeYAMLOrDir(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clix.ValidateOutput(output); err != nil {
				return err
			}
			ref := args[0]
			cat := "."
			if len(args) == 2 {
				cat = args[1]
			}
			findings, err := publish.Lint(cmd.Context(), cat)
			if err != nil {
				return err
			}
			if len(findings) > 0 {
				for _, f := range findings {
					ui.Warn(f.String())
				}
				return &ExitError{Code: 1}
			}
			res, err := publish.Publish(cmd.Context(), cat, ref, publish.Options{Tags: tags, DryRun: dryRun, Force: force})
			if err != nil {
				return err
			}
			if output == "json" {
				if err := ui.JSON(publishDocument(ref, res, force)); err != nil {
					return err
				}
			} else if dryRun {
				renderPublishPlan(res)
			}
			if dryRun {
				ui.Successf("Would publish %s: %s, tags %s", ref, publishPlanSummary(res), strings.Join(res.Tags, ", "))
				return nil
			}
			ui.Successf("Published %s: %s, tags %s", ref, publishedSummary(res), strings.Join(res.Tags, ", "))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag to move to the published index (repeatable; default v3)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without writing anything")
	cmd.Flags().BoolVar(&force, "force", false, "push every package manifest even when unchanged")
	clix.AddOutputFlag(cmd, &output)
	return cmd
}

type publishResult struct {
	Ref      string            `json:"ref"`
	Tags     []string          `json:"tags"`
	Index    string            `json:"index"`
	Packages []publish.Package `json:"packages"`
}

func publishDocument(ref string, res *publish.Result, force bool) publishResult {
	changed := make([]publish.Package, 0, len(res.Packages))
	for _, p := range res.Packages {
		if force || p.Status != publish.StatusUnchanged {
			changed = append(changed, p)
		}
	}
	return publishResult{Ref: ref, Tags: res.Tags, Index: res.Index, Packages: changed}
}

func renderPublishPlan(res *publish.Result) {
	rows := make([][]string, 0, len(res.Packages))
	for _, p := range res.Packages {
		if p.Status == publish.StatusUnchanged {
			continue
		}
		rows = append(rows, []string{p.Name, orDash(p.Version), p.Status})
	}
	if len(rows) == 0 {
		return
	}
	ui.Table([]string{"PACKAGE", "VERSION", "STATUS"}, rows)
}

func publishPlanSummary(res *publish.Result) string {
	var groups []string
	if res.Pushed > 0 {
		groups = append(groups, fmt.Sprintf("%d to push", res.Pushed))
	}
	if res.Removed > 0 {
		groups = append(groups, fmt.Sprintf("%d removed", res.Removed))
	}
	if res.Unchanged > 0 {
		groups = append(groups, fmt.Sprintf("%d unchanged", res.Unchanged))
	}
	return strings.Join(groups, ", ")
}

func publishedSummary(res *publish.Result) string {
	s := fmt.Sprintf("%d pushed", res.Pushed)
	if res.Removed > 0 {
		s += fmt.Sprintf(", %d removed", res.Removed)
	}
	return s + fmt.Sprintf(", %d unchanged", res.Unchanged)
}
