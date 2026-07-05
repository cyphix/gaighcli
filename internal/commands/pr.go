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

const PRHelp = `usage: gai-ghcli pr <subcommand> [flags]
subcommands[15]:
  list, view <number>, create, edit <number>, close <number>, merge <number>, review <number>, checks <number>, diff <number>, checkout <number>, ready <number>, reopen <number>, comment <number>, update-branch <number>, revert <number>
flags{list}:
  --state <open|closed|all>, --label, --assignee, --author, --base, --head, --draft, --limit <n> (default 30), --fields <a,b,c>
flags{view}:
  --comments, --reviews (show review submissions and inline review comments), --full (show complete body without truncation)
flags{create}:
  --title <text> (required), --body <text> or --body-file <path>, --base, --head, --draft, --assignee, --reviewer, --label <name> (repeatable), --milestone
flags{edit}:
  --title <text>, --body <text> or --body-file <path>, --add-label, --remove-label, --add-assignee, --remove-assignee, --add-reviewer, --remove-reviewer, --milestone
flags{merge}:
  --method <merge|squash|rebase>, --merge, --squash, --rebase, --auto, --delete-branch, --body <text> or --body-file <path>, --subject
flags{review}:
  --approve, --request-changes, --comment, --body <text> or --body-file <path>
flags{comment}:
  --body <text> or --body-file <path> (required)
flags{checks}:
  (none)
flags{diff}:
  --full (show complete diff without truncation)
examples:
  gai-ghcli pr list --state open --label bug
  gai-ghcli pr view 42 --comments
  gai-ghcli pr view 42 --reviews
  gai-ghcli pr comment 42 --body-file review.md
  gai-ghcli pr merge 42 --squash --delete-branch`

const diffTruncateLimit = 4000

var (
	prReviewMap = map[string]string{
		"APPROVED":          "approved",
		"CHANGES_REQUESTED": "changes_requested",
		"REVIEW_REQUIRED":   "required",
	}
	prReviewStateMap = map[string]string{
		"APPROVED":          "approved",
		"CHANGES_REQUESTED": "changes_requested",
		"COMMENTED":         "commented",
		"DISMISSED":         "dismissed",
		"PENDING":           "pending",
	}

	prListSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
		toon.Pluck("author", "login", "author"),
		toon.BoolYesNo("isDraft", "draft"),
		toon.MapEnum("reviewDecision", prReviewMap, "none", "review"),
	}

	prListJSONFields = "number,title,state,author,isDraft,reviewDecision"

	prListExtraFields = map[string]fields.ExtraFieldSpec{
		"body":      {JSONKey: "body", Def: toon.Field("body", "")},
		"createdAt": {JSONKey: "createdAt", Def: toon.RelativeTime("createdAt", "created")},
		"labels":    {JSONKey: "labels", Def: toon.JoinArray("labels", "name", "labels", "")},
		"milestone": {JSONKey: "milestone", Def: toon.Pluck("milestone", "title", "milestone")},
		"mergedAt":  {JSONKey: "mergedAt", Def: toon.RelativeTime("mergedAt", "merged_at")},
		"url":       {JSONKey: "url", Def: toon.Field("url", "")},
	}

	prViewJSONFields = "number,title,state,author,isDraft,mergedAt,statusCheckRollup,body,comments,reviews"
)

func prViewSchema(full bool) []toon.FieldDef {
	schema := []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
		toon.Pluck("author", "login", "author"),
		toon.BoolYesNo("isDraft", "draft"),
		toon.Custom("merged", func(item map[string]any) any {
			state, _ := item["state"].(string)
			if strings.ToUpper(state) == "MERGED" {
				if mergedAt, ok := item["mergedAt"].(string); ok && mergedAt != "" {
					return mergedAt
				}
				return "yes"
			}
			return "no"
		}),
		toon.Custom("checks", func(item map[string]any) any {
			checks, _ := item["statusCheckRollup"].([]any)
			if len(checks) == 0 {
				return "0 passed, 0 failed — this PR has no CI checks configured"
			}
			passed, failed, skipped := 0, 0, 0
			for _, c := range checks {
				m, _ := c.(map[string]any)
				switch classifyCheck(m) {
				case "pass":
					passed++
				case "fail":
					failed++
				case "skip":
					skipped++
				}
			}
			parts := []string{
				fmt.Sprintf("%d passed", passed),
				fmt.Sprintf("%d failed", failed),
			}
			if skipped > 0 {
				parts = append(parts, fmt.Sprintf("%d skipped", skipped))
			}
			parts = append(parts, fmt.Sprintf("%d total", len(checks)))
			return strings.Join(parts, ", ")
		}),
	}
	if full {
		schema = append(schema, toon.Custom("body", func(item map[string]any) any {
			if s, ok := item["body"].(string); ok {
				return s
			}
			return ""
		}))
	} else {
		schema = append(schema, toon.Custom("body", func(item map[string]any) any {
			return body.TruncateBody(item["body"], 500)
		}))
	}
	return schema
}

func classifyCheck(c map[string]any) string {
	if c == nil {
		return "pending"
	}
	conc := strings.ToUpper(fmt.Sprint(c["conclusion"]))
	st := strings.ToUpper(fmt.Sprint(c["state"]))
	if st == "" {
		st = strings.ToUpper(fmt.Sprint(c["status"]))
	}
	if conc == "SUCCESS" || conc == "NEUTRAL" {
		return "pass"
	}
	if conc == "FAILURE" || conc == "TIMED_OUT" || conc == "ACTION_REQUIRED" {
		return "fail"
	}
	if conc == "SKIPPED" || conc == "CANCELLED" || st == "EXPECTED" || st == "NEUTRAL" {
		return "skip"
	}
	return "pending"
}

func prRestPath(ctx *context.RepoContext, num int, suffix string) string {
	repoPath := "repos/{owner}/{repo}"
	if ctx != nil {
		repoPath = fmt.Sprintf("repos/%s/%s", ctx.Owner, ctx.Name)
	}
	return fmt.Sprintf("%s/pulls/%d/%s", repoPath, num, suffix)
}

func flattenPaginated[T any](items any) []T {
	switch v := items.(type) {
	case []any:
		if len(v) > 0 {
			if _, ok := v[0].([]any); ok {
				var flat []T
				for _, page := range v {
					if pageArr, ok := page.([]any); ok {
						for _, item := range pageArr {
							if t, ok := item.(T); ok {
								flat = append(flat, t)
							} else if m, ok := item.(map[string]any); ok {
								var t T
								raw, _ := json.Marshal(m)
								_ = json.Unmarshal(raw, &t)
								flat = append(flat, t)
							}
						}
					}
				}
				return flat
			}
		}
		var out []T
		for _, item := range v {
			if t, ok := item.(T); ok {
				out = append(out, t)
			} else if m, ok := item.(map[string]any); ok {
				var t T
				raw, _ := json.Marshal(m)
				_ = json.Unmarshal(raw, &t)
				out = append(out, t)
			}
		}
		return out
	default:
		raw, _ := json.Marshal(items)
		var out []T
		_ = json.Unmarshal(raw, &out)
		return out
	}
}

func ghAPIPaginatedArray(ctx *context.RepoContext, path string) ([]map[string]any, error) {
	pages, err := gh.JSON[any](Runner, []string{"api", path, "--paginate", "--slurp"}, ctx)
	if err != nil {
		return nil, err
	}
	return flattenPaginated[map[string]any](pages), nil
}

func prList(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	for _, arg := range work {
		if arg == "--search" {
			return "", errors.NewGoAIError(
				`pr list does not support --search. Use `+"`gai-ghcli search prs \"<query>\"`"+` instead for full-text search with total counts.`,
				"VALIDATION_ERROR",
			)
		}
	}

	fieldsArg := args.TakeFlag(&work, "--fields")
	parsed, err := fields.ParseFields(fieldsArg, prListExtraFields)
	if err != nil {
		return "", err
	}
	state := args.TakeFlag(&work, "--state")
	if state == "" {
		state = "open"
	}
	label := args.TakeFlag(&work, "--label")
	assignee := args.TakeFlag(&work, "--assignee")
	author := args.TakeFlag(&work, "--author")
	base := args.TakeFlag(&work, "--base")
	head := args.TakeFlag(&work, "--head")
	draft := args.TakeBoolFlag(&work, "--draft")
	limit := args.TakeFlag(&work, "--limit")
	if limit == "" {
		limit = "30"
	}

	jsonFields := prListJSONFields
	if len(parsed.ExtraJSONKeys) > 0 {
		jsonFields += "," + strings.Join(parsed.ExtraJSONKeys, ",")
	}
	ghArgs := []string{"pr", "list", "--json", jsonFields, "--state", state, "--limit", limit}
	if label != "" {
		ghArgs = append(ghArgs, "--label", label)
	}
	if assignee != "" {
		ghArgs = append(ghArgs, "--assignee", assignee)
	}
	if author != "" {
		ghArgs = append(ghArgs, "--author", author)
	}
	if base != "" {
		ghArgs = append(ghArgs, "--base", base)
	}
	if head != "" {
		ghArgs = append(ghArgs, "--head", head)
	}
	if draft {
		ghArgs = append(ghArgs, "--draft")
	}

	items, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	isEmpty := len(items) == 0
	limitNum, _ := strconv.Atoi(limit)

	var totalCount *int
	if len(items) == limitNum && ctx != nil {
		ghState := strings.ToUpper(state)
		statesFilter := ""
		if ghState != "ALL" {
			if ghState == "CLOSED" {
				statesFilter = "states:[CLOSED,MERGED]"
			} else {
				statesFilter = "states:[" + ghState + "]"
			}
		}
		query := fmt.Sprintf(`{ repository(owner:"%s", name:"%s") { pullRequests(%s) { totalCount } } }`, ctx.Owner, ctx.Name, statesFilter)
		gqlResult, err := gh.Raw(Runner, []string{"api", "graphql", "-f", "query=" + query}, nil)
		if err == nil && gqlResult.ExitCode == 0 {
			var parsed struct {
				Data struct {
					Repository struct {
						PullRequests struct {
							TotalCount int `json:"totalCount"`
						} `json:"pullRequests"`
					} `json:"repository"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(gqlResult.Stdout), &parsed) == nil {
				tc := parsed.Data.Repository.PullRequests.TotalCount
				totalCount = &tc
			}
		}
	}

	countLine := format.CountLine(format.CountLineOptions{
		Count: len(items), Limit: &limitNum, TotalCount: totalCount,
	})
	extendedSchema := prListSchema
	if len(parsed.ExtraDefs) > 0 {
		extendedSchema = append(append([]toon.FieldDef{}, prListSchema...), parsed.ExtraDefs...)
	}
	list, err := toon.RenderList("pull_requests", items, extendedSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{countLine, list}, suggestions.Context{
		Domain: "pr", Action: "list", IsEmpty: boolPtr(isEmpty), Repo: ctx,
	})
}

func prView(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	includeComments := args.TakeBoolFlag(&work, "--comments")
	includeReviews := args.TakeBoolFlag(&work, "--reviews")
	full := args.TakeBoolFlag(&work, "--full")
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	pr, err := gh.JSON[map[string]any](Runner,
		[]string{"pr", "view", strconv.Itoa(num), "--json", prViewJSONFields}, ctx)
	if err != nil {
		return "", err
	}

	schema := prViewSchema(full)
	if includeComments {
		if comments, ok := pr["comments"].([]any); ok {
			schema = append(schema, toon.Custom("comments", func(_ map[string]any) any {
				out := make([]map[string]any, len(comments))
				for i, c := range comments {
					cm, _ := c.(map[string]any)
					author := "unknown"
					if aObj, ok := cm["author"].(map[string]any); ok {
						if login, ok := aObj["login"].(string); ok {
							author = login
						}
					}
					out[i] = map[string]any{
						"author":  author,
						"body":    cm["body"],
						"created": cm["createdAt"],
					}
				}
				return out
			}))
		}
	} else {
		commentCount := 0
		if comments, ok := pr["comments"].([]any); ok {
			commentCount = len(comments)
		}
		schema = append(schema, toon.Custom("comment_count", func(_ map[string]any) any {
			return fmt.Sprintf("%d — use --comments to see full comments", commentCount)
		}))
	}

	if includeReviews {
		reviews, err := ghAPIPaginatedArray(ctx, prRestPath(ctx, num, "reviews"))
		if err != nil {
			return "", err
		}
		var inlineComments []map[string]any
		if len(reviews) > 0 {
			inlineComments, err = ghAPIPaginatedArray(ctx, prRestPath(ctx, num, "comments"))
			if err != nil {
				return "", err
			}
		}
		commentsByReview := make(map[float64][]map[string]any)
		for _, c := range inlineComments {
			if id, ok := c["pull_request_review_id"].(float64); ok {
				commentsByReview[id] = append(commentsByReview[id], c)
			}
		}
		schema = append(schema, toon.Custom("reviews", func(_ map[string]any) any {
			out := make([]map[string]any, len(reviews))
			for i, r := range reviews {
				stateUpper := strings.ToUpper(fmt.Sprint(r["state"]))
				state := prReviewStateMap[stateUpper]
				if state == "" {
					state = strings.ToLower(stateUpper)
					if state == "" {
						state = "unknown"
					}
				}
				var reviewID float64
				if id, ok := r["id"].(float64); ok {
					reviewID = id
				}
				inline := commentsByReview[reviewID]
				inlineOut := make([]map[string]any, len(inline))
				for j, c := range inline {
					author := "unknown"
					if u, ok := c["user"].(map[string]any); ok {
						if login, ok := u["login"].(string); ok {
							author = login
						}
					}
					line := c["line"]
					if line == nil {
						line = c["original_line"]
					}
					inlineOut[j] = map[string]any{
						"author":  author,
						"path":    c["path"],
						"line":    line,
						"body":    c["body"],
						"created": c["created_at"],
					}
				}
				author := "unknown"
				if u, ok := r["user"].(map[string]any); ok {
					if login, ok := u["login"].(string); ok {
						author = login
					}
				}
				out[i] = map[string]any{
					"author":           author,
					"state":            state,
					"submitted":        r["submitted_at"],
					"body":             r["body"],
					"inline_comments": inlineOut,
				}
			}
			return out
		}))
	} else {
		reviewCount := 0
		if reviews, ok := pr["reviews"].([]any); ok {
			reviewCount = len(reviews)
		}
		schema = append(schema, toon.Custom("review_count", func(_ map[string]any) any {
			return fmt.Sprintf("%d — use --reviews to see full reviews", reviewCount)
		}))
	}

	detail, err := toon.RenderDetail("pull_request", pr, schema)
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(detail), nil
}

func prCreate(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	title := args.TakeFlag(&work, "--title")
	if title == "" {
		return "", errors.NewGoAIError("--title is required", "VALIDATION_ERROR")
	}
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}
	base := args.TakeFlag(&work, "--base")
	head := args.TakeFlag(&work, "--head")
	draft := args.TakeBoolFlag(&work, "--draft")
	assignee := args.TakeFlag(&work, "--assignee")
	reviewer := args.TakeFlag(&work, "--reviewer")
	labels := args.GetAllFlags(work, "--label")
	milestone := args.TakeFlag(&work, "--milestone")
	project := args.TakeFlag(&work, "--project")

	ghArgs := []string{"pr", "create", "--title", title}
	if bodyText != "" {
		ghArgs = append(ghArgs, "--body", bodyText)
	}
	if base != "" {
		ghArgs = append(ghArgs, "--base", base)
	}
	if head != "" {
		ghArgs = append(ghArgs, "--head", head)
	}
	if draft {
		ghArgs = append(ghArgs, "--draft")
	}
	if assignee != "" {
		ghArgs = append(ghArgs, "--assignee", assignee)
	}
	if reviewer != "" {
		ghArgs = append(ghArgs, "--reviewer", reviewer)
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

	stdout, err := gh.Exec(Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	numRe := regexp.MustCompile(`/pull/(\d+)`)
	var num int
	if m := numRe.FindStringSubmatch(stdout); len(m) == 2 {
		num, _ = strconv.Atoi(m[1])
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	url := ""
	if len(lines) > 0 {
		url = strings.TrimSpace(lines[len(lines)-1])
	}

	displayNum := any(num)
	if num == 0 {
		displayNum = url
	}
	detail, err := toon.RenderDetail("created", map[string]any{
		"number": displayNum,
		"url":    url,
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("url", "")})
	if err != nil {
		return "", err
	}
	id := strconv.Itoa(num)
	if num == 0 {
		id = ""
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "create", ID: id, Repo: ctx,
	})
}

func prEdit(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}
	title := args.TakeFlag(&work, "--title")
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}
	addLabel := args.TakeFlag(&work, "--add-label")
	removeLabel := args.TakeFlag(&work, "--remove-label")
	addAssignee := args.TakeFlag(&work, "--add-assignee")
	removeAssignee := args.TakeFlag(&work, "--remove-assignee")
	addReviewer := args.TakeFlag(&work, "--add-reviewer")
	removeReviewer := args.TakeFlag(&work, "--remove-reviewer")
	milestone := args.TakeFlag(&work, "--milestone")
	base := args.TakeFlag(&work, "--base")

	ghArgs := []string{"pr", "edit", strconv.Itoa(num)}
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
	if addReviewer != "" {
		ghArgs = append(ghArgs, "--add-reviewer", addReviewer)
	}
	if removeReviewer != "" {
		ghArgs = append(ghArgs, "--remove-reviewer", removeReviewer)
	}
	if milestone != "" {
		ghArgs = append(ghArgs, "--milestone", milestone)
	}
	if base != "" {
		ghArgs = append(ghArgs, "--base", base)
	}

	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("edited", map[string]any{
		"number": num,
		"status": "ok",
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("status", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "edit", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prClose(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	comment := args.TakeFlag(&work, "--comment")
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	pr, err := gh.JSON[map[string]any](Runner,
		[]string{"pr", "view", strconv.Itoa(num), "--json", "state"}, ctx)
	if err != nil {
		return "", err
	}
	state := strings.ToUpper(fmt.Sprint(pr["state"]))
	if state == "CLOSED" || state == "MERGED" {
		detail, err := toon.RenderDetail("pull_request", map[string]any{
			"number": num,
			"state":  strings.ToLower(state),
			"already": true,
		}, []toon.FieldDef{
			toon.Field("number", ""),
			toon.Field("state", ""),
			toon.Field("already", ""),
		})
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "pr", Action: "close", ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	ghArgs := []string{"pr", "close", strconv.Itoa(num)}
	if comment != "" {
		ghArgs = append(ghArgs, "--comment", comment)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("closed", map[string]any{
		"number": num,
		"status": "ok",
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("status", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "close", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prMerge(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}
	explicitMethod := args.TakeFlag(&work, "--method")
	var shorthandMethods []string
	for _, candidate := range []string{"merge", "squash", "rebase"} {
		if args.TakeBoolFlag(&work, "--"+candidate) {
			shorthandMethods = append(shorthandMethods, candidate)
		}
	}
	if len(shorthandMethods) > 1 {
		return "", errors.NewGoAIError(
			"Choose only one merge method: --merge, --squash, or --rebase",
			"VALIDATION_ERROR",
		)
	}
	if explicitMethod != "" && len(shorthandMethods) == 1 && explicitMethod != shorthandMethods[0] {
		return "", errors.NewGoAIError(
			"Choose either --method or a matching merge method shorthand, not both",
			"VALIDATION_ERROR",
		)
	}
	method := explicitMethod
	if method == "" && len(shorthandMethods) == 1 {
		method = shorthandMethods[0]
	}
	if method != "" {
		valid := false
		for _, m := range []string{"merge", "squash", "rebase"} {
			if method == m {
				valid = true
				break
			}
		}
		if !valid {
			return "", errors.NewGoAIError(
				"--method must be one of: merge, squash, rebase",
				"VALIDATION_ERROR",
			)
		}
	}
	auto := args.TakeBoolFlag(&work, "--auto")
	deleteBranch := args.TakeBoolFlag(&work, "--delete-branch")
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}
	subject := args.TakeFlag(&work, "--subject")

	pr, err := gh.JSON[map[string]any](Runner,
		[]string{"pr", "view", strconv.Itoa(num), "--json", "state,mergedBy,mergedAt"}, ctx)
	if err != nil {
		return "", err
	}
	if strings.ToUpper(fmt.Sprint(pr["state"])) == "MERGED" {
		mergedBy := any(nil)
		if mb, ok := pr["mergedBy"].(map[string]any); ok {
			mergedBy = mb["login"]
		}
		detail, err := toon.RenderDetail("pull_request", map[string]any{
			"number":    num,
			"state":     "merged",
			"merged_by": mergedBy,
			"merged_at": pr["mergedAt"],
		}, []toon.FieldDef{
			toon.Field("number", ""),
			toon.Field("state", ""),
			toon.Field("merged_by", ""),
			toon.Field("merged_at", ""),
		})
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "pr", Action: "merge", ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	ghArgs := []string{"pr", "merge", strconv.Itoa(num)}
	if method != "" {
		ghArgs = append(ghArgs, "--"+method)
	}
	if auto {
		ghArgs = append(ghArgs, "--auto")
	}
	if deleteBranch {
		ghArgs = append(ghArgs, "--delete-branch")
	}
	if bodyText != "" {
		ghArgs = append(ghArgs, "--body", bodyText)
	}
	if subject != "" {
		ghArgs = append(ghArgs, "--subject", subject)
	}

	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}

	mergeMethod := method
	if mergeMethod == "" {
		mergeMethod = "default"
	}
	detail, err := toon.RenderDetail("merged", map[string]any{
		"number": num,
		"status": "ok",
		"method": mergeMethod,
	}, []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("status", ""),
		toon.Field("method", ""),
	})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "merge", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prReview(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}
	approve := args.TakeBoolFlag(&work, "--approve")
	requestChanges := args.TakeBoolFlag(&work, "--request-changes")
	commentFlag := args.TakeBoolFlag(&work, "--comment")
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{})
	if err != nil {
		return "", err
	}

	ghArgs := []string{"pr", "review", strconv.Itoa(num)}
	if approve {
		ghArgs = append(ghArgs, "--approve")
	} else if requestChanges {
		ghArgs = append(ghArgs, "--request-changes")
	} else if commentFlag {
		ghArgs = append(ghArgs, "--comment")
	}
	if bodyText != "" {
		ghArgs = append(ghArgs, "--body", bodyText)
	}

	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}

	action := "commented"
	if approve {
		action = "approved"
	} else if requestChanges {
		action = "changes_requested"
	}
	detail, err := toon.RenderDetail("review", map[string]any{
		"number": num,
		"action": action,
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("action", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "review", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prChecks(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	pr, err := gh.JSON[map[string]any](Runner,
		[]string{"pr", "view", strconv.Itoa(num), "--json", "statusCheckRollup"}, ctx)
	if err != nil {
		return "", err
	}
	checksRaw, _ := pr["statusCheckRollup"].([]any)
	checks := make([]map[string]any, len(checksRaw))
	for i, c := range checksRaw {
		checks[i], _ = c.(map[string]any)
	}

	if len(checks) == 0 {
		enc, err := encode(map[string]any{
			"checks": "0 passed, 0 failed — this PR has no CI checks configured",
		})
		if err != nil {
			return "", err
		}
		return enc, nil
	}

	passed, failed, skipped := 0, 0, 0
	for _, c := range checks {
		switch classifyCheck(c) {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}
	pending := len(checks) - passed - failed - skipped
	summaryParts := []string{
		fmt.Sprintf("%d passed", passed),
		fmt.Sprintf("%d failed", failed),
	}
	if skipped > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d skipped", skipped))
	}
	if pending > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d pending", pending))
	}
	summaryParts = append(summaryParts, fmt.Sprintf("%d total", len(checks)))

	checksSchema := []toon.FieldDef{
		toon.Custom("name", func(c map[string]any) any {
			if name, ok := c["name"].(string); ok && name != "" {
				return name
			}
			if ctxName, ok := c["context"].(string); ok && ctxName != "" {
				return ctxName
			}
			return "check"
		}),
		toon.Custom("conclusion", func(c map[string]any) any {
			return classifyCheck(c)
		}),
	}
	summaryEnc, err := encode(map[string]any{"summary": strings.Join(summaryParts, ", ")})
	if err != nil {
		return "", err
	}
	list, err := toon.RenderList("checks", checks, checksSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{summaryEnc, list}, suggestions.Context{
		Domain: "pr", Action: "checks", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prDiff(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	full := args.TakeBoolFlag(&work, "--full")
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	diff, err := gh.Exec(Runner, []string{"pr", "diff", strconv.Itoa(num)}, ctx)
	if err != nil {
		return "", err
	}

	shouldTruncate := !full && len(diff) > diffTruncateLimit
	prDiffBlock := map[string]any{
		"number": num,
		"diff":   diff,
	}
	if shouldTruncate {
		prDiffBlock["diff"] = diff[:diffTruncateLimit]
		prDiffBlock["truncated"] = true
		prDiffBlock["original_length"] = len(diff)
	}

	sugg := suggestions.Get(suggestions.Context{
		Domain: "pr", Action: "diff", ID: strconv.Itoa(num), Repo: ctx,
	})
	if shouldTruncate {
		repoArg := ""
		if ctx != nil && ctx.Source != context.SourceGit {
			repoArg = " -R " + ctx.NWO
		}
		hint := fmt.Sprintf("Run `gai-ghcli%s pr diff %d --full` to see the complete diff", repoArg, num)
		sugg = append([]string{hint}, sugg...)
	}

	enc, err := encode(map[string]any{"pr_diff": prDiffBlock})
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(enc, toon.RenderHelp(sugg)), nil
}

func prCheckout(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	stdout, err := gh.Exec(Runner, []string{"pr", "checkout", strconv.Itoa(num)}, ctx)
	if err != nil {
		return "", err
	}
	branchRe := regexp.MustCompile(`Switched to branch '([^']+)'`)
	branch := strings.TrimSpace(stdout)
	if m := branchRe.FindStringSubmatch(stdout); len(m) == 2 {
		branch = m[1]
	}

	detail, err := toon.RenderDetail("checkout", map[string]any{
		"number": num,
		"branch": branch,
		"status": "ok",
	}, []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("branch", ""),
		toon.Field("status", ""),
	})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "checkout", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prReady(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	pr, err := gh.JSON[map[string]any](Runner,
		[]string{"pr", "view", strconv.Itoa(num), "--json", "isDraft"}, ctx)
	if err != nil {
		return "", err
	}
	isDraft, _ := pr["isDraft"].(bool)
	if !isDraft {
		detail, err := toon.RenderDetail("pull_request", map[string]any{
			"number":  num,
			"draft":   "no",
			"already": true,
		}, []toon.FieldDef{
			toon.Field("number", ""),
			toon.Field("draft", ""),
			toon.Field("already", ""),
		})
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "pr", Action: "ready", ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	if _, err := gh.Exec(Runner, []string{"pr", "ready", strconv.Itoa(num)}, ctx); err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("ready", map[string]any{
		"number": num,
		"status": "ok",
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("status", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "ready", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prReopen(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	pr, err := gh.JSON[map[string]any](Runner,
		[]string{"pr", "view", strconv.Itoa(num), "--json", "state"}, ctx)
	if err != nil {
		return "", err
	}
	if strings.ToUpper(fmt.Sprint(pr["state"])) == "OPEN" {
		detail, err := toon.RenderDetail("pull_request", map[string]any{
			"number":  num,
			"state":   "open",
			"already": true,
		}, []toon.FieldDef{
			toon.Field("number", ""),
			toon.Field("state", ""),
			toon.Field("already", ""),
		})
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "pr", Action: "reopen", ID: strconv.Itoa(num), Repo: ctx,
		})
	}

	if _, err := gh.Exec(Runner, []string{"pr", "reopen", strconv.Itoa(num)}, ctx); err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("reopened", map[string]any{
		"number": num,
		"status": "ok",
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("status", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "reopen", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prComment(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}
	bodyText, err := body.TakeBody(&work, body.TakeBodyOptions{Required: true})
	if err != nil {
		return "", err
	}

	if _, err := gh.Exec(Runner, []string{"pr", "comment", strconv.Itoa(num), "--body", bodyText}, ctx); err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("commented", map[string]any{
		"number": num,
		"status": "ok",
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("status", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "comment", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prUpdateBranch(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	if _, err := gh.Exec(Runner, []string{"pr", "update-branch", strconv.Itoa(num)}, ctx); err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("updated", map[string]any{
		"number": num,
		"status": "ok",
	}, []toon.FieldDef{toon.Field("number", ""), toon.Field("status", "")})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "update-branch", ID: strconv.Itoa(num), Repo: ctx,
	})
}

func prRevert(a []string, ctx *context.RepoContext) (string, error) {
	work := append([]string{}, a...)
	num, err := args.TakeNumber(&work, "PR")
	if err != nil {
		return "", err
	}

	result, err := gh.Raw(Runner, []string{"pr", "revert", strconv.Itoa(num)}, ctx)
	if err != nil {
		return "", err
	}
	if result.ExitCode == 0 {
		numRe := regexp.MustCompile(`/pull/(\d+)`)
		var newNum *int
		if m := numRe.FindStringSubmatch(result.Stdout); len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			newNum = &n
		}
		revertPR := any(nil)
		if newNum != nil {
			revertPR = *newNum
		}
		detail, err := toon.RenderDetail("reverted", map[string]any{
			"number":    num,
			"revert_pr": revertPR,
			"status":    "ok",
		}, []toon.FieldDef{
			toon.Field("number", ""),
			toon.Field("revert_pr", ""),
			toon.Field("status", ""),
		})
		if err != nil {
			return "", err
		}
		suggID := strconv.Itoa(num)
		if newNum != nil {
			suggID = strconv.Itoa(*newNum)
		}
		return renderWithHelp([]string{detail}, suggestions.Context{
			Domain: "pr", Action: "revert", ID: suggID, Repo: ctx,
		})
	}

	apiResult, err := gh.Raw(Runner,
		[]string{"api", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/revert", num), "--method", "POST"}, ctx)
	if err != nil {
		return "", err
	}
	if apiResult.ExitCode != 0 {
		msg := strings.TrimSpace(apiResult.Stderr)
		if msg != "" {
			msg = strings.Split(msg, "\n")[0]
		} else {
			msg = fmt.Sprintf("Failed to revert PR #%d", num)
		}
		return "", errors.NewGoAIError(msg, "UNKNOWN")
	}

	var revertData map[string]any
	if err := json.Unmarshal([]byte(apiResult.Stdout), &revertData); err != nil {
		revertData = map[string]any{}
	}

	detail, err := toon.RenderDetail("reverted", map[string]any{
		"number":    num,
		"revert_pr": revertData["number"],
		"url":       revertData["html_url"],
		"status":    "ok",
	}, []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("revert_pr", ""),
		toon.Field("url", ""),
		toon.Field("status", ""),
	})
	if err != nil {
		return "", err
	}
	suggID := strconv.Itoa(num)
	if n, ok := revertData["number"].(float64); ok {
		suggID = strconv.Itoa(int(n))
	}
	return renderWithHelp([]string{detail}, suggestions.Context{
		Domain: "pr", Action: "revert", ID: suggID, Repo: ctx,
	})
}

// PR handles pull request subcommands.
func PR(a []string, ctx *context.RepoContext) (string, error) {
	sub := ""
	if len(a) > 0 {
		sub = a[0]
	}
	rest := a[1:]

	switch sub {
	case "list":
		return prList(rest, ctx)
	case "view":
		return prView(rest, ctx)
	case "create":
		return prCreate(rest, ctx)
	case "edit":
		return prEdit(rest, ctx)
	case "close":
		return prClose(rest, ctx)
	case "merge":
		return prMerge(rest, ctx)
	case "review":
		return prReview(rest, ctx)
	case "checks":
		return prChecks(rest, ctx)
	case "diff":
		return prDiff(rest, ctx)
	case "checkout":
		return prCheckout(rest, ctx)
	case "ready":
		return prReady(rest, ctx)
	case "reopen":
		return prReopen(rest, ctx)
	case "comment":
		return prComment(rest, ctx)
	case "update-branch":
		return prUpdateBranch(rest, ctx)
	case "revert":
		return prRevert(rest, ctx)
	case "--help", "-h", "help", "":
		return PRHelp, nil
	default:
		return toon.RenderError(
			"Unknown pr subcommand: "+sub,
			"VALIDATION_ERROR",
			"Run `gai-ghcli pr --help` to see available subcommands",
		)
	}
}
