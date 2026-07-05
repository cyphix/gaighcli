package commands

import (
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
subcommands[19]:
  list, view <number>, create, edit <number>, close <number>, delete <number>, copy <number>,
  mark-template <number>, link <number>, unlink <number>,
  item-list <number>, item-add <number>, item-create <number>, item-edit, item-delete <number>, item-archive <number>,
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
