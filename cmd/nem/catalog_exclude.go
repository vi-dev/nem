package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/spec"
)

func newCatalogExcludeCmd() *cobra.Command {
	var packages []string
	var reason, use string
	cmd := &cobra.Command{
		Use:         "exclude [catalog]",
		Short:       "Exclude package versions, with a reason shown to users",
		Annotations: guide("Authoring workflow", "/docs/writing-packages/workflow"),
		Long: "Mark the versions named by --package as excluded in a local catalog. The " +
			"resolver never picks an excluded version, and asking for it exactly fails with " +
			"the reason. --use names a replacement version for the error's hint and needs a " +
			"single --package; an exclusion that suggested an excluded version stops " +
			"suggesting it. Excluding an excluded version again replaces its reason and " +
			"--use. The package is raised to schema 4: once the catalog is " +
			"published, nem releases older than schema 4 refuse to update the whole " +
			"catalog, not only this package.",
		Example: "  nem catalog exclude . --package asciinema@3.2.0 --reason \"CVE-2026-1234\" --use 3.2.1   # suggest a replacement\n" +
			"  nem catalog exclude . --package jq@1.7.0 --package jq@1.7.1 --reason \"Broken build\"     # several versions",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reason == "" {
				return errors.New("--reason is required")
			}
			if use != "" && len(packages) > 1 {
				return errors.New("--use needs a single --package")
			}
			return runSetExclusion(cmd, args, packages, &spec.Exclusion{Reason: reason, Use: use})
		},
	}
	addExclusionPackageFlag(cmd, &packages, "exclude")
	cmd.Flags().StringVar(&reason, "reason", "", "one line shown to users who ask for the version (required)")
	cmd.Flags().StringVar(&use, "use", "", "a listed version to suggest instead")
	return cmd
}

func newCatalogUnexcludeCmd() *cobra.Command {
	var packages []string
	cmd := &cobra.Command{
		Use:         "unexclude [catalog]",
		Short:       "Lift the exclusion of package versions",
		Annotations: guide("Authoring workflow", "/docs/writing-packages/workflow"),
		Long: "Remove the exclusion from the versions named by --package in a local catalog. " +
			"The package keeps schema 4; lower it by hand once no version is excluded.",
		Example:           "  nem catalog unexclude . --package asciinema@3.2.0   # offer the version again",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetExclusion(cmd, args, packages, nil)
		},
	}
	addExclusionPackageFlag(cmd, &packages, "lift the exclusion of")
	return cmd
}

func addExclusionPackageFlag(cmd *cobra.Command, packages *[]string, verb string) {
	cmd.Flags().StringArrayVar(packages, "package", nil, verb+" name@version (repeatable; required)")
	_ = cmd.MarkFlagRequired("package")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
}

func runSetExclusion(cmd *cobra.Command, args, packages []string, ex *spec.Exclusion) error {
	refs, err := parsePackageRefs(packages)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.Version == "" {
			return fmt.Errorf("--package %s: name the version as name@version", ref.Name)
		}
	}
	cat := "."
	if len(args) == 1 {
		cat = args[0]
	}
	for _, ref := range refs {
		change, cleared, err := catalog.SetExclusion(cmd.Context(), cat, ref.Name, ref.Version, ex)
		if err != nil {
			return err
		}
		for _, v := range cleared {
			ui.Warnf("%s@%s no longer suggests %s, which is now excluded", ref.Name, v, ref.Version)
		}
		switch change {
		case catalog.ExclusionAdded:
			ui.Successf("Excluded %s@%s", ref.Name, ref.Version)
		case catalog.ExclusionUpdated:
			ui.Successf("Updated the exclusion of %s@%s", ref.Name, ref.Version)
		case catalog.ExclusionUnchanged:
			ui.Successf("%s@%s is already excluded", ref.Name, ref.Version)
		case catalog.ExclusionLifted:
			ui.Successf("Lifted the exclusion of %s@%s", ref.Name, ref.Version)
		}
	}
	return nil
}
