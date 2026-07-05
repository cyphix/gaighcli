package commands

import (
	"strconv"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/format"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const RepoHelp = `usage: gai-ghcli repo <subcommand> [flags]
subcommands[6]:
  view, create <name>, edit, clone <repo>, fork [repo], list [owner]
flags{create}:
  --public, --private, --internal, --description, --clone, --template
flags{edit}:
  --description, --visibility, --default-branch, --enable-issues, --enable-wiki
flags{fork}:
  --clone, --remote
flags{list}:
  --limit <n> (default 30), --visibility, --language, --archived
examples:
  gai-ghcli repo view
  gai-ghcli repo create my-project --public --description "A new project"
  gai-ghcli repo list --visibility public --language TypeScript`

var (
	repoViewSchema = []toon.FieldDef{
		toon.Field("name", ""),
		toon.Field("description", ""),
		toon.Pluck("defaultBranchRef", "name", "branch"),
		toon.Field("stargazerCount", "stars"),
		toon.Field("forkCount", "forks"),
		toon.Custom("issues", func(item map[string]any) any {
			if issues, ok := item["issues"].(map[string]any); ok {
				if tc, ok := issues["totalCount"]; ok {
					return tc
				}
			}
			return 0
		}),
		toon.Custom("prs", func(item map[string]any) any {
			if prs, ok := item["pullRequests"].(map[string]any); ok {
				if tc, ok := prs["totalCount"]; ok {
					return tc
				}
			}
			return 0
		}),
		toon.Lower("visibility", ""),
		toon.Pluck("primaryLanguage", "name", "language"),
	}
	repoListSchema = []toon.FieldDef{
		toon.Field("name", ""),
		toon.Field("description", ""),
		toon.Lower("visibility", ""),
		toon.Pluck("primaryLanguage", "name", "language"),
		toon.Field("stargazerCount", "stars"),
		toon.RelativeTime("updatedAt", "updated"),
	}
)

func viewRepo(_ []string, ctx *context.RepoContext) (string, error) {
	ghArgs := []string{"repo", "view"}
	if ctx != nil {
		ghArgs = append(ghArgs, ctx.NWO)
	}
	ghArgs = append(ghArgs, "--json", "name,description,defaultBranchRef,stargazerCount,forkCount,issues,pullRequests,visibility,primaryLanguage")

	repo, err := gh.JSON[map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}
	detail, err := toon.RenderDetail("repo", repo, repoViewSchema)
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(detail), nil
}

func createRepo(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 1)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Repository name is required: gai-ghcli repo create <name>",
			"VALIDATION_ERROR",
		)
	}
	name := pos[0]

	ghArgs := []string{"repo", "create", name}
	if args.HasFlag(cmdArgs, "--public") {
		ghArgs = append(ghArgs, "--public")
	} else if args.HasFlag(cmdArgs, "--private") {
		ghArgs = append(ghArgs, "--private")
	} else if args.HasFlag(cmdArgs, "--internal") {
		ghArgs = append(ghArgs, "--internal")
	}
	if description := args.GetFlag(cmdArgs, "--description"); description != "" {
		ghArgs = append(ghArgs, "--description", description)
	}
	if args.HasFlag(cmdArgs, "--clone") {
		ghArgs = append(ghArgs, "--clone")
	}
	if template := args.GetFlag(cmdArgs, "--template"); template != "" {
		ghArgs = append(ghArgs, "--template", template)
	}
	if _, err := gh.Exec(Runner, ghArgs, nil); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"created": "ok", "repo": name})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "repo", Action: "create", Repo: ctx})
}

func editRepo(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	ghArgs := []string{"repo", "edit"}
	if ctx != nil && ctx.Source != context.SourceGit {
		ghArgs = append(ghArgs, ctx.NWO)
	}
	if description := args.GetFlag(cmdArgs, "--description"); description != "" {
		ghArgs = append(ghArgs, "--description", description)
	}
	if visibility := args.GetFlag(cmdArgs, "--visibility"); visibility != "" {
		ghArgs = append(ghArgs, "--visibility", visibility)
	}
	if defaultBranch := args.GetFlag(cmdArgs, "--default-branch"); defaultBranch != "" {
		ghArgs = append(ghArgs, "--default-branch", defaultBranch)
	}
	if enableIssues := args.GetFlag(cmdArgs, "--enable-issues"); enableIssues != "" {
		ghArgs = append(ghArgs, "--enable-issues="+enableIssues)
	}
	if enableWiki := args.GetFlag(cmdArgs, "--enable-wiki"); enableWiki != "" {
		ghArgs = append(ghArgs, "--enable-wiki="+enableWiki)
	}
	if _, err := gh.Exec(Runner, ghArgs, nil); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"edit": "ok"})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "repo", Action: "edit", Repo: ctx})
}

func cloneRepo(cmdArgs []string) (string, error) {
	pos := positionals(cmdArgs, 1)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Repository is required: gai-ghcli repo clone <repo>",
			"VALIDATION_ERROR",
		)
	}
	repo := pos[0]

	if _, err := gh.Exec(Runner, []string{"repo", "clone", repo}, nil); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"clone": "ok", "repo": repo})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "repo", Action: "clone"})
}

func forkRepo(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 1)
	var repo string
	if len(pos) > 0 {
		repo = pos[0]
	}

	ghArgs := []string{"repo", "fork"}
	if repo != "" {
		ghArgs = append(ghArgs, repo)
	}
	if args.HasFlag(cmdArgs, "--clone") {
		ghArgs = append(ghArgs, "--clone")
	}
	if args.HasFlag(cmdArgs, "--remote") {
		ghArgs = append(ghArgs, "--remote")
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}

	outRepo := repo
	if outRepo == "" {
		if ctx != nil {
			outRepo = ctx.NWO
		} else {
			outRepo = "current"
		}
	}
	enc, err := encode(map[string]any{"fork": "ok", "repo": outRepo})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "repo", Action: "fork", Repo: ctx})
}

func listRepos(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 1)
	var owner string
	if len(pos) > 0 {
		owner = pos[0]
	}

	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = "30"
	}
	ghArgs := []string{"repo", "list", "--json", "name,description,visibility,primaryLanguage,stargazerCount,updatedAt", "--limit", limit}
	if owner != "" {
		ghArgs = append(ghArgs[:2], append([]string{owner}, ghArgs[2:]...)...)
	}
	if visibility := args.GetFlag(cmdArgs, "--visibility"); visibility != "" {
		ghArgs = append(ghArgs, "--visibility", visibility)
	}
	if language := args.GetFlag(cmdArgs, "--language"); language != "" {
		ghArgs = append(ghArgs, "--language", language)
	}
	if args.HasFlag(cmdArgs, "--archived") {
		ghArgs = append(ghArgs, "--archived")
	}

	repos, err := gh.JSON[[]map[string]any](Runner, ghArgs, nil)
	if err != nil {
		return "", err
	}

	isEmpty := len(repos) == 0
	limitNum, _ := strconv.Atoi(limit)
	countLine := format.CountLine(format.CountLineOptions{Count: len(repos), Limit: &limitNum})
	listOut, err := toon.RenderList("repos", repos, repoListSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{countLine, listOut}, suggestions.Context{
		Domain: "repo", Action: "list", IsEmpty: boolPtr(isEmpty), Repo: ctx,
	})
}

// Repo handles repo subcommands.
func Repo(args []string, ctx *context.RepoContext) (string, error) {
	if len(args) == 0 || args[0] == "--help" {
		return RepoHelp, nil
	}
	switch args[0] {
	case "view":
		return viewRepo(args, ctx)
	case "create":
		return createRepo(args, ctx)
	case "edit":
		return editRepo(args, ctx)
	case "clone":
		return cloneRepo(args)
	case "fork":
		return forkRepo(args, ctx)
	case "list":
		return listRepos(args, ctx)
	default:
		return toon.RenderError("Unknown subcommand: "+args[0], "VALIDATION_ERROR",
			"Available subcommands: view, create, edit, clone, fork, list")
	}
}
