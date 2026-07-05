package commands

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/body"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/fields"
	"github.com/cyphix/gaighcli/internal/format"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const IssueHelp = `usage: gai-ghcli issue <subcommand> [flags]
subcommands[14]:
  list, view <number>, create, edit <number>, close <number>, reopen <number>, comment <number>, delete <number>, lock <number>, unlock <number>, pin <number>, unpin <number>, transfer <number>, subissue <add|remove|list>
flags{list}:
  --state <open|closed|all>, --label <name>, --assignee <login>, --author <login>, --milestone <name>, --sort <created|updated|comments>, --limit <n> (default 30), --fields <a,b,c>
flags{view}:
  --comments, --full (show complete body without truncation)
flags{create}:
  --title <text> (required), --body <text> or --body-file <path>, --assignee <login>, --label <name> (repeatable), --milestone <name>, --project <title>, --type <name>
flags{edit}:
  --title, --body <text> or --body-file <path>, --add-label, --remove-label, --add-assignee, --remove-assignee, --milestone, --type <name>, --no-type
flags{close}:
  --reason <completed|not_planned>, --comment <text>
flags{comment}:
  --body <text> or --body-file <path> (required)
flags{transfer}:
  --to-repo <owner/name> (required)
subissue:
  add <parent> <child> [<child> ...], remove <parent> <child>, list <parent>
examples:
  gai-ghcli issue list --state closed --label bug
  gai-ghcli issue view 42 --comments
  gai-ghcli issue create --title "Fix login" --body "Steps to reproduce..."
  gai-ghcli issue comment 42 --body-file comment.md
  gai-ghcli issue close 42 --reason completed
  gai-ghcli issue transfer 42 -R source/repo --to-repo dest/repo
  gai-ghcli issue subissue add 16 20 101 125
  gai-ghcli issue subissue list 16`

const subissueHelp = `usage: gai-ghcli issue subissue <add|remove|list> <parent> [child...]
subcommands[3]:
  add <parent> <child> [<child> ...], remove <parent> <child>, list <parent>
examples:
  gai-ghcli issue subissue add 16 20 101 125
  gai-ghcli issue subissue remove 16 101
  gai-ghcli issue subissue list 16`

var (
	issueListSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
		toon.Pluck("author", "login", "author"),
		toon.RelativeTime("createdAt", "created"),
	}

	issueTypeField = toon.Custom("type", func(item map[string]any) any {
		it, _ := item["issueType"].(map[string]any)
		if it != nil {
			if name, ok := it["name"].(string); ok && name != "" {
				return name
			}
		}
		return "none"
	})

	issueViewSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
		toon.Pluck("author", "login", "author"),
		toon.RelativeTime("createdAt", "created"),
		issueTypeField,
		toon.Custom("body", func(item map[string]any) any {
			return body.TruncateBody(item["body"], 500)
		}),
	}

	issueCreateResultSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
		toon.Field("url", ""),
	}

	issueEditResultSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
		toon.JoinArray("labels", "name", "labels", ""),
		toon.JoinArray("assignees", "login", "assignees", ""),
	}

	issueStateResultSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Lower("state", ""),
	}

	issueCommentResultSchema = []toon.FieldDef{
		toon.Field("number", "issue"),
		toon.Pluck("author", "login", "author"),
		toon.RelativeTime("createdAt", "created"),
		toon.Custom("body", func(item map[string]any) any {
			return body.TruncateBody(item["body"], 800)
		}),
	}

	issueLockResultSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Lower("state", ""),
		toon.Field("locked", ""),
	}

	issuePinResultSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Lower("state", ""),
		toon.Field("isPinned", "pinned"),
	}

	issueTransferResultSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("url", ""),
	}

	issueListExtraFields = map[string]fields.ExtraFieldSpec{
		"body":      {JSONKey: "body", Def: toon.Field("body", "")},
		"closedAt":  {JSONKey: "closedAt", Def: toon.RelativeTime("closedAt", "closed_at")},
		"labels":    {JSONKey: "labels", Def: toon.JoinArray("labels", "name", "labels", "")},
		"milestone": {JSONKey: "milestone", Def: toon.Pluck("milestone", "title", "milestone")},
		"updatedAt": {JSONKey: "updatedAt", Def: toon.RelativeTime("updatedAt", "updated_at")},
		"url":       {JSONKey: "url", Def: toon.Field("url", "")},
	}
)

func issueViewSchemaWithoutType(full bool) []toon.FieldDef {
	var base []toon.FieldDef
	for _, f := range issueViewSchema {
		if f.As != "type" {
			base = append(base, f)
		}
	}
	if full {
		return issueViewSchemaFullFrom(base)
	}
	return base
}

func issueViewSchemaWithType(full bool) []toon.FieldDef {
	if full {
		return issueViewSchemaFullFrom(issueViewSchema)
	}
	return issueViewSchema
}

func issueViewSchemaFullFrom(schema []toon.FieldDef) []toon.FieldDef {
	out := make([]toon.FieldDef, len(schema))
	for i, f := range schema {
		if f.As == "body" {
			out[i] = toon.Custom("body", func(item map[string]any) any {
				if s, ok := item["body"].(string); ok {
					return s
				}
				return ""
			})
		} else {
			out[i] = f
		}
	}
	return out
}

type resolvedIssueType struct {
	ID   string
	Name string
}

type resolvedIssue struct {
	ID     string
	Number int
}

func getOptionalRequiredFlag(a []string, name string) (string, error) {
	if !args.HasFlag(a, name) {
		return "", nil
	}
	value := args.GetFlag(a, name)
	if value == "" || strings.HasPrefix(value, "--") {
		return "", errors.NewGoAIError(name+" requires a value", "VALIDATION_ERROR")
	}
	return value, nil
}

func getOwnerName(ctx *context.RepoContext) (owner, name string, err error) {
	if ctx != nil {
		return ctx.Owner, ctx.Name, nil
	}
	repo, err := gh.JSON[map[string]any](Runner, []string{"repo", "view", "--json", "owner,name"}, nil)
	if err != nil {
		return "", "", err
	}
	ownerObj, _ := repo["owner"].(map[string]any)
	login, _ := ownerObj["login"].(string)
	repoName, _ := repo["name"].(string)
	return login, repoName, nil
}

func resolveIssueType(typeName string, ctx *context.RepoContext) (resolvedIssueType, error) {
	owner, name, err := getOwnerName(ctx)
	if err != nil {
		return resolvedIssueType{}, err
	}
	query := "query($owner:String!,$name:String!){repository(owner:$owner,name:$name){issueTypes(first:25){nodes{id name}}}}"
	result, err := gh.Raw(Runner, []string{
		"api", "graphql",
		"-f", "owner=" + owner,
		"-f", "name=" + name,
		"-f", "query=" + query,
	}, nil)
	if err != nil {
		return resolvedIssueType{}, err
	}
	if result.ExitCode != 0 {
		return resolvedIssueType{}, errors.MapGhError(result.Stderr, result.ExitCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &parsed); err != nil {
		return resolvedIssueType{}, errors.NewGoAIError("Unable to resolve issue types from GitHub", "UNKNOWN")
	}
	data, _ := parsed["data"].(map[string]any)
	repo, _ := data["repository"].(map[string]any)
	issueTypes, _ := repo["issueTypes"].(map[string]any)
	nodes, _ := issueTypes["nodes"].([]any)
	if len(nodes) == 0 {
		return resolvedIssueType{}, errors.NewGoAIError(
			"Issue types are not configured for this repository. Enable them in repo settings before using --type.",
			"VALIDATION_ERROR",
		)
	}
	wanted := strings.ToLower(typeName)
	var available []string
	for _, n := range nodes {
		node, _ := n.(map[string]any)
		nodeName, _ := node["name"].(string)
		if nodeName != "" {
			available = append(available, nodeName)
			if strings.ToLower(nodeName) == wanted {
				id, _ := node["id"].(string)
				return resolvedIssueType{ID: id, Name: nodeName}, nil
			}
		}
	}
	return resolvedIssueType{}, errors.NewGoAIError(
		fmt.Sprintf(`Unknown issue type "%s". Available types: %s`, typeName, strings.Join(available, ", ")),
		"VALIDATION_ERROR",
	)
}

func applyIssueType(issueNodeID string, typeID *string) error {
	var mutation string
	ghArgs := []string{"api", "graphql", "-f", "id=" + issueNodeID}
	if typeID == nil {
		mutation = `mutation($id:ID!){updateIssue(input:{id:$id,issueTypeId:null}){issue{id}}}`
	} else {
		mutation = `mutation($id:ID!,$typeId:ID!){updateIssue(input:{id:$id,issueTypeId:$typeId}){issue{id}}}`
		ghArgs = append(ghArgs, "-f", "typeId="+*typeID)
	}
	ghArgs = append(ghArgs, "-f", "query="+mutation)
	result, err := gh.Raw(Runner, ghArgs, nil)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return errors.MapGhError(result.Stderr, result.ExitCode)
	}
	return nil
}

func requireRepo(ctx *context.RepoContext) (*context.RepoContext, error) {
	if ctx == nil {
		return nil, errors.NewGoAIError(
			"Could not determine repository — pass --repo <owner/name> or run inside a git checkout",
			"VALIDATION_ERROR",
		)
	}
	return ctx, nil
}

func gqlRequest[T any](query string) (T, error) {
	var zero T
	data, err := gh.JSON[map[string]any](Runner, []string{"api", "graphql", "-f", "query=" + query}, nil)
	if err != nil {
		return zero, err
	}
	raw, err := json.Marshal(data["data"])
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func resolveIssueIds(parent int, children []int, ctx *context.RepoContext) (resolvedIssue, []resolvedIssue, error) {
	childFields := make([]string, len(children))
	for i, n := range children {
		childFields[i] = fmt.Sprintf("c%d: issue(number: %d) { id number }", i, n)
	}
	query := fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { parent: issue(number: %d) { id number } %s } }`,
		ctx.Owner, ctx.Name, parent, strings.Join(childFields, " "),
	)
	result, err := gqlRequest[struct {
		Repository map[string]json.RawMessage `json:"repository"`
	}](query)
	if err != nil {
		return resolvedIssue{}, nil, err
	}
	repo := result.Repository
	var parentNode resolvedIssue
	if raw, ok := repo["parent"]; ok {
		_ = json.Unmarshal(raw, &parentNode)
	}
	if parentNode.ID == "" {
		return resolvedIssue{}, nil, errors.NewGoAIError(
			fmt.Sprintf("Issue #%d not found in %s", parent, ctx.NWO),
			"NOT_FOUND",
		)
	}
	var childNodes []resolvedIssue
	for i, n := range children {
		key := fmt.Sprintf("c%d", i)
		raw, ok := repo[key]
		if !ok {
			return resolvedIssue{}, nil, errors.NewGoAIError(
				fmt.Sprintf("Issue #%d not found in %s", n, ctx.NWO),
				"NOT_FOUND",
			)
		}
		var node resolvedIssue
		if err := json.Unmarshal(raw, &node); err != nil || node.ID == "" {
			return resolvedIssue{}, nil, errors.NewGoAIError(
				fmt.Sprintf("Issue #%d not found in %s", n, ctx.NWO),
				"NOT_FOUND",
			)
		}
		childNodes = append(childNodes, node)
	}
	return parentNode, childNodes, nil
}

func fetchSubIssueRelationships(num int, ctx *context.RepoContext) (parent *int, subIssues []int, err error) {
	query := fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { issue(number: %d) { parent { number } subIssues(first: 100) { totalCount nodes { number } } } } }`,
		ctx.Owner, ctx.Name, num,
	)
	data, err := gqlRequest[struct {
		Repository struct {
			Issue *struct {
				Parent *struct {
					Number int `json:"number"`
				} `json:"parent"`
				SubIssues struct {
					Nodes []struct {
						Number int `json:"number"`
					} `json:"nodes"`
				} `json:"subIssues"`
			} `json:"issue"`
		} `json:"repository"`
	}](query)
	if err != nil {
		return nil, nil, err
	}
	issue := data.Repository.Issue
	if issue == nil {
		return nil, nil, nil
	}
	if issue.Parent != nil {
		p := issue.Parent.Number
		parent = &p
	}
	for _, n := range issue.SubIssues.Nodes {
		subIssues = append(subIssues, n.Number)
	}
	return parent, subIssues, nil
}

func listIssues(a []string, ctx *context.RepoContext) (string, error) {
	if args.HasFlag(a, "--search") {
		return "", errors.NewGoAIError(
			`issue list does not support --search. Use `+"`gai-ghcli search issues \"<query>\"`"+` instead for full-text search with total counts.`,
			"VALIDATION_ERROR",
		)
	}
	work := append([]string{}, a...)
	fieldsArg := args.TakeFlag(&work, "--fields")
	parsed, err := fields.ParseFields(fieldsArg, issueListExtraFields)
	if err != nil {
		return "", err
	}
	state := args.GetFlag(work, "--state")
	label := args.GetFlag(work, "--label")
	assignee := args.GetFlag(work, "--assignee")
	author := args.GetFlag(work, "--author")
	milestone := args.GetFlag(work, "--milestone")
	sortFlag := args.GetFlag(work, "--sort")
	limit := 30
	if limitRaw := args.GetFlag(work, "--limit"); limitRaw != "" {
		if n, err := strconv.Atoi(limitRaw); err == nil {
			limit = n
		}
	}

	baseJSONFields := "number,title,state,author,createdAt"
	jsonFields := baseJSONFields
	if len(parsed.ExtraJSONKeys) > 0 {
		jsonFields += "," + strings.Join(parsed.ExtraJSONKeys, ",")
	}
	ghArgs := []string{"issue", "list", "--json", jsonFields, "--limit", strconv.Itoa(limit)}
	if state != "" {
		ghArgs = append(ghArgs, "--state", state)
	}
	if label != "" {
		ghArgs = append(ghArgs, "--label", label)
	}
	if assignee != "" {
		ghArgs = append(ghArgs, "--assignee", assignee)
	}
	if author != "" {
		ghArgs = append(ghArgs, "--author", author)
	}
	if milestone != "" {
		ghArgs = append(ghArgs, "--milestone", milestone)
	}
	if sortFlag != "" {
		ghArgs = append(ghArgs, "--search", "sort:"+sortFlag+"-desc")
	}

	items, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	isEmpty := len(items) == 0

	var totalCount *int
	if len(items) == limit && ctx != nil {
		ghState := strings.ToUpper(state)
		if ghState == "" {
			ghState = "OPEN"
		}
		query := fmt.Sprintf(`{ repository(owner:"%s", name:"%s") { issues(states:[%s]) { totalCount } } }`, ctx.Owner, ctx.Name, ghState)
		gqlResult, err := gh.Raw(Runner, []string{"api", "graphql", "-f", "query=" + query}, nil)
		if err == nil && gqlResult.ExitCode == 0 {
			var parsed struct {
				Data struct {
					Repository struct {
						Issues struct {
							TotalCount int `json:"totalCount"`
						} `json:"issues"`
					} `json:"repository"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(gqlResult.Stdout), &parsed) == nil {
				tc := parsed.Data.Repository.Issues.TotalCount
				totalCount = &tc
			}
		}
	}
	countLine := format.CountLine(format.CountLineOptions{Count: len(items), Limit: &limit, TotalCount: totalCount})

	extendedSchema := issueListSchema
	if len(parsed.ExtraDefs) > 0 {
		extendedSchema = append(append([]toon.FieldDef{}, issueListSchema...), parsed.ExtraDefs...)
	}
	list, err := toon.RenderList("issues", items, extendedSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{countLine, list}, suggestions.Context{
		Domain: "issue", Action: "list", IsEmpty: boolPtr(isEmpty), Repo: ctx,
	})
}

func viewIssue(a []string, ctx *context.RepoContext) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}
	withComments := args.HasFlag(a, "--comments")
	full := args.HasFlag(a, "--full")

	baseFields := "number,title,state,author,createdAt,body"
	if withComments {
		baseFields += ",comments"
	}
	fieldsWithType := baseFields + ",issueType"
	ghArgs := []string{"issue", "view", strconv.Itoa(num), "--json", fieldsWithType}

	item, err := gh.JSON[map[string]any](Runner, ghArgs, ctx)
	supportsIssueType := true
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "issuetype") {
			supportsIssueType = false
			item, err = gh.JSON[map[string]any](Runner,
				[]string{"issue", "view", strconv.Itoa(num), "--json", baseFields}, ctx)
		}
		if err != nil {
			return "", err
		}
	}

	var baseSchema []toon.FieldDef
	if supportsIssueType {
		baseSchema = issueViewSchemaWithType(full)
	} else {
		baseSchema = issueViewSchemaWithoutType(full)
	}

	var parentNum *int
	var childNums []int
	if ctx != nil {
		p, children, _ := fetchSubIssueRelationships(num, ctx)
		parentNum = p
		childNums = children
	}

	schema := append([]toon.FieldDef{}, baseSchema...)
	augmented := make(map[string]any)
	for k, v := range item {
		augmented[k] = v
	}
	if len(childNums) > 0 {
		tags := make([]string, len(childNums))
		for i, n := range childNums {
			tags[i] = "#" + strconv.Itoa(n)
		}
		augmented["_subissues"] = tags
		schema = append(schema, toon.Custom("subissues", func(it map[string]any) any {
			return it["_subissues"]
		}))
	}
	if parentNum != nil {
		augmented["_parent"] = "#" + strconv.Itoa(*parentNum)
		schema = append(schema, toon.Custom("parent", func(it map[string]any) any {
			return it["_parent"]
		}))
	}

	detail, err := toon.RenderDetail("issue", augmented, schema)
	if err != nil {
		return "", err
	}
	blocks := []string{detail}

	if withComments {
		if comments, ok := item["comments"].([]any); ok {
			commentItems := make([]map[string]any, len(comments))
			for i, c := range comments {
				commentItems[i], _ = c.(map[string]any)
			}
			var commentSchema []toon.FieldDef
			for _, d := range issueCommentResultSchema {
				if d.Key != "number" {
					commentSchema = append(commentSchema, d)
				}
			}
			list, err := toon.RenderList("comments", commentItems, commentSchema)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, list)
		}
	}
	return toon.RenderOutput(blocks...), nil
}

func createIssue(a []string, ctx *context.RepoContext) (string, error) {
	title := args.GetFlag(a, "--title")
	if title == "" {
		return "", errors.NewGoAIError("--title is required", "VALIDATION_ERROR")
	}
	work := append([]string{}, a...)
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}
	assignee := args.GetFlag(work, "--assignee")
	labels := args.GetAllFlags(work, "--label")
	milestone := args.GetFlag(work, "--milestone")
	project := args.GetFlag(work, "--project")
	typeName, err := getOptionalRequiredFlag(work, "--type")
	if err != nil {
		return "", err
	}

	var resolvedType *resolvedIssueType
	if typeName != "" {
		rt, err := resolveIssueType(typeName, ctx)
		if err != nil {
			return "", err
		}
		resolvedType = &rt
	}

	ghArgs := []string{"issue", "create", "--title", title}
	if bodyText != "" {
		ghArgs = append(ghArgs, "--body", bodyText)
	}
	if assignee != "" {
		ghArgs = append(ghArgs, "--assignee", assignee)
	}
	for _, l := range labels {
		ghArgs = append(ghArgs, "--label", l)
	}
	if milestone != "" {
		ghArgs = append(ghArgs, "--milestone", milestone)
	}
	if project != "" {
		ghArgs = append(ghArgs, "--project", project)
	}

	output, err := gh.Exec(Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	urlRe := regexp.MustCompile(`https://github\.com/[^\s]+`)
	numRe := regexp.MustCompile(`/issues/(\d+)`)
	url := output
	if m := urlRe.FindString(output); m != "" {
		url = m
	} else {
		url = strings.TrimSpace(output)
	}
	num := 0
	if m := numRe.FindStringSubmatch(url); len(m) == 2 {
		num, _ = strconv.Atoi(m[1])
	}

	item, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "number,title,state,url,id"}, ctx)
	if err != nil {
		return "", err
	}

	if resolvedType != nil {
		if issueNodeID, ok := item["id"].(string); ok && issueNodeID != "" {
			if err := applyIssueType(issueNodeID, &resolvedType.ID); err != nil {
				return "", err
			}
		}
		item["issueType"] = map[string]any{"name": resolvedType.Name}
	}

	schema := issueCreateResultSchema
	if resolvedType != nil {
		schema = append(append([]toon.FieldDef{}, issueCreateResultSchema...), issueTypeField)
	}
	detail, err := toon.RenderDetail("issue", item, schema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: "create", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func editIssue(a []string, ctx *context.RepoContext) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}
	work := append([]string{}, a...)
	title := args.GetFlag(work, "--title")
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}
	addLabel := args.GetFlag(work, "--add-label")
	removeLabel := args.GetFlag(work, "--remove-label")
	addAssignee := args.GetFlag(work, "--add-assignee")
	removeAssignee := args.GetFlag(work, "--remove-assignee")
	milestone := args.GetFlag(work, "--milestone")
	clearType := args.TakeBoolFlag(&work, "--no-type")
	typeName, err := getOptionalRequiredFlag(work, "--type")
	if err != nil {
		return "", err
	}

	var resolvedType *resolvedIssueType
	if typeName != "" {
		rt, err := resolveIssueType(typeName, ctx)
		if err != nil {
			return "", err
		}
		resolvedType = &rt
	}

	ghArgs := []string{"issue", "edit", strconv.Itoa(num)}
	if title != "" {
		ghArgs = append(ghArgs, "--title", title)
	}
	if bodyText != "" {
		ghArgs = append(ghArgs, "--body", bodyText)
	}
	if addLabel != "" {
		ghArgs = append(ghArgs, "--add-label", addLabel)
	}
	if removeLabel != "" {
		ghArgs = append(ghArgs, "--remove-label", removeLabel)
	}
	if addAssignee != "" {
		ghArgs = append(ghArgs, "--add-assignee", addAssignee)
	}
	if removeAssignee != "" {
		ghArgs = append(ghArgs, "--remove-assignee", removeAssignee)
	}
	if milestone != "" {
		ghArgs = append(ghArgs, "--milestone", milestone)
	}

	if len(ghArgs) > 3 {
		if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
			return "", err
		}
	}

	item, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "number,title,state,labels,assignees,id"}, ctx)
	if err != nil {
		return "", err
	}

	if resolvedType != nil || clearType {
		if issueNodeID, ok := item["id"].(string); ok && issueNodeID != "" {
			var typeID *string
			if resolvedType != nil {
				typeID = &resolvedType.ID
			}
			if err := applyIssueType(issueNodeID, typeID); err != nil {
				return "", err
			}
		}
		if resolvedType != nil {
			item["issueType"] = map[string]any{"name": resolvedType.Name}
		} else if clearType {
			item["issueType"] = nil
		}
	}

	schema := issueEditResultSchema
	if resolvedType != nil || clearType {
		schema = append(append([]toon.FieldDef{}, issueEditResultSchema...), issueTypeField)
	}
	detail, err := toon.RenderDetail("issue", item, schema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: "edit", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func closeIssue(a []string, ctx *context.RepoContext) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}
	reason := args.GetFlag(a, "--reason")
	comment := args.GetFlag(a, "--comment")

	current, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "state"}, ctx)
	if err != nil {
		return "", err
	}
	state, _ := current["state"].(string)
	if strings.ToLower(state) == "closed" {
		item, err := gh.JSON[map[string]any](Runner,
			[]string{"issue", "view", strconv.Itoa(num), "--json", "number,state"}, ctx)
		if err != nil {
			return "", err
		}
		item["_message"] = "Already closed"
		schema := append(append([]toon.FieldDef{}, issueStateResultSchema...), toon.Field("_message", "message"))
		detail, err := toon.RenderDetail("issue", item, schema)
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "issue", Action: "close", ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	ghArgs := []string{"issue", "close", strconv.Itoa(num)}
	if reason != "" {
		ghArgs = append(ghArgs, "--reason", reason)
	}
	if comment != "" {
		ghArgs = append(ghArgs, "--comment", comment)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}

	item, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "number,state"}, ctx)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("issue", item, issueStateResultSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: "close", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func reopenIssue(a []string, ctx *context.RepoContext) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}

	current, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "state"}, ctx)
	if err != nil {
		return "", err
	}
	state, _ := current["state"].(string)
	if strings.ToLower(state) == "open" {
		item, err := gh.JSON[map[string]any](Runner,
			[]string{"issue", "view", strconv.Itoa(num), "--json", "number,state"}, ctx)
		if err != nil {
			return "", err
		}
		item["_message"] = "Already open"
		schema := append(append([]toon.FieldDef{}, issueStateResultSchema...), toon.Field("_message", "message"))
		detail, err := toon.RenderDetail("issue", item, schema)
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "issue", Action: "reopen", ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	if _, err := gh.Exec(Runner, []string{"issue", "reopen", strconv.Itoa(num)}, ctx); err != nil {
		return "", err
	}

	item, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "number,state"}, ctx)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("issue", item, issueStateResultSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: "reopen", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func commentOnIssue(a []string, ctx *context.RepoContext) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}
	work := append([]string{}, a...)
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{Required: true})
	if err != nil {
		return "", err
	}

	if _, err := gh.Exec(Runner, []string{"issue", "comment", strconv.Itoa(num), "--body", bodyText}, ctx); err != nil {
		return "", err
	}

	issue, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "comments"}, ctx)
	if err != nil {
		return "", err
	}
	comments, _ := issue["comments"].([]any)
	var lastComment map[string]any
	if len(comments) > 0 {
		lastComment, _ = comments[len(comments)-1].(map[string]any)
	}
	if lastComment == nil {
		lastComment = map[string]any{}
	}
	lastComment["number"] = num

	detail, err := toon.RenderDetail("comment", lastComment, issueCommentResultSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: "comment", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func deleteIssue(a []string, ctx *context.RepoContext) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}

	if _, err := gh.Exec(Runner, []string{"issue", "delete", strconv.Itoa(num), "--yes"}, ctx); err != nil {
		return "", err
	}

	detail, err := toon.RenderDetail("issue", map[string]any{
		"number": num,
		"status": "deleted",
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("status", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: "delete", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func lockIssue(a []string, ctx *context.RepoContext) (string, error) {
	return issueLockUnlockPin(a, ctx, "lock", true)
}

func unlockIssue(a []string, ctx *context.RepoContext) (string, error) {
	return issueLockUnlockPin(a, ctx, "unlock", false)
}

func pinIssue(a []string, ctx *context.RepoContext) (string, error) {
	return issuePinUnpin(a, ctx, "pin", true)
}

func unpinIssue(a []string, ctx *context.RepoContext) (string, error) {
	return issuePinUnpin(a, ctx, "unpin", false)
}

func issueLockUnlockPin(a []string, ctx *context.RepoContext, action string, _ bool) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}

	current, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "state,locked"}, ctx)
	if err != nil {
		return "", err
	}
	locked, _ := current["locked"].(bool)
	state, _ := current["state"].(string)

	already := (action == "lock" && locked) || (action == "unlock" && !locked)
	if already {
		msg := "Already locked"
		if action == "unlock" {
			msg = "Already unlocked"
		}
		item := map[string]any{
			"number": num,
			"state":  state,
			"locked": locked,
			"_message": msg,
		}
		schema := append(append([]toon.FieldDef{}, issueLockResultSchema...), toon.Field("_message", "message"))
		detail, err := toon.RenderDetail("issue", item, schema)
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "issue", Action: action, ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	if _, err := gh.Exec(Runner, []string{"issue", action, strconv.Itoa(num)}, ctx); err != nil {
		return "", err
	}

	item, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "number,state,locked"}, ctx)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("issue", item, issueLockResultSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: action, ID: strconv.Itoa(num), Repo: ctx,
	})
}

func issuePinUnpin(a []string, ctx *context.RepoContext, action string, _ bool) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}

	current, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "state,isPinned"}, ctx)
	if err != nil {
		return "", err
	}
	isPinned, _ := current["isPinned"].(bool)
	state, _ := current["state"].(string)

	already := (action == "pin" && isPinned) || (action == "unpin" && !isPinned)
	if already {
		msg := "Already pinned"
		if action == "unpin" {
			msg = "Already unpinned"
		}
		item := map[string]any{
			"number":   num,
			"state":    state,
			"isPinned": isPinned,
			"_message": msg,
		}
		schema := append(append([]toon.FieldDef{}, issuePinResultSchema...), toon.Field("_message", "message"))
		detail, err := toon.RenderDetail("issue", item, schema)
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "issue", Action: action, ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	if _, err := gh.Exec(Runner, []string{"issue", action, strconv.Itoa(num)}, ctx); err != nil {
		return "", err
	}

	item, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "number,state,isPinned"}, ctx)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("issue", item, issuePinResultSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: action, ID: strconv.Itoa(num), Repo: ctx,
	})
}

func transferIssue(a []string, ctx *context.RepoContext) (string, error) {
	num, err := args.RequireNumber(args.GetPositional(a, 1), "issue")
	if err != nil {
		return "", err
	}
	destRepo := args.GetFlag(a, "--to-repo")
	if destRepo == "" {
		return "", errors.NewGoAIError("--to-repo is required for transfer", "VALIDATION_ERROR")
	}

	if _, err := gh.Exec(Runner, []string{"issue", "transfer", strconv.Itoa(num), destRepo}, ctx); err != nil {
		return "", err
	}

	destCtx := &context.RepoContext{NWO: destRepo, Source: context.SourceFlag}
	if parts := strings.Split(destRepo, "/"); len(parts) == 2 {
		destCtx.Owner = parts[0]
		destCtx.Name = parts[1]
	}

	item := map[string]any{"number": num, "url": fmt.Sprintf("https://github.com/%s/issues/%d", destRepo, num)}
	fetched, err := gh.JSON[map[string]any](Runner,
		[]string{"issue", "view", strconv.Itoa(num), "--json", "number,url"}, destCtx)
	if err == nil {
		item = fetched
	}

	detail, err := toon.RenderDetail("issue", item, issueTransferResultSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "issue", Action: "transfer", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func subissueAdd(a []string, ctx *context.RepoContext) (string, error) {
	repo, err := requireRepo(ctx)
	if err != nil {
		return "", err
	}
	parentNum, err := args.RequireNumber(args.GetPositional(a, 2), "parent")
	if err != nil {
		return "", err
	}
	var childRaw []string
	for _, arg := range a[3:] {
		if !strings.HasPrefix(arg, "--") {
			childRaw = append(childRaw, arg)
		}
	}
	if len(childRaw) == 0 {
		return "", errors.NewGoAIError("subissue add requires at least one child issue number", "VALIDATION_ERROR")
	}
	childNums := make([]int, len(childRaw))
	for i, r := range childRaw {
		childNums[i], err = args.RequireNumber(r, "child")
		if err != nil {
			return "", err
		}
	}

	parent, children, err := resolveIssueIds(parentNum, childNums, repo)
	if err != nil {
		return "", err
	}

	var addedNumbers []int
	for _, child := range children {
		mutation := fmt.Sprintf(
			`mutation { addSubIssue(input: { issueId: "%s", subIssueId: "%s" }) { subIssue { number } } }`,
			parent.ID, child.ID,
		)
		var result struct {
			AddSubIssue *struct {
				SubIssue *struct {
					Number *int `json:"number"`
				} `json:"subIssue"`
			} `json:"addSubIssue"`
		}
		result, gqlErr := gqlRequest[struct {
			AddSubIssue *struct {
				SubIssue *struct {
					Number *int `json:"number"`
				} `json:"subIssue"`
			} `json:"addSubIssue"`
		}](mutation)
		if gqlErr != nil {
			if len(addedNumbers) == 0 {
				return "", gqlErr
			}
			added := make([]string, len(addedNumbers))
			for i, n := range addedNumbers {
				added[i] = "#" + strconv.Itoa(n)
			}
			return "", errors.NewGoAIError(
				gqlErr.Error()+"\nAdded before failure: "+strings.Join(added, ", "),
				"UNKNOWN",
			)
		}
		if result.AddSubIssue != nil && result.AddSubIssue.SubIssue != nil && result.AddSubIssue.SubIssue.Number != nil {
			addedNumbers = append(addedNumbers, *result.AddSubIssue.SubIssue.Number)
		} else {
			addedNumbers = append(addedNumbers, child.Number)
		}
	}

	addedTags := make([]string, len(addedNumbers))
	for i, n := range addedNumbers {
		addedTags[i] = "#" + strconv.Itoa(n)
	}
	detail, err := toon.RenderDetail("subissue_add", map[string]any{
		"parent": "#" + strconv.Itoa(parent.Number),
		"added":  addedTags,
	}, []toon.FieldDef{
		toon.Field("parent", ""),
		toon.Custom("added", func(it map[string]any) any { return it["added"] }),
	})
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(detail, toon.RenderHelp([]string{
		fmt.Sprintf("Run `gai-ghcli issue view %d` to see the parent with its sub-issues", parent.Number),
	})), nil
}

func subissueRemove(a []string, ctx *context.RepoContext) (string, error) {
	repo, err := requireRepo(ctx)
	if err != nil {
		return "", err
	}
	parentNum, err := args.RequireNumber(args.GetPositional(a, 2), "parent")
	if err != nil {
		return "", err
	}
	childRaw := args.GetPositional(a, 3)
	if childRaw == "" {
		return "", errors.NewGoAIError("subissue remove requires a child issue number", "VALIDATION_ERROR")
	}
	childNum, err := args.RequireNumber(childRaw, "child")
	if err != nil {
		return "", err
	}

	parent, children, err := resolveIssueIds(parentNum, []int{childNum}, repo)
	if err != nil {
		return "", err
	}
	child := children[0]

	mutation := fmt.Sprintf(
		`mutation { removeSubIssue(input: { issueId: "%s", subIssueId: "%s" }) { issue { number } } }`,
		parent.ID, child.ID,
	)
	if _, err := gqlRequest[any](mutation); err != nil {
		return "", err
	}

	detail, err := toon.RenderDetail("subissue_remove", map[string]any{
		"parent":  "#" + strconv.Itoa(parent.Number),
		"removed": "#" + strconv.Itoa(child.Number),
	}, []toon.FieldDef{toon.Field("parent", ""), toon.Field("removed", "")})
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(detail, toon.RenderHelp([]string{
		fmt.Sprintf("Run `gai-ghcli issue subissue list %d` to see remaining sub-issues", parent.Number),
	})), nil
}

func subissueList(a []string, ctx *context.RepoContext) (string, error) {
	repo, err := requireRepo(ctx)
	if err != nil {
		return "", err
	}
	parentNum, err := args.RequireNumber(args.GetPositional(a, 2), "parent")
	if err != nil {
		return "", err
	}

	query := fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { issue(number: %d) { subIssues(first: 100) { totalCount nodes { number title state } } } } }`,
		repo.Owner, repo.Name, parentNum,
	)
	data, err := gqlRequest[struct {
		Repository struct {
			Issue *struct {
				SubIssues struct {
					TotalCount int                    `json:"totalCount"`
					Nodes      []map[string]any       `json:"nodes"`
				} `json:"subIssues"`
			} `json:"issue"`
		} `json:"repository"`
	}](query)
	if err != nil {
		return "", err
	}
	issue := data.Repository.Issue
	if issue == nil {
		return "", errors.NewGoAIError(
			fmt.Sprintf("Issue #%d not found in %s", parentNum, repo.NWO),
			"NOT_FOUND",
		)
	}

	nodes := issue.SubIssues.Nodes
	totalCount := issue.SubIssues.TotalCount
	if totalCount == 0 {
		totalCount = len(nodes)
	}
	limit := 100
	countLine := format.CountLine(format.CountLineOptions{
		Count: len(nodes), Limit: &limit, TotalCount: &totalCount,
	})
	schema := []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
	}
	list, err := toon.RenderList("subissues", nodes, schema)
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(
		fmt.Sprintf("parent: #%d", parentNum),
		countLine,
		list,
	), nil
}

func subissueCommand(a []string, ctx *context.RepoContext) (string, error) {
	sub := ""
	if len(a) > 1 {
		sub = a[1]
	}
	if sub == "" || args.HasFlag(a, "--help") {
		return toon.RenderOutput(subissueHelp), nil
	}
	switch sub {
	case "add":
		return subissueAdd(a, ctx)
	case "remove":
		return subissueRemove(a, ctx)
	case "list":
		return subissueList(a, ctx)
	default:
		return toon.RenderError(
			"Unknown subissue subcommand: "+sub,
			"VALIDATION_ERROR",
			"Run `gai-ghcli issue subissue --help` for usage",
		)
	}
}

// Issue handles issue subcommands.
func Issue(a []string, ctx *context.RepoContext) (string, error) {
	sub := ""
	if len(a) > 0 {
		sub = a[0]
	}

	if sub == "subissue" {
		return subissueCommand(a, ctx)
	}

	if sub == "" || args.HasFlag(a, "--help") {
		blocks := []string{IssueHelp}
		help := suggestions.Get(suggestions.Context{Domain: "issue", Action: "help", Repo: ctx})
		if len(help) > 0 {
			blocks = append(blocks, toon.RenderHelp(help))
		}
		return toon.RenderOutput(blocks...), nil
	}

	switch sub {
	case "list":
		return listIssues(a, ctx)
	case "view":
		return viewIssue(a, ctx)
	case "create":
		return createIssue(a, ctx)
	case "edit":
		return editIssue(a, ctx)
	case "close":
		return closeIssue(a, ctx)
	case "reopen":
		return reopenIssue(a, ctx)
	case "comment":
		return commentOnIssue(a, ctx)
	case "delete":
		return deleteIssue(a, ctx)
	case "lock":
		return lockIssue(a, ctx)
	case "unlock":
		return unlockIssue(a, ctx)
	case "pin":
		return pinIssue(a, ctx)
	case "unpin":
		return unpinIssue(a, ctx)
	case "transfer":
		return transferIssue(a, ctx)
	default:
		return toon.RenderError(
			"Unknown issue subcommand: "+sub,
			"VALIDATION_ERROR",
			"Run `gai-ghcli issue --help` for usage",
		)
	}
}
