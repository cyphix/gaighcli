package commands

import (
	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/secretvalue"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const VariableHelp = `usage: gai-ghcli variable <subcommand> [flags]
subcommands[3]:
  list, set <name>, delete <name>
flags{set}:
  --body/-b <value> (reads from stdin if omitted)
examples:
  gai-ghcli variable list
  gai-ghcli variable set NODE_ENV --body production
  echo -n "production" | gai-ghcli variable set NODE_ENV
  gai-ghcli variable delete NODE_ENV`

var variableListSchema = []toon.FieldDef{
	toon.Field("name", ""),
	toon.Field("value", ""),
	toon.RelativeTime("updatedAt", "updated"),
}

func listVariables(_ []string, ctx *context.RepoContext) (string, error) {
	variables, err := gh.JSON[[]map[string]any](Runner, []string{"variable", "list", "--json", "name,value,updatedAt"}, ctx)
	if err != nil {
		return "", err
	}
	isEmpty := len(variables) == 0
	listOut, err := toon.RenderList("variables", variables, variableListSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp(
		[]string{formatCount(variables), listOut},
		suggestions.Context{Domain: "variable", Action: "list", IsEmpty: boolPtr(isEmpty), Repo: ctx},
	)
}

func setVariable(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	remaining := cmdArgs[1:]
	flagValue := args.TakeFlag(&remaining, "--body")
	if flagValue == "" {
		flagValue = args.TakeFlag(&remaining, "-b")
	}
	pos := positionals(remaining, 0)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Variable name is required: gai-ghcli variable set <name>",
			"VALIDATION_ERROR",
		)
	}
	name := pos[0]

	value, err := secretvalue.ResolveValue(flagValue, secretvalue.NounVariable)
	if err != nil {
		return "", err
	}
	if _, err := gh.ExecWithStdin(Runner, []string{"variable", "set", name}, value, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"set": "ok", "variable": name})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{
		Domain: "variable", Action: "set", ID: name, Repo: ctx,
	})
}

func deleteVariable(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 1)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Variable name is required: gai-ghcli variable delete <name>",
			"VALIDATION_ERROR",
		)
	}
	name := pos[0]

	if _, err := gh.Exec(Runner, []string{"variable", "delete", name}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"delete": "ok", "variable": name})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{
		Domain: "variable", Action: "delete", ID: name, Repo: ctx,
	})
}

// Variable handles variable subcommands.
func Variable(args []string, ctx *context.RepoContext) (string, error) {
	if len(args) == 0 || args[0] == "--help" {
		return VariableHelp, nil
	}
	switch args[0] {
	case "list":
		return listVariables(args, ctx)
	case "set":
		return setVariable(args, ctx)
	case "delete":
		return deleteVariable(args, ctx)
	default:
		return toon.RenderError("Unknown subcommand: "+args[0], "VALIDATION_ERROR",
			"Available subcommands: list, set, delete")
	}
}
