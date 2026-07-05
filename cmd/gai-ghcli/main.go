package main

import (
	"os"

	"github.com/cyphix/gaighcli/internal/cli"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaisdk"
)

func main() {
	os.Exit(gaisdk.RunCLI(gaisdk.Options[*context.RepoContext]{
		Description:    cli.Description,
		Version:        cli.Version,
		ModulePath:     cli.ModulePath,
		TopLevelHelp:   cli.TopHelp,
		Home:           cli.HomeHandler,
		Commands:       cli.Commands(),
		GetCommandHelp: cli.GetCommandHelp,
		ResolveContext: cli.ResolveRepoContext,
	}))
}
