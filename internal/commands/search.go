package commands

import (
	"strconv"
	"strings"
	"time"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/format"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const SearchHelp = `usage: gai-ghcli search <type> <query> [flags]
types[5]:
  issues, prs, repos, commits, code
flags{common}:
  --repo, --owner, --state, --label, --assignee, --author, --sort, --limit <n> (default 1000)
flags{prs}:
  --draft, --review
flags{repos}:
  --language, --stars (e.g. ">100")
examples:
  gai-ghcli search issues "login bug" --repo octo/repo --state open
  gai-ghcli search prs "feat" --author alice --sort updated
  gai-ghcli search repos "cli tool" --language Go --stars ">50"`

const (
	defaultSearchLimit = "1000"
	displayLimit       = 30
)

var searchValueFlags = map[string]bool{
	"--repo":     true,
	"--owner":    true,
	"--state":    true,
	"--label":    true,
	"--assignee": true,
	"--author":   true,
	"--sort":     true,
	"--limit":    true,
	"--review":   true,
	"--language": true,
	"--stars":    true,
}

var commonFilterFlags = []string{
	"--repo", "--owner", "--state", "--label", "--assignee", "--author",
}

var (
	issueSearchSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Pluck("repository", "nameWithOwner", "repo"),
		toon.Lower("state", ""),
		toon.Pluck("author", "login", "author"),
		toon.JoinArray("labels", "name", "labels", "none"),
		toon.RelativeTime("createdAt", "created"),
	}
	prSearchSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Pluck("repository", "nameWithOwner", "repo"),
		toon.Lower("state", ""),
		toon.Pluck("author", "login", "author"),
		toon.RelativeTime("createdAt", "created"),
	}
	repoSearchSchema = []toon.FieldDef{
		toon.Field("fullName", "name"),
		toon.Field("description", ""),
		toon.Field("stargazersCount", "stars"),
		toon.Field("forksCount", "forks"),
		toon.Field("language", ""),
		toon.RelativeTime("updatedAt", "updated"),
	}
	commitSearchSchema = []toon.FieldDef{
		toon.Field("sha", ""),
		toon.Custom("message", func(item map[string]any) any {
			commit, _ := item["commit"].(map[string]any)
			if commit == nil {
				return ""
			}
			message, _ := commit["message"].(string)
			if message == "" {
				return ""
			}
			if idx := strings.IndexByte(message, '\n'); idx >= 0 {
				return message[:idx]
			}
			return message
		}),
		toon.Pluck("repository", "fullName", "repo"),
		toon.Pluck("author", "login", "author"),
		toon.Custom("date", func(item map[string]any) any {
			commit, _ := item["commit"].(map[string]any)
			if commit == nil {
				return "unknown"
			}
			author, _ := commit["author"].(map[string]any)
			if author == nil {
				return "unknown"
			}
			d, _ := author["date"].(string)
			if d == "" {
				return "unknown"
			}
			then, err := time.Parse(time.RFC3339, d)
			if err != nil {
				return "unknown"
			}
			diffH := int(time.Since(then).Hours())
			if diffH < 1 {
				return "just now"
			}
			if diffH < 24 {
				return strconv.Itoa(diffH) + "h ago"
			}
			return strconv.Itoa(diffH/24) + "d ago"
		}),
	}
	codeSearchSchema = []toon.FieldDef{
		toon.Field("path", ""),
		toon.Pluck("repository", "fullName", "repo"),
		toon.Custom("matches", func(item map[string]any) any {
			tm, _ := item["textMatches"].([]any)
			if len(tm) == 0 {
				return 0
			}
			return len(tm)
		}),
	}
)

func extractSearchQuery(cmdArgs []string) string {
	var positionals []string
	i := 1
	for i < len(cmdArgs) {
		arg := cmdArgs[i]
		if strings.HasPrefix(arg, "--") {
			if strings.Contains(arg, "=") || !searchValueFlags[arg] {
				i++
			} else {
				i += 2
			}
		} else {
			positionals = append(positionals, arg)
			i++
		}
	}
	return strings.Join(positionals, " ")
}

func getSearchRepo(cmdArgs []string, ctx *context.RepoContext) string {
	if repo := args.GetFlag(cmdArgs, "--repo"); repo != "" {
		return repo
	}
	if ctx != nil {
		return ctx.NWO
	}
	return ""
}

func hasSearchFilters(cmdArgs []string, extraFlags ...string) bool {
	flags := append(append([]string{}, commonFilterFlags...), extraFlags...)
	for _, f := range flags {
		if args.HasFlag(cmdArgs, f) || args.GetFlag(cmdArgs, f) != "" {
			return true
		}
	}
	return false
}

func appendSearchCommonFlags(ghArgs []string, cmdArgs []string, ctx *context.RepoContext, includeRepo bool) []string {
	if includeRepo {
		if repo := getSearchRepo(cmdArgs, ctx); repo != "" {
			ghArgs = append(ghArgs, "--repo", repo)
		}
	}
	if owner := args.GetFlag(cmdArgs, "--owner"); owner != "" {
		ghArgs = append(ghArgs, "--owner", owner)
	}
	if state := args.GetFlag(cmdArgs, "--state"); state != "" {
		ghArgs = append(ghArgs, "--state", state)
	}
	if label := args.GetFlag(cmdArgs, "--label"); label != "" {
		ghArgs = append(ghArgs, "--label", label)
	}
	if assignee := args.GetFlag(cmdArgs, "--assignee"); assignee != "" {
		ghArgs = append(ghArgs, "--assignee", assignee)
	}
	if author := args.GetFlag(cmdArgs, "--author"); author != "" {
		ghArgs = append(ghArgs, "--author", author)
	}
	if sort := args.GetFlag(cmdArgs, "--sort"); sort != "" {
		ghArgs = append(ghArgs, "--sort", sort)
	}
	return ghArgs
}

func renderSearchResults(
	label string,
	results []map[string]any,
	limit string,
	schema []toon.FieldDef,
	sugg suggestions.Context,
) (string, error) {
	limitNum, _ := strconv.Atoi(limit)
	displayed := results
	if len(displayed) > displayLimit {
		displayed = displayed[:displayLimit]
	}
	dl := displayLimit
	countLine := format.CountLine(format.CountLineOptions{
		Count:        len(results),
		Limit:        &limitNum,
		APILimitHit:  len(results) == limitNum,
		DisplayLimit: &dl,
	})
	listOut, err := toon.RenderList(label, displayed, schema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{countLine, listOut}, sugg)
}

func searchIssues(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	query := extractSearchQuery(cmdArgs)
	if query == "" && !hasSearchFilters(cmdArgs) {
		return "", errors.NewGoAIError(
			"Search query or filters required: gai-ghcli search issues <query> [--assignee x] [--state open] ...",
			"VALIDATION_ERROR",
		)
	}
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = defaultSearchLimit
	}
	ghArgs := []string{"search", "issues"}
	if query != "" {
		ghArgs = append(ghArgs, query)
	}
	ghArgs = append(ghArgs,
		"--json", "number,title,repository,state,author,labels,createdAt",
		"--limit", limit,
	)
	ghArgs = appendSearchCommonFlags(ghArgs, cmdArgs, ctx, true)

	results, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	return renderSearchResults("issues", results, limit, issueSearchSchema, suggestions.Context{
		Domain: "search", Action: "issues", Repo: ctx,
	})
}

func searchPRs(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	query := extractSearchQuery(cmdArgs)
	if query == "" && !hasSearchFilters(cmdArgs, "--draft", "--review") {
		return "", errors.NewGoAIError(
			"Search query or filters required: gai-ghcli search prs <query> [--assignee x] [--state open] ...",
			"VALIDATION_ERROR",
		)
	}
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = defaultSearchLimit
	}
	ghArgs := []string{"search", "prs"}
	if query != "" {
		ghArgs = append(ghArgs, query)
	}
	ghArgs = append(ghArgs,
		"--json", "number,title,repository,state,author,createdAt",
		"--limit", limit,
	)
	ghArgs = appendSearchCommonFlags(ghArgs, cmdArgs, ctx, true)
	if args.HasFlag(cmdArgs, "--draft") {
		ghArgs = append(ghArgs, "--draft")
	}
	if review := args.GetFlag(cmdArgs, "--review"); review != "" {
		ghArgs = append(ghArgs, "--review", review)
	}

	results, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	return renderSearchResults("prs", results, limit, prSearchSchema, suggestions.Context{
		Domain: "search", Action: "prs", Repo: ctx,
	})
}

func searchRepos(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	query := extractSearchQuery(cmdArgs)
	if query == "" && !hasSearchFilters(cmdArgs, "--language", "--stars") {
		return "", errors.NewGoAIError(
			"Search query or filters required: gai-ghcli search repos <query> [--owner x] [--language y] ...",
			"VALIDATION_ERROR",
		)
	}
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = defaultSearchLimit
	}
	ghArgs := []string{"search", "repos"}
	if query != "" {
		ghArgs = append(ghArgs, query)
	}
	ghArgs = append(ghArgs,
		"--json", "fullName,description,stargazersCount,forksCount,language,updatedAt",
		"--limit", limit,
	)
	if owner := args.GetFlag(cmdArgs, "--owner"); owner != "" {
		ghArgs = append(ghArgs, "--owner", owner)
	}
	if language := args.GetFlag(cmdArgs, "--language"); language != "" {
		ghArgs = append(ghArgs, "--language", language)
	}
	if stars := args.GetFlag(cmdArgs, "--stars"); stars != "" {
		ghArgs = append(ghArgs, "--stars", stars)
	}
	if sort := args.GetFlag(cmdArgs, "--sort"); sort != "" {
		ghArgs = append(ghArgs, "--sort", sort)
	}

	results, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	return renderSearchResults("repos", results, limit, repoSearchSchema, suggestions.Context{
		Domain: "search", Action: "repos", Repo: ctx,
	})
}

func searchCommits(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	query := extractSearchQuery(cmdArgs)
	if query == "" && !hasSearchFilters(cmdArgs) {
		return "", errors.NewGoAIError(
			"Search query or filters required: gai-ghcli search commits <query> [--repo x] [--author y] ...",
			"VALIDATION_ERROR",
		)
	}
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = defaultSearchLimit
	}
	ghArgs := []string{"search", "commits"}
	if query != "" {
		ghArgs = append(ghArgs, query)
	}
	ghArgs = append(ghArgs, "--json", "sha,commit,repository,author", "--limit", limit)
	ghArgs = appendSearchCommonFlags(ghArgs, cmdArgs, ctx, true)

	results, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	return renderSearchResults("commits", results, limit, commitSearchSchema, suggestions.Context{
		Domain: "search", Action: "commits", Repo: ctx,
	})
}

func searchCode(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	query := extractSearchQuery(cmdArgs)
	if query == "" && !hasSearchFilters(cmdArgs, "--language") {
		return "", errors.NewGoAIError(
			"Search query or filters required: gai-ghcli search code <query> [--repo x] [--language y] ...",
			"VALIDATION_ERROR",
		)
	}
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = defaultSearchLimit
	}
	ghArgs := []string{"search", "code"}
	if query != "" {
		ghArgs = append(ghArgs, query)
	}
	ghArgs = append(ghArgs, "--json", "path,repository,textMatches", "--limit", limit)
	if repo := getSearchRepo(cmdArgs, ctx); repo != "" {
		ghArgs = append(ghArgs, "--repo", repo)
	}
	if owner := args.GetFlag(cmdArgs, "--owner"); owner != "" {
		ghArgs = append(ghArgs, "--owner", owner)
	}
	if language := args.GetFlag(cmdArgs, "--language"); language != "" {
		ghArgs = append(ghArgs, "--language", language)
	}

	results, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	return renderSearchResults("results", results, limit, codeSearchSchema, suggestions.Context{
		Domain: "search", Action: "code", Repo: ctx,
	})
}

// Search handles search subcommands.
func Search(args []string, ctx *context.RepoContext) (string, error) {
	if len(args) == 0 || args[0] == "--help" {
		return SearchHelp, nil
	}
	switch args[0] {
	case "issues":
		return searchIssues(args, ctx)
	case "prs":
		return searchPRs(args, ctx)
	case "repos":
		return searchRepos(args, ctx)
	case "commits":
		return searchCommits(args, ctx)
	case "code":
		return searchCode(args, ctx)
	default:
		return toon.RenderError("Unknown search type: "+args[0], "VALIDATION_ERROR",
			"Available types: issues, prs, repos, commits, code")
	}
}
