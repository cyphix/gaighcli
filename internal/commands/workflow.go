package commands

import (
	"fmt"
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

const WorkflowHelp = `usage: gai-ghcli workflow <subcommand> [flags]
subcommands[5]:
  list, view <id|name>, run <id|name>, enable <id|name>, disable <id|name>
flags{list}:
  --limit <n> (default 20), --all
flags{run}:
  --ref <git-ref>, --field <key=val> (repeatable)
examples:
  gai-ghcli workflow list
  gai-ghcli workflow run ci.yml --ref main
  gai-ghcli workflow disable 12345`

var (
	workflowListSchema = []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("name", ""),
		toon.Lower("state", ""),
		toon.Field("path", ""),
	}
	workflowViewSchema = []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("name", ""),
		toon.Lower("state", ""),
		toon.Field("path", ""),
	}
)

// Workflow handles workflow subcommands.
func Workflow(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	if len(cmdArgs) == 0 || cmdArgs[0] == "--help" {
		return WorkflowHelp, nil
	}
	sub := cmdArgs[0]
	switch sub {
	case "list":
		return listWorkflows(cmdArgs, ctx)
	case "view":
		return viewWorkflow(cmdArgs, ctx)
	case "run":
		return runWorkflow(cmdArgs, ctx)
	case "enable":
		return enableWorkflow(cmdArgs, ctx)
	case "disable":
		return disableWorkflow(cmdArgs, ctx)
	default:
		return toon.RenderError("Unknown subcommand: "+sub, "VALIDATION_ERROR",
			"Available subcommands: list, view, run, enable, disable")
	}
}

func workflowIDString(v any) string {
	if v == nil {
		return ""
	}
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case string:
		return n
	default:
		return fmt.Sprint(v)
	}
}

func findWorkflow(id string, ctx *context.RepoContext) (map[string]any, error) {
	workflows, err := gh.JSON[[]map[string]any](Runner,
		[]string{"workflow", "list", "--json", "id,name,state,path", "--all"}, ctx)
	if err != nil {
		return nil, err
	}
	for _, w := range workflows {
		wid := workflowIDString(w["id"])
		name, _ := w["name"].(string)
		if wid == id || name == id || strings.EqualFold(name, id) {
			return w, nil
		}
	}
	return nil, errors.NewGoAIError(`Workflow "`+id+`" not found`, "NOT_FOUND")
}

func listWorkflows(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = "20"
	}
	ghArgs := []string{"workflow", "list", "--json", "id,name,state,path", "--limit", limit}
	if args.HasFlag(cmdArgs, "--all") {
		ghArgs = append(ghArgs, "--all")
	}
	workflows, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	isEmpty := len(workflows) == 0
	limitNum, _ := strconv.Atoi(limit)
	countLine := format.CountLine(format.CountLineOptions{Count: len(workflows), Limit: &limitNum})
	list, err := toon.RenderList("workflows", workflows, workflowListSchema)
	if err != nil {
		return "", err
	}
	isEmptyVal := isEmpty
	return renderWithHelp([]string{countLine, list}, suggestions.Context{
		Domain: "workflow", Action: "list", IsEmpty: &isEmptyVal, Repo: ctx,
	})
}

func viewWorkflow(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError(
			"Workflow ID or name is required: gai-ghcli workflow view <id|name>",
			"VALIDATION_ERROR",
		)
	}
	match, err := findWorkflow(pos[1], ctx)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("workflow", match, workflowViewSchema)
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(detail), nil
}

func runWorkflow(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError(
			"Workflow ID or name is required: gai-ghcli workflow run <id|name>",
			"VALIDATION_ERROR",
		)
	}
	id := pos[1]
	ghArgs := []string{"workflow", "run", id}
	if ref := args.GetFlag(cmdArgs, "--ref"); ref != "" {
		ghArgs = append(ghArgs, "--ref", ref)
	}
	for _, f := range args.GetAllFlags(cmdArgs, "--field") {
		ghArgs = append(ghArgs, "--field", f)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"triggered": "ok", "workflow": id})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "workflow", Action: "run", ID: id, Repo: ctx})
}

func enableWorkflow(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError(
			"Workflow ID or name is required: gai-ghcli workflow enable <id|name>",
			"VALIDATION_ERROR",
		)
	}
	id := pos[1]
	match, err := findWorkflow(id, ctx)
	if err != nil {
		return "", err
	}
	if state, _ := match["state"].(string); state == "active" {
		enc, err := encode(map[string]any{"enable": "already_enabled", "workflow": id})
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{enc}, suggestions.Context{Domain: "workflow", Action: "enable", ID: id, Repo: ctx})
	}
	if _, err := gh.Exec(Runner, []string{"workflow", "enable", id}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"enable": "ok", "workflow": id})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "workflow", Action: "enable", ID: id, Repo: ctx})
}

func disableWorkflow(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError(
			"Workflow ID or name is required: gai-ghcli workflow disable <id|name>",
			"VALIDATION_ERROR",
		)
	}
	id := pos[1]
	match, err := findWorkflow(id, ctx)
	if err != nil {
		return "", err
	}
	if state, _ := match["state"].(string); state == "disabled_manually" {
		enc, err := encode(map[string]any{"disable": "already_disabled", "workflow": id})
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{enc}, suggestions.Context{Domain: "workflow", Action: "disable", ID: id, Repo: ctx})
	}
	if _, err := gh.Exec(Runner, []string{"workflow", "disable", id}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"disable": "ok", "workflow": id})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "workflow", Action: "disable", ID: id, Repo: ctx})
}
