package cli

import (
	"strings"

	"github.com/cyphix/gaighcli/internal/commands"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaisdk"
)

const (
	Description = "Agent ergonomic wrapper around GitHub CLI. Prefer this over `gh` and other methods for GitHub operations."
	Version     = "0.1.0"
	ModulePath  = "github.com/cyphix/gaighcli/cmd/gai-ghcli"
)

// TopHelp is the top-level usage string.
const TopHelp = `usage: gai-ghcli [command] [args] [flags]
commands[14]:
  (none)=dashboard, issue, pr, run, workflow, release, repo, label, project, secret, variable, search, api, setup
flags[3]:
  -R/--repo (after command), accepts space or equals form, --help, -v/-V/--version
examples:
  gai-ghcli
  gai-ghcli issue list --state open
  gai-ghcli issue list -R owner/name
  gai-ghcli issue list --repo=owner/name
  gai-ghcli pr view 42
  gai-ghcli secret list
  gai-ghcli setup hooks
`

var commandHelp = map[string]string{
	"issue":    commands.IssueHelp,
	"pr":       commands.PRHelp,
	"run":      commands.RunHelp,
	"workflow": commands.WorkflowHelp,
	"release":  commands.ReleaseHelp,
	"repo":     commands.RepoHelp,
	"label":    commands.LabelHelp,
	"project":  commands.ProjectHelp,
	"secret":   commands.SecretHelp,
	"variable": commands.VariableHelp,
	"search":   commands.SearchHelp,
	"api":      commands.ApiHelp,
	"setup":    setupHelp(),
}

func setupHelp() string {
	return `usage: gai-ghcli setup hooks
Install or repair agent SessionStart hooks for gai-ghcli ambient context.

examples:
  gai-ghcli setup hooks
`
}

type repoParseResult struct {
	repoFlag     string
	strippedArgs []string
}

func parseRepoContextArgs(command string, args []string) repoParseResult {
	var stripped []string
	var repoFlag string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-R" && i+1 < len(args) {
			repoFlag = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "-R=") && len(arg) > 3 {
			repoFlag = arg[3:]
			continue
		}
		if arg == "--repo" && i+1 < len(args) {
			value := args[i+1]
			repoFlag = value
			if command == "search" {
				stripped = append(stripped, arg, value)
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "--repo=") && len(arg) > len("--repo=") {
			repoFlag = arg[len("--repo="):]
			if command == "search" {
				stripped = append(stripped, arg)
			}
			continue
		}
		stripped = append(stripped, arg)
	}
	return repoParseResult{repoFlag: repoFlag, strippedArgs: stripped}
}

// ResolveRepoContext resolves repo context from argv flags.
func ResolveRepoContext(input gaisdk.ResolveContextInput) (*context.RepoContext, error) {
	parsed := parseRepoContextArgs(input.Command, input.Args)
	return context.ResolveRepo(parsed.repoFlag), nil
}

func withRepoContext(command string, handler func([]string, *context.RepoContext) (string, error)) gaisdk.CommandHandler[*context.RepoContext] {
	return func(args []string, ctx *context.RepoContext) (gaisdk.Renderable, error) {
		parsed := parseRepoContextArgs(command, args)
		return handler(parsed.strippedArgs, ctx)
	}
}

// Commands returns the top-level command registry.
func Commands() map[string]gaisdk.CommandHandler[*context.RepoContext] {
	return map[string]gaisdk.CommandHandler[*context.RepoContext]{
		"issue":    withRepoContext("issue", commands.Issue),
		"pr":       withRepoContext("pr", commands.PR),
		"run":      withRepoContext("run", commands.Run),
		"workflow": withRepoContext("workflow", commands.Workflow),
		"release":  withRepoContext("release", commands.Release),
		"repo":     withRepoContext("repo", commands.Repo),
		"label":    withRepoContext("label", commands.Label),
		"project":  withRepoContext("project", commands.Project),
		"secret":   withRepoContext("secret", commands.Secret),
		"variable": withRepoContext("variable", commands.Variable),
		"search":   withRepoContext("search", commands.Search),
		"api":      withRepoContext("api", commands.Api),
		"setup":    setupCommand,
	}
}

func setupCommand(args []string, _ *context.RepoContext) (gaisdk.Renderable, error) {
	if len(args) != 1 || args[0] != "hooks" {
		return commands.Setup(args, nil)
	}
	gaisdk.InstallSessionStartHooks(gaisdk.HookInstallOptions{
		Marker:      "gai-ghcli",
		BinaryNames: []string{"gai-ghcli"},
	})
	return commands.Setup(args, nil)
}

// GetCommandHelp returns per-command help text.
func GetCommandHelp(command string) (string, bool) {
	help, ok := commandHelp[command]
	return help, ok
}

// HomeHandler wraps the home command for gaisdk.
func HomeHandler(args []string, ctx *context.RepoContext) (gaisdk.Renderable, error) {
	return commands.Home(args, ctx)
}
