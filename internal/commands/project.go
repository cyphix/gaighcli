package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/body"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/format"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const ProjectHelp = `usage: gai-ghcli project <subcommand> [flags]
subcommands[20]:
  list, view <number>, create, edit <number>, close <number>, delete <number>, copy <number>,
  mark-template <number>, link <number>, unlink <number>,
  item-list <number>, item-add <number>, item-create <number>, item-edit, item-set-status <number>, item-delete <number>, item-archive <number>,
  field-list <number>, field-create <number>, field-delete
flags{common}:
  --owner <login> (default: current repo owner or @me)
flags{list}:
  --closed, --limit <n> (default 30)
flags{view}:
  (none)
flags{create}:
  --title <text> (required)
flags{edit}:
  --title, --description, --readme, --visibility <PUBLIC|PRIVATE>
flags{close}:
  --undo
flags{copy}:
  --title, --source-owner, --target-owner, --drafts
flags{mark-template}:
  --undo
flags{link,unlink}:
  --repo <owner/name>, --team <slug>
flags{item-list}:
  --limit <n> (default 30), --query <filter>
flags{item-add}:
  --url <issue-or-pr-url> (required)
flags{item-create}:
  --title <text> (required), --body <text> or --body-file <path>
flags{item-edit}:
  --id <item-id> (required), --project-id, --field-id, --text, --number, --date, --single-select-option-id, --iteration-id, --clear, --body
flags{item-set-status}:
  --issue <n> (required unless --title set), --title <text> (required unless --issue set), --status <name> (required, case-sensitive),
  --owner, --repo, --status-field (default Status), --config, --limit <n> (default 500)
flags{item-delete,item-archive}:
  --id <item-id> (required), --undo (item-archive only)
flags{field-list}:
  --limit <n> (default 30)
flags{field-create}:
  --name <text> (required), --data-type <TEXT|SINGLE_SELECT|DATE|NUMBER>, --single-select-options <a,b,c>
flags{field-delete}:
  --id <field-id> (required)
examples:
  gai-ghcli project list --owner myorg
  gai-ghcli project view 1 --owner @me
  gai-ghcli project create --title "Roadmap" --owner myorg
  gai-ghcli project item-list 1 --query "status:Ready"
  gai-ghcli project item-add 1 --url https://github.com/owner/repo/issues/42
  gai-ghcli project item-set-status 3 --issue 42 --status Ready
  gai-ghcli project item-set-status 3 --title "Fix login bug" --status "In progress"
  gai-ghcli project link 1 --repo owner/repo
notes:
  Requires gh token scope "project" — run gh auth refresh -s project if auth fails`

var (
	projectListSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Pluck("owner", "login", "owner"),
		toon.Custom("closed", projectBoolField("closed")),
		toon.Custom("items", projectNestedCount("items")),
		toon.Field("url", ""),
	}

	projectViewSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Pluck("owner", "login", "owner"),
		toon.Custom("closed", projectBoolField("closed")),
		toon.Custom("public", projectBoolField("public")),
		toon.Custom("items", projectNestedCount("items")),
		toon.Custom("fields", projectNestedCount("fields")),
		toon.Field("shortDescription", "description"),
		toon.Field("url", ""),
	}

	projectItemListSchema = []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("title", ""),
		toon.Field("status", ""),
		toon.Custom("type", projectContentField("type", "draft")),
		toon.Custom("number", projectContentNumber()),
		toon.Custom("repo", projectContentField("repository", "")),
		toon.JoinArray("labels", "name", "labels", "none"),
	}

	projectFieldListSchema = []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("name", ""),
		toon.Custom("type", projectFieldType()),
		toon.Custom("options", projectFieldOptions()),
	}

	projectMutationSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Field("url", ""),
	}
)

func projectBoolField(key string) func(item map[string]any) any {
	return func(item map[string]any) any {
		if b, ok := item[key].(bool); ok && b {
			return "yes"
		}
		return "no"
	}
}

func projectNestedCount(key string) func(item map[string]any) any {
	return func(item map[string]any) any {
		if nested, ok := item[key].(map[string]any); ok {
			switch tc := nested["totalCount"].(type) {
			case float64:
				return int(tc)
			case int:
				return tc
			}
		}
		return 0
	}
}

func projectContentField(key, fallback string) func(item map[string]any) any {
	return func(item map[string]any) any {
		content, _ := item["content"].(map[string]any)
		if content == nil {
			return fallback
		}
		if v, ok := content[key].(string); ok && v != "" {
			return v
		}
		if v, ok := content[key].(float64); ok {
			return int(v)
		}
		return fallback
	}
}

func projectContentNumber() func(item map[string]any) any {
	return func(item map[string]any) any {
		content, _ := item["content"].(map[string]any)
		if content == nil {
			return nil
		}
		switch n := content["number"].(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
		return nil
	}
}

func projectFieldType() func(item map[string]any) any {
	return func(item map[string]any) any {
		raw, _ := item["type"].(string)
		return strings.TrimPrefix(raw, "ProjectV2")
	}
}

func projectFieldOptions() func(item map[string]any) any {
	return func(item map[string]any) any {
		opts, _ := item["options"].([]any)
		if len(opts) == 0 {
			return "none"
		}
		names := make([]string, 0, len(opts))
		for _, o := range opts {
			m, _ := o.(map[string]any)
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			return "none"
		}
		return strings.Join(names, ",")
	}
}

func resolveProjectOwner(work []string, ctx *context.RepoContext) string {
	if owner := args.GetFlag(work, "--owner"); owner != "" {
		return owner
	}
	if ctx != nil && ctx.Owner != "" {
		return ctx.Owner
	}
	return ""
}

func appendOwnerFlag(ghArgs []string, owner string) []string {
	if owner != "" {
		return append(ghArgs, "--owner", owner)
	}
	return ghArgs
}

func appendOptionalFlag(ghArgs []string, flag, value string) []string {
	if value != "" {
		return append(ghArgs, flag, value)
	}
	return ghArgs
}

func forwardProjectFlags(ghArgs, work []string, flags ...string) []string {
	for _, flag := range flags {
		if value := args.GetFlag(work, flag); value != "" {
			ghArgs = append(ghArgs, flag, value)
		}
	}
	return ghArgs
}

func resolveLinkRepo(work []string, ctx *context.RepoContext) string {
	if repo := args.GetFlag(work, "--repo"); repo != "" {
		return repo
	}
	if repo := args.GetFlag(work, "-R"); repo != "" {
		return repo
	}
	if ctx != nil {
		return ctx.NWO
	}
	return ""
}

func requireProjectNumber(a []string, pos int) (int, error) {
	return args.RequireNumber(args.GetPositional(a, pos), "project")
}

func renderProjectMutation(item map[string]any, action string, id string, ctx *context.RepoContext) (string, error) {
	if item == nil {
		item = map[string]any{}
	}
	if _, ok := item["number"]; !ok && id != "" {
		item["number"] = id
	}
	detail, err := toon.RenderDetail("project", item, projectMutationSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "project", Action: action, ID: id, Repo: ctx,
	})
}

func renderProjectStatus(action string, fields map[string]any, ctx *context.RepoContext) (string, error) {
	enc, err := encode(fields)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{
		Domain: "project", Action: action, Repo: ctx,
	})
}

func listProjects(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a[1:]...)
	owner := resolveProjectOwner(work, ctx)
	limit := args.GetFlag(work, "--limit")
	if limit == "" {
		limit = "30"
	}

	ghArgs := []string{"project", "list", "--format", "json", "--limit", limit}
	ghArgs = appendOwnerFlag(ghArgs, owner)
	if args.HasFlag(work, "--closed") {
		ghArgs = append(ghArgs, "--closed")
	}

	resp, err := gh.JSON[struct {
		Projects   []map[string]any `json:"projects"`
		TotalCount int              `json:"totalCount"`
	}](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}

	limitNum, _ := strconv.Atoi(limit)
	total := resp.TotalCount
	countLine := format.CountLine(format.CountLineOptions{
		Count: len(resp.Projects), Limit: &limitNum, TotalCount: &total,
	})
	listOut, err := toon.RenderList("projects", resp.Projects, projectListSchema)
	if err != nil {
		return "", err
	}
	isEmpty := len(resp.Projects) == 0
	return renderWithHelp([]string{countLine, listOut}, suggestions.Context{
		Domain: "project", Action: "list", IsEmpty: boolPtr(isEmpty), Repo: ctx,
	})
}

func viewProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)

	ghArgs := appendOwnerFlag([]string{
		"project", "view", strconv.Itoa(num), "--format", "json",
	}, owner)

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("project", item, projectViewSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "project", Action: "view", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func createProject(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a[1:]...)
	title := args.GetFlag(work, "--title")
	if title == "" {
		return "", errors.NewGoAIError("--title is required", "VALIDATION_ERROR")
	}
	owner := resolveProjectOwner(work, ctx)

	ghArgs := appendOwnerFlag([]string{
		"project", "create", "--format", "json", "--title", title,
	}, owner)

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	id := ""
	if n, ok := item["number"].(float64); ok {
		id = strconv.Itoa(int(n))
	}
	return renderProjectMutation(item, "create", id, ctx)
}

func editProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)

	ghArgs := appendOwnerFlag([]string{
		"project", "edit", strconv.Itoa(num), "--format", "json",
	}, owner)
	ghArgs = forwardProjectFlags(ghArgs, work, "--title", "--description", "--readme", "--visibility")

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	return renderProjectMutation(item, "edit", strconv.Itoa(num), ctx)
}

func closeProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	undo := args.HasFlag(work, "--undo")

	if !undo {
		current, err := gh.JSON[map[string]any](Runner, appendOwnerFlag([]string{
			"project", "view", strconv.Itoa(num), "--format", "json",
		}, owner), nil)
		if err != nil {
			return "", err
		}
		if closed, _ := current["closed"].(bool); closed {
			return renderProjectStatus("close", map[string]any{
				"number":  num,
				"closed":  "yes",
				"message": "Already closed",
			}, ctx)
		}
	} else {
		current, err := gh.JSON[map[string]any](Runner, appendOwnerFlag([]string{
			"project", "view", strconv.Itoa(num), "--format", "json",
		}, owner), nil)
		if err != nil {
			return "", err
		}
		if closed, _ := current["closed"].(bool); !closed {
			return renderProjectStatus("close", map[string]any{
				"number":  num,
				"closed":  "no",
				"message": "Already open",
			}, ctx)
		}
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "close", strconv.Itoa(num), "--format", "json",
	}, owner)
	if undo {
		ghArgs = append(ghArgs, "--undo")
	}

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	return renderProjectMutation(item, "close", strconv.Itoa(num), ctx)
}

func deleteProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)

	ghArgs := appendOwnerFlag([]string{
		"project", "delete", strconv.Itoa(num), "--format", "json",
	}, owner)

	if _, err := gh.JSON[map[string]any](Runner, ghArgs, nil); err != nil {
		return "", err
	}
	return renderProjectStatus("delete", map[string]any{
		"number": num,
		"status": "deleted",
	}, ctx)
}

func copyProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)

	ghArgs := appendOwnerFlag([]string{
		"project", "copy", strconv.Itoa(num), "--format", "json",
	}, owner)
	ghArgs = forwardProjectFlags(ghArgs, work, "--title", "--source-owner", "--target-owner")
	if args.HasFlag(work, "--drafts") {
		ghArgs = append(ghArgs, "--drafts")
	}

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	id := strconv.Itoa(num)
	if n, ok := item["number"].(float64); ok {
		id = strconv.Itoa(int(n))
	}
	return renderProjectMutation(item, "copy", id, ctx)
}

func markTemplateProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)

	ghArgs := appendOwnerFlag([]string{
		"project", "mark-template", strconv.Itoa(num), "--format", "json",
	}, owner)
	if args.HasFlag(work, "--undo") {
		ghArgs = append(ghArgs, "--undo")
	}

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	return renderProjectMutation(item, "mark-template", strconv.Itoa(num), ctx)
}

func linkProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	repo := resolveLinkRepo(work, ctx)
	team := args.GetFlag(work, "--team")

	ghArgs := appendOwnerFlag([]string{"project", "link", strconv.Itoa(num)}, owner)
	ghArgs = appendOptionalFlag(ghArgs, "--repo", repo)
	ghArgs = appendOptionalFlag(ghArgs, "--team", team)

	if _, err := gh.Exec(Runner, ghArgs, nil); err != nil {
		return "", err
	}
	status := map[string]any{"number": num, "link": "ok"}
	if repo != "" {
		status["repo"] = repo
	}
	if team != "" {
		status["team"] = team
	}
	return renderProjectStatus("link", status, ctx)
}

func unlinkProject(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	repo := resolveLinkRepo(work, ctx)
	team := args.GetFlag(work, "--team")

	ghArgs := appendOwnerFlag([]string{"project", "unlink", strconv.Itoa(num)}, owner)
	ghArgs = appendOptionalFlag(ghArgs, "--repo", repo)
	ghArgs = appendOptionalFlag(ghArgs, "--team", team)

	if _, err := gh.Exec(Runner, ghArgs, nil); err != nil {
		return "", err
	}
	status := map[string]any{"number": num, "unlink": "ok"}
	if repo != "" {
		status["repo"] = repo
	}
	if team != "" {
		status["team"] = team
	}
	return renderProjectStatus("unlink", status, ctx)
}

func listProjectItems(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	limit := args.GetFlag(work, "--limit")
	if limit == "" {
		limit = "30"
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "item-list", strconv.Itoa(num), "--format", "json", "--limit", limit,
	}, owner)
	ghArgs = forwardProjectFlags(ghArgs, work, "--query")

	resp, err := gh.JSON[struct {
		Items      []map[string]any `json:"items"`
		TotalCount int              `json:"totalCount"`
	}](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}

	limitNum, _ := strconv.Atoi(limit)
	total := resp.TotalCount
	countLine := format.CountLine(format.CountLineOptions{
		Count: len(resp.Items), Limit: &limitNum, TotalCount: &total,
	})
	listOut, err := toon.RenderList("items", resp.Items, projectItemListSchema)
	if err != nil {
		return "", err
	}
	isEmpty := len(resp.Items) == 0
	return renderWithHelp([]string{countLine, listOut}, suggestions.Context{
		Domain: "project", Action: "item-list", ID: strconv.Itoa(num), IsEmpty: boolPtr(isEmpty), Repo: ctx,
	})
}

func addProjectItem(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	url := args.GetFlag(work, "--url")
	if url == "" {
		return "", errors.NewGoAIError("--url is required", "VALIDATION_ERROR")
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "item-add", strconv.Itoa(num), "--format", "json", "--url", url,
	}, owner)

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("item", item, []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("title", ""),
	})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "project", Action: "item-add", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func createProjectItem(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	title := args.GetFlag(work, "--title")
	if title == "" {
		return "", errors.NewGoAIError("--title is required", "VALIDATION_ERROR")
	}
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "item-create", strconv.Itoa(num), "--format", "json", "--title", title,
	}, owner)
	if bodyText != "" {
		ghArgs = append(ghArgs, "--body", bodyText)
	}

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("item", item, []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("title", ""),
	})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "project", Action: "item-create", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func editProjectItem(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a[1:]...)
	itemID := args.GetFlag(work, "--id")
	if itemID == "" {
		return "", errors.NewGoAIError("--id is required", "VALIDATION_ERROR")
	}
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}

	ghArgs := []string{"project", "item-edit", "--format", "json", "--id", itemID}
	ghArgs = forwardProjectFlags(ghArgs, work,
		"--project-id", "--field-id", "--text", "--number", "--date",
		"--single-select-option-id", "--iteration-id",
	)
	if args.HasFlag(work, "--clear") {
		ghArgs = append(ghArgs, "--clear")
	}
	if bodyText != "" {
		ghArgs = append(ghArgs, "--body", bodyText)
	}

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("item", item, []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("title", ""),
	})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "project", Action: "item-edit", ID: itemID, Repo: ctx,
	})
}

func deleteProjectItem(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	itemID := args.GetFlag(work, "--id")
	if itemID == "" {
		return "", errors.NewGoAIError("--id is required", "VALIDATION_ERROR")
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "item-delete", strconv.Itoa(num), "--format", "json", "--id", itemID,
	}, owner)

	if _, err := gh.JSON[map[string]any](Runner, ghArgs, nil); err != nil {
		return "", err
	}
	return renderProjectStatus("item-delete", map[string]any{
		"project": num,
		"id":      itemID,
		"status":  "deleted",
	}, ctx)
}

func archiveProjectItem(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	itemID := args.GetFlag(work, "--id")
	if itemID == "" {
		return "", errors.NewGoAIError("--id is required", "VALIDATION_ERROR")
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "item-archive", strconv.Itoa(num), "--format", "json", "--id", itemID,
	}, owner)
	if args.HasFlag(work, "--undo") {
		ghArgs = append(ghArgs, "--undo")
	}

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("item", item, []toon.FieldDef{toon.Field("id", "")})
	if err != nil {
		return "", err
	}
	action := "item-archive"
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "project", Action: action, ID: itemID, Repo: ctx,
	})
}

func listProjectFields(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	limit := args.GetFlag(work, "--limit")
	if limit == "" {
		limit = "30"
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "field-list", strconv.Itoa(num), "--format", "json", "--limit", limit,
	}, owner)

	resp, err := gh.JSON[struct {
		Fields     []map[string]any `json:"fields"`
		TotalCount int              `json:"totalCount"`
	}](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}

	limitNum, _ := strconv.Atoi(limit)
	total := resp.TotalCount
	countLine := format.CountLine(format.CountLineOptions{
		Count: len(resp.Fields), Limit: &limitNum, TotalCount: &total,
	})
	listOut, err := toon.RenderList("fields", resp.Fields, projectFieldListSchema)
	if err != nil {
		return "", err
	}
	isEmpty := len(resp.Fields) == 0
	return renderWithHelp([]string{countLine, listOut}, suggestions.Context{
		Domain: "project", Action: "field-list", ID: strconv.Itoa(num), IsEmpty: boolPtr(isEmpty), Repo: ctx,
	})
}

func createProjectField(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)
	owner := resolveProjectOwner(work, ctx)
	name := args.GetFlag(work, "--name")
	if name == "" {
		return "", errors.NewGoAIError("--name is required", "VALIDATION_ERROR")
	}

	ghArgs := appendOwnerFlag([]string{
		"project", "field-create", strconv.Itoa(num), "--format", "json", "--name", name,
	}, owner)
	ghArgs = forwardProjectFlags(ghArgs, work, "--data-type", "--single-select-options")

	item, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("field", item, []toon.FieldDef{
		toon.Field("id", ""),
		toon.Field("name", ""),
		toon.Custom("type", projectFieldType()),
	})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "project", Action: "field-create", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func deleteProjectField(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a[1:]...)
	fieldID := args.GetFlag(work, "--id")
	if fieldID == "" {
		return "", errors.NewGoAIError("--id is required", "VALIDATION_ERROR")
	}

	ghArgs := []string{"project", "field-delete", "--format", "json", "--id", fieldID}
	if _, err := gh.JSON[map[string]any](Runner, ghArgs, nil); err != nil {
		return "", err
	}
	return renderProjectStatus("field-delete", map[string]any{
		"id":     fieldID,
		"status": "deleted",
	}, ctx)
}

type issueBoardConfig struct {
	ProjectNumber int               `json:"projectNumber"`
	ProjectID     string            `json:"projectId"`
	StatusFieldID string            `json:"statusFieldId"`
	StatusOptions map[string]string `json:"statusOptions"`
}

func gitRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func resolveIssueBoardConfigPath(work []string) string {
	if path := args.GetFlag(work, "--config"); path != "" {
		return path
	}
	root := gitRoot()
	if root == "" {
		return ""
	}
	candidate := filepath.Join(root, ".github", "issue-board.json")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func loadIssueBoardConfig(path string) (*issueBoardConfig, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.NewGoAIError("Failed to read config: "+path, "VALIDATION_ERROR")
	}
	var cfg issueBoardConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, errors.NewGoAIError("Invalid issue-board config JSON: "+err.Error(), "VALIDATION_ERROR")
	}
	return &cfg, nil
}

func validateIssueBoardConfig(cfg *issueBoardConfig, projectNum int) error {
	if cfg == nil || cfg.ProjectNumber == 0 {
		return nil
	}
	if cfg.ProjectNumber != projectNum {
		return errors.NewGoAIError(
			fmt.Sprintf("config projectNumber %d does not match argument %d", cfg.ProjectNumber, projectNum),
			"VALIDATION_ERROR",
		)
	}
	return nil
}

func resolveIssueBoardOwner(work []string, ctx *context.RepoContext) string {
	if o := args.GetFlag(work, "--owner"); o != "" {
		return o
	}
	if o := os.Getenv("ISSUE_BOARD_OWNER"); o != "" {
		return o
	}
	return resolveProjectOwner(work, ctx)
}

func projectContentNumberValue(item map[string]any) (int, bool) {
	content, _ := item["content"].(map[string]any)
	if content == nil {
		return 0, false
	}
	switch n := content["number"].(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func itemMatchesRepo(item map[string]any, repo string) bool {
	if repo == "" {
		return true
	}
	content, _ := item["content"].(map[string]any)
	if content == nil {
		return false
	}
	if r, ok := content["repository"].(string); ok && r != "" {
		return r == repo
	}
	if url, ok := content["url"].(string); ok {
		if n, ok := projectContentNumberValue(item); ok {
			if strings.HasSuffix(url, fmt.Sprintf("/issues/%d", n)) ||
				strings.HasSuffix(url, fmt.Sprintf("/pull/%d", n)) {
				return strings.Contains(url, "/"+repo+"/")
			}
		}
	}
	return false
}

func findProjectItemByIssue(items []map[string]any, issueNum int, repo string) (map[string]any, bool) {
	for _, item := range items {
		n, ok := projectContentNumberValue(item)
		if !ok || n != issueNum {
			continue
		}
		if itemMatchesRepo(item, repo) {
			return item, true
		}
	}
	return nil, false
}

func findProjectItemByTitle(items []map[string]any, title, repo string) (map[string]any, []map[string]any, bool) {
	want := strings.TrimSpace(title)
	var matches []map[string]any
	for _, item := range items {
		if !itemMatchesRepo(item, repo) {
			continue
		}
		itemTitle, _ := item["title"].(string)
		if strings.EqualFold(strings.TrimSpace(itemTitle), want) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return nil, nil, false
	case 1:
		return matches[0], nil, true
	default:
		return nil, matches, false
	}
}

func projectItemTitle(item map[string]any) string {
	title, _ := item["title"].(string)
	return strings.TrimSpace(title)
}

func fetchProjectItems(projectNum int, owner, limit string) ([]map[string]any, error) {
	resp, err := gh.JSON[struct {
		Items []map[string]any `json:"items"`
	}](Runner, appendOwnerFlag([]string{
		"project", "item-list", strconv.Itoa(projectNum), "--format", "json", "--limit", limit,
	}, owner), nil)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func resolveProjectIDFromGH(projectNum int, owner string) (string, error) {
	item, err := gh.JSON[map[string]any](Runner, appendOwnerFlag([]string{
		"project", "view", strconv.Itoa(projectNum), "--format", "json",
	}, owner), nil)
	if err != nil {
		return "", err
	}
	id, _ := item["id"].(string)
	if id == "" {
		return "", errors.NewGoAIError("Unable to resolve project ID", "UNKNOWN")
	}
	return id, nil
}

type statusFieldResolution struct {
	FieldID   string
	OptionID  string
	OptionNames []string
}

func resolveStatusField(projectNum int, owner, statusField, statusName string, cfg *issueBoardConfig) (statusFieldResolution, error) {
	var res statusFieldResolution

	if cfg != nil {
		if cfg.ProjectID != "" {
			// project ID resolved separately
		}
		if cfg.StatusFieldID != "" {
			res.FieldID = cfg.StatusFieldID
		}
		if cfg.StatusOptions != nil {
			if optID, ok := cfg.StatusOptions[statusName]; ok && optID != "" {
				res.OptionID = optID
			}
		}
	}

	if res.FieldID != "" && res.OptionID != "" {
		return res, nil
	}

	resp, err := gh.JSON[struct {
		Fields []map[string]any `json:"fields"`
	}](Runner, appendOwnerFlag([]string{
		"project", "field-list", strconv.Itoa(projectNum), "--format", "json", "--limit", "100",
	}, owner), nil)
	if err != nil {
		return res, err
	}

	for _, field := range resp.Fields {
		name, _ := field["name"].(string)
		if name != statusField {
			continue
		}
		if res.FieldID == "" {
			res.FieldID, _ = field["id"].(string)
		}
		opts, _ := field["options"].([]any)
		for _, o := range opts {
			m, _ := o.(map[string]any)
			optName, _ := m["name"].(string)
			res.OptionNames = append(res.OptionNames, optName)
			if optName == statusName {
				if id, ok := m["id"].(string); ok {
					res.OptionID = id
				}
			}
		}
		break
	}

	if res.FieldID == "" {
		return res, errors.NewGoAIError(
			fmt.Sprintf("Status field %q not found on project %d", statusField, projectNum),
			"NOT_FOUND",
			fmt.Sprintf("Run `gai-ghcli project field-list %d` to see fields", projectNum),
		)
	}

	if res.OptionID == "" && cfg != nil && cfg.StatusOptions != nil {
		if optID, ok := cfg.StatusOptions[statusName]; ok && optID != "" {
			for _, field := range resp.Fields {
				name, _ := field["name"].(string)
				if name != statusField {
					continue
				}
				opts, _ := field["options"].([]any)
				for _, o := range opts {
					m, _ := o.(map[string]any)
					id, _ := m["id"].(string)
					if id == optID || strings.HasSuffix(id, optID) {
						res.OptionID = id
						break
					}
				}
				break
			}
			if res.OptionID == "" {
				res.OptionID = optID
			}
		}
	}

	if res.OptionID == "" {
		msg := fmt.Sprintf("Unknown status %q for field %s", statusName, statusField)
		if len(res.OptionNames) > 0 {
			return res, errors.NewGoAIError(
				msg+": available "+strings.Join(res.OptionNames, ", "),
				"VALIDATION_ERROR",
			)
		}
		return res, errors.NewGoAIError(msg, "VALIDATION_ERROR")
	}

	return res, nil
}

func renderItemSetStatusResult(repo, targetStatus, previousStatus string, item map[string]any, changed bool) map[string]any {
	fields := map[string]any{
		"repo":           repo,
		"previousStatus": previousStatus,
		"status":         targetStatus,
	}
	if n, ok := projectContentNumberValue(item); ok {
		fields["issue"] = n
	}
	if title := projectItemTitle(item); title != "" {
		fields["title"] = title
	}
	if changed {
		fields["changed"] = "yes"
	} else {
		fields["changed"] = "no"
	}
	return fields
}

func renderItemSetStatusOutput(projectNum int, fields map[string]any, ctx *context.RepoContext) (string, error) {
	enc, err := encode(fields)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{
		Domain: "project", Action: "item-set-status", ID: strconv.Itoa(projectNum), Repo: ctx,
	})
}

func setProjectItemStatus(a []string, ctx *context.RepoContext) (string, error) {
	num, err := requireProjectNumber(a, 1)
	if err != nil {
		return "", err
	}
	work := append([]string{}, a[2:]...)

	issueRaw := args.GetFlag(work, "--issue")
	titleRaw := args.GetFlag(work, "--title")
	targetStatus := args.GetFlag(work, "--status")

	switch {
	case issueRaw == "" && titleRaw == "":
		return "", errors.NewGoAIError("--issue or --title is required", "VALIDATION_ERROR")
	case issueRaw != "" && titleRaw != "":
		return "", errors.NewGoAIError("Use --issue or --title, not both", "VALIDATION_ERROR")
	case targetStatus == "":
		return "", errors.NewGoAIError("--status is required", "VALIDATION_ERROR")
	}

	var issueNum int
	if issueRaw != "" {
		issueNum, err = args.RequireNumber(issueRaw, "issue")
		if err != nil {
			return "", err
		}
	}

	repo := resolveLinkRepo(work, ctx)
	if repo == "" {
		return "", errors.NewGoAIError("--repo is required (or run from a git repo)", "VALIDATION_ERROR")
	}

	owner := resolveIssueBoardOwner(work, ctx)
	statusField := args.GetFlag(work, "--status-field")
	if statusField == "" {
		statusField = "Status"
	}
	limit := args.GetFlag(work, "--limit")
	if limit == "" {
		limit = "500"
	}

	configPath := resolveIssueBoardConfigPath(work)
	cfg, err := loadIssueBoardConfig(configPath)
	if err != nil {
		return "", err
	}
	if err := validateIssueBoardConfig(cfg, num); err != nil {
		return "", err
	}

	items, err := fetchProjectItems(num, owner, limit)
	if err != nil {
		return "", err
	}

	var item map[string]any
	if issueRaw != "" {
		var found bool
		item, found = findProjectItemByIssue(items, issueNum, repo)
		if !found {
			return "", errors.NewGoAIError(
				fmt.Sprintf("Issue #%d not found on project %d", issueNum, num),
				"NOT_FOUND",
				fmt.Sprintf("gai-ghcli project item-add %d --url https://github.com/%s/issues/%d", num, repo, issueNum),
			)
		}
	} else {
		var ambiguous []map[string]any
		var found bool
		item, ambiguous, found = findProjectItemByTitle(items, titleRaw, repo)
		if !found {
			if len(ambiguous) > 0 {
				var parts []string
				for _, m := range ambiguous {
					n, _ := projectContentNumberValue(m)
					parts = append(parts, fmt.Sprintf("#%d %q", n, projectItemTitle(m)))
				}
				return "", errors.NewGoAIError(
					fmt.Sprintf("Multiple items match title %q: %s", titleRaw, strings.Join(parts, ", ")),
					"AMBIGUOUS_MATCH",
					"Re-run with --issue <n>",
				)
			}
			return "", errors.NewGoAIError(
				fmt.Sprintf("No project item titled %q in %s on project %d", titleRaw, repo, num),
				"NOT_FOUND",
				fmt.Sprintf("gai-ghcli project item-list %d", num),
			)
		}
	}

	itemID, _ := item["id"].(string)
	if itemID == "" {
		return "", errors.NewGoAIError("Matched project item has no ID", "UNKNOWN")
	}

	previousStatus, _ := item["status"].(string)
	if previousStatus == targetStatus {
		return renderItemSetStatusOutput(num,
			renderItemSetStatusResult(repo, targetStatus, previousStatus, item, false), ctx)
	}

	projectID := ""
	if cfg != nil {
		projectID = cfg.ProjectID
	}
	if projectID == "" {
		projectID, err = resolveProjectIDFromGH(num, owner)
		if err != nil {
			return "", err
		}
	}

	fieldRes, err := resolveStatusField(num, owner, statusField, targetStatus, cfg)
	if err != nil {
		return "", err
	}

	_, err = gh.JSON[map[string]any](Runner, []string{
		"project", "item-edit", "--format", "json",
		"--id", itemID,
		"--project-id", projectID,
		"--field-id", fieldRes.FieldID,
		"--single-select-option-id", fieldRes.OptionID,
	}, nil)
	if err != nil {
		return "", err
	}

	return renderItemSetStatusOutput(num,
		renderItemSetStatusResult(repo, targetStatus, previousStatus, item, true), ctx)
}

// Project handles project subcommands.
func Project(a []string, ctx *context.RepoContext) (string, error) {
	sub := ""
	if len(a) > 0 {
		sub = a[0]
	}
	if sub == "" || args.HasFlag(a, "--help") {
		blocks := []string{ProjectHelp}
		help := suggestions.Get(suggestions.Context{Domain: "project", Action: "help", Repo: ctx})
		if len(help) > 0 {
			blocks = append(blocks, toon.RenderHelp(help))
		}
		return toon.RenderOutput(blocks...), nil
	}

	switch sub {
	case "list":
		return listProjects(a, ctx)
	case "view":
		return viewProject(a, ctx)
	case "create":
		return createProject(a, ctx)
	case "edit":
		return editProject(a, ctx)
	case "close":
		return closeProject(a, ctx)
	case "delete":
		return deleteProject(a, ctx)
	case "copy":
		return copyProject(a, ctx)
	case "mark-template":
		return markTemplateProject(a, ctx)
	case "link":
		return linkProject(a, ctx)
	case "unlink":
		return unlinkProject(a, ctx)
	case "item-list":
		return listProjectItems(a, ctx)
	case "item-add":
		return addProjectItem(a, ctx)
	case "item-create":
		return createProjectItem(a, ctx)
	case "item-edit":
		return editProjectItem(a, ctx)
	case "item-set-status":
		return setProjectItemStatus(a, ctx)
	case "item-delete":
		return deleteProjectItem(a, ctx)
	case "item-archive":
		return archiveProjectItem(a, ctx)
	case "field-list":
		return listProjectFields(a, ctx)
	case "field-create":
		return createProjectField(a, ctx)
	case "field-delete":
		return deleteProjectField(a, ctx)
	default:
		return toon.RenderError(
			"Unknown project subcommand: "+sub,
			"VALIDATION_ERROR",
			"Run `gai-ghcli project --help` for usage",
		)
	}
}
