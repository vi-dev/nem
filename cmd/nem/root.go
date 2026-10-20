package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"github.com/vi-dev/clix"
	"github.com/vi-dev/clix/console"

	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/netx"
)

var (
	ui      *console.Console
	nemHome home.Home
	ranHook bool
)

func nemOptions() clix.Options {
	return clix.Options{
		App:       "nem",
		Version:   version,
		EnvPrefix: "NEM",
		Hint:      hintFor,
		Setup:     setup,
	}
}

func setup(ctx context.Context, c *console.Console) (context.Context, error) {
	ui = c
	ranHook = true
	nemHome = home.Resolve(os.Getenv)
	hosts, warnings := config.LoadHostSettingsLenient(nemHome)
	for _, w := range warnings {
		c.Warnf("%s", w)
	}
	settings := make(map[string]netx.HostSettings, len(hosts))
	for host, e := range hosts {
		settings[host] = netx.HostSettings{CA: e.CA, PlainHTTP: e.PlainHTTP, Insecure: e.Insecure}
	}
	netx.Set(settings)
	return ctx, nil
}

func newRoot() *cobra.Command {
	ranHook = false
	ui = nil
	nemHome = home.Home{}
	root := &cobra.Command{
		Use:   "nem",
		Short: "Reproducible dev environments. For you, your teams, and your agents.",
		Long: "nem gives each project its own packages and environment variables, " +
			"declared in nem.toml and pinned with digests in nem.lock. Packages install " +
			"under NEM_HOME and join PATH only while the shell is inside the project, " +
			"so teammates, CI, and agents reproduce the same environment from the two files.",
		Example: "  nem activate                 # hook nem into the current shell\n" +
			"  nem use kubectl go@1.27.0    # declare packages in this project\n" +
			"  nem sync                     # install what nem.lock pins",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("{{.Version}}\n")

	root.AddGroup(
		&cobra.Group{ID: groupEnvironment, Title: "Environment:"},
		&cobra.Group{ID: groupDiscovery, Title: "Discovery:"},
		&cobra.Group{ID: groupCatalogs, Title: "Catalogs:"},
		&cobra.Group{ID: groupShell, Title: "Shell integration:"},
		&cobra.Group{ID: groupMaintenance, Title: "Maintenance:"},
	)
	addGrouped(root, groupEnvironment, newUseCmd(), newUnuseCmd(), newUpdateCmd(), newLockCmd(), newSyncCmd(), newStatusCmd(), newExecCmd(), newWhichCmd())
	addGrouped(root, groupDiscovery, newSearchCmd(), newInfoCmd())
	addGrouped(root, groupCatalogs, newCatalogCmd())
	addGrouped(root, groupShell, newActivateCmd(), newDeactivateCmd(), newEnvCmd())
	addGrouped(root, groupMaintenance, newCleanCmd(), newSelfCmd())
	root.AddCommand(newVersionCmd(), newGendocsCmd())
	return root
}

const (
	groupEnvironment = "environment"
	groupDiscovery   = "discovery"
	groupCatalogs    = "catalogs"
	groupShell       = "shell"
	groupMaintenance = "maintenance"
)

func addGrouped(parent *cobra.Command, groupID string, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = groupID
	}
	parent.AddCommand(cmds...)
}
