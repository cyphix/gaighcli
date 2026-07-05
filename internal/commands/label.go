package commands

import (
	"strconv"
	"strings"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/format"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const LabelHelp = `usage: gai-ghcli label <subcommand> [flags]
subcommands[4]:
  list, create, edit <name>, delete <name>
flags{list}:
  --limit <n> (default 500)
flags{create}:
  --name <text> (required), --color <hex> (required, without #), --description <text>
flags{edit}:
  --name, --color, --description
examples:
  gai-ghcli label list
  gai-ghcli label create --name "priority:high" --color ff0000 --description "High priority"
  gai-ghcli label delete "priority:low"`

var labelListSchema = []toon.FieldDef{
	toon.Field("name", ""),
}

func listLabels(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = "500"
	}
	ghArgs := []string{"label", "list", "--json", "name", "--limit", limit}

	labels, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}

	isEmpty := len(labels) == 0
	limitNum, _ := strconv.Atoi(limit)
	countLine := format.CountLine(format.CountLineOptions{Count: len(labels), Limit: &limitNum})

	listOut, err := toon.RenderList("labels", labels, labelListSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{countLine, listOut}, suggestions.Context{
		Domain: "label", Action: "list", IsEmpty: boolPtr(isEmpty), Repo: ctx,
	})
}

func createLabel(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	name := args.GetFlag(cmdArgs, "--name")
	if name == "" {
		return "", errors.NewGoAIError(
			`--name is required: gai-ghcli label create --name "..." --color "..."`,
			"VALIDATION_ERROR",
		)
	}
	color := args.GetFlag(cmdArgs, "--color")
	if color == "" {
		return "", errors.NewGoAIError(
			`--name is required: gai-ghcli label create --name "..." --color "..."`,
			"VALIDATION_ERROR",
		)
	}

	existing, err := gh.JSON[[]map[string]any](Runner, []string{"label", "list", "--json", "name"}, ctx)
	if err != nil {
		return "", err
	}
	for _, l := range existing {
		lname, _ := l["name"].(string)
		if strings.EqualFold(lname, name) {
			enc, err := encode(map[string]any{"create": "already_exists", "label": lname})
			if err != nil {
				return "", err
			}
			return renderWithHelp([]string{enc}, suggestions.Context{Domain: "label", Action: "create", Repo: ctx})
		}
	}

	ghArgs := []string{"label", "create", name, "--color", color}
	if description := args.GetFlag(cmdArgs, "--description"); description != "" {
		ghArgs = append(ghArgs, "--description", description)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"created": "ok", "label": name})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "label", Action: "create", Repo: ctx})
}

func editLabel(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 1)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Label name is required: gai-ghcli label edit <name>",
			"VALIDATION_ERROR",
		)
	}
	labelName := pos[0]

	ghArgs := []string{"label", "edit", labelName}
	newName := args.GetFlag(cmdArgs, "--name")
	if newName != "" {
		ghArgs = append(ghArgs, "--name", newName)
	}
	if color := args.GetFlag(cmdArgs, "--color"); color != "" {
		ghArgs = append(ghArgs, "--color", color)
	}
	if description := args.GetFlag(cmdArgs, "--description"); description != "" {
		ghArgs = append(ghArgs, "--description", description)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}

	outName := labelName
	if newName != "" {
		outName = newName
	}
	enc, err := encode(map[string]any{"edit": "ok", "label": outName})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "label", Action: "edit", Repo: ctx})
}

func deleteLabel(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 1)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Label name is required: gai-ghcli label delete <name>",
			"VALIDATION_ERROR",
		)
	}
	name := pos[0]

	if _, err := gh.Exec(Runner, []string{"label", "delete", name, "--yes"}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"delete": "ok", "label": name})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "label", Action: "delete", Repo: ctx})
}

// Label handles label subcommands.
func Label(args []string, ctx *context.RepoContext) (string, error) {
	if len(args) == 0 || args[0] == "--help" {
		return LabelHelp, nil
	}
	switch args[0] {
	case "list":
		return listLabels(args, ctx)
	case "create":
		return createLabel(args, ctx)
	case "edit":
		return editLabel(args, ctx)
	case "delete":
		return deleteLabel(args, ctx)
	default:
		return toon.RenderError("Unknown subcommand: "+args[0], "VALIDATION_ERROR",
			"Available subcommands: list, create, edit, delete")
	}
}
