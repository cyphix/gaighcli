package suggestions

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cyphix/gaighcli/internal/context"
)

// Context holds suggestion lookup parameters.
type Context struct {
	Domain  string
	Action  string
	State   string
	IsEmpty *bool
	ID      string
	Repo    *context.RepoContext
}

func repoFlag(ctx Context) string {
	if ctx.Repo != nil && ctx.Repo.Source != context.SourceGit {
		return " -R " + ctx.Repo.NWO
	}
	return ""
}

var normalizeRe = regexp.MustCompile("`gai-ghcli -R ([^`\\s]+) ([^`]+)`")

func normalizeRepoFlagLine(line string) string {
	return normalizeRe.ReplaceAllString(line, "`gai-ghcli $2 -R $1`")
}

type entry struct {
	match func(Context) bool
	templates []string
	useCtx bool
}

var table = []entry{
	{
		match: func(ctx Context) bool { return ctx.Domain == "home" },
		templates: []string{
			"gai-ghcli <command> <subcommand>",
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "list" && (ctx.IsEmpty == nil || !*ctx.IsEmpty) },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue view <number>",
			"gai-ghcli${repoFlag(c)} issue create --title \"...\" --body-file <path>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "list" && ctx.IsEmpty != nil && *ctx.IsEmpty },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue create --title \"...\" --body-file <path>",
			"gai-ghcli${repoFlag(c)} issue list --state closed",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "view" && ctx.State == "open" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue comment ${c.id} --body-file <path>",
			"gai-ghcli${repoFlag(c)} issue close ${c.id}",
			"gai-ghcli${repoFlag(c)} issue edit ${c.id} --add-assignee <user>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "view" && ctx.State == "closed" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue reopen ${c.id}",
			"gai-ghcli${repoFlag(c)} issue comment ${c.id} --body-file <path>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "create" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue view ${c.id}",
			"gai-ghcli${repoFlag(c)} issue edit ${c.id} --add-label <label>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "close" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue reopen ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "reopen" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue close ${c.id}",
			"gai-ghcli${repoFlag(c)} issue view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "edit" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "comment" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue view ${c.id} --comments",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "delete" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool {
			return ctx.Domain == "issue" &&
				slices.Contains([]string{"lock", "unlock", "pin", "unpin"}, ctx.Action)
		},
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "issue" && ctx.Action == "transfer" },
		templates: []string{
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "list" && (ctx.IsEmpty == nil || !*ctx.IsEmpty) },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view <number>",
			"gai-ghcli${repoFlag(c)} pr create --title \"...\" --body-file <path>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "list" && ctx.IsEmpty != nil && *ctx.IsEmpty },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr create --title \"...\" --body-file <path>",
			"gai-ghcli${repoFlag(c)} pr list --state closed",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "view" && ctx.State == "open" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr checks ${c.id}",
			"gai-ghcli${repoFlag(c)} pr review ${c.id} --approve",
			"gai-ghcli${repoFlag(c)} pr merge ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "view" && ctx.State == "closed" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr reopen ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "view" && ctx.State == "merged" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr revert ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "create" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view ${c.id}",
			"gai-ghcli${repoFlag(c)} pr checks ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "close" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr reopen ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "merge" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr revert ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "review" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "checks" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view ${c.id}",
			"gai-ghcli${repoFlag(c)} pr merge ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "diff" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr review ${c.id} --approve",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "checkout" },
		templates: []string{
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "ready" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "reopen" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "comment" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view ${c.id} --comments",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "update-branch" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr checks ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "pr" && ctx.Action == "revert" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} pr view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "list" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run view <id>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "view" && ctx.State == "completed" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run rerun ${c.id}",
			"gai-ghcli${repoFlag(c)} run view ${c.id} --log-failed",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "view" && ctx.State == "in_progress" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run watch ${c.id}",
			"gai-ghcli${repoFlag(c)} run cancel ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "view" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run view ${c.id} --log",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "rerun" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run watch ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "cancel" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "delete" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "watch" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "run" && ctx.Action == "download" },
		templates: []string{
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "workflow" && ctx.Action == "list" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} workflow view <id>",
			"gai-ghcli${repoFlag(c)} workflow run <id>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "workflow" && ctx.Action == "view" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} workflow run ${c.id}",
			"gai-ghcli${repoFlag(c)} run list --workflow ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "workflow" && ctx.Action == "run" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} run list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool {
			return ctx.Domain == "workflow" && slices.Contains([]string{"enable", "disable"}, ctx.Action)
		},
		templates: []string{
			"gai-ghcli${repoFlag(c)} workflow list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "release" && ctx.Action == "list" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} release view <tag>",
			"gai-ghcli${repoFlag(c)} release create <tag> --body-file <path>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "release" && ctx.Action == "view" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} release download ${c.id}",
			"gai-ghcli${repoFlag(c)} release edit ${c.id} --body-file <path>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "release" && ctx.Action == "create" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} release view ${c.id}",
			"gai-ghcli${repoFlag(c)} release upload ${c.id} <files...>",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "release" && ctx.Action == "edit" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} release view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "release" && ctx.Action == "delete" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} release list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "release" && ctx.Action == "download" },
		templates: []string{
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "release" && ctx.Action == "upload" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} release view ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "repo" && ctx.Action == "view" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} issue list",
			"gai-ghcli${repoFlag(c)} pr list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "repo" && ctx.Action == "create" },
		templates: []string{
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "repo" && ctx.Action == "list" },
		templates: []string{
			"gai-ghcli repo view -R <owner/name>",
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool {
			return ctx.Domain == "repo" && slices.Contains([]string{"edit", "clone", "fork"}, ctx.Action)
		},
		templates: []string{
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "label" && ctx.Action == "list" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} label create --name \"...\" --color \"...\"",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "label" && ctx.Action == "create" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} label list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "label" && ctx.Action == "edit" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} label list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "label" && ctx.Action == "delete" },
		templates: []string{
			"gai-ghcli${repoFlag(c)} label list",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "project" && ctx.Action == "list" && (ctx.IsEmpty == nil || !*ctx.IsEmpty) },
		templates: []string{
			"gai-ghcli project view <number>",
			"gai-ghcli project item-list <number>",
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "project" && ctx.Action == "list" && ctx.IsEmpty != nil && *ctx.IsEmpty },
		templates: []string{
			"gai-ghcli project create --title \"...\"",
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "project" && ctx.Action == "view" },
		templates: []string{
			"gai-ghcli project item-list ${c.id}",
			"gai-ghcli project field-list ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "project" && ctx.Action == "create" },
		templates: []string{
			"gai-ghcli project view ${c.id}",
			"gai-ghcli project item-list ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "project" && ctx.Action == "item-list" && (ctx.IsEmpty == nil || !*ctx.IsEmpty) },
		templates: []string{
			"gai-ghcli project item-add ${c.id} --url <issue-or-pr-url>",
			"gai-ghcli project field-list ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "project" && ctx.Action == "item-add" },
		templates: []string{
			"gai-ghcli project item-list ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "project" && ctx.Action == "field-list" },
		templates: []string{
			"gai-ghcli project item-list ${c.id}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "secret" && ctx.Action == "list" && (ctx.IsEmpty == nil || !*ctx.IsEmpty) },
		templates: []string{
			"echo -n \"<value>\" | gai-ghcli secret set <name>${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "secret" && ctx.Action == "list" && ctx.IsEmpty != nil && *ctx.IsEmpty },
		templates: []string{
			"echo -n \"<value>\" | gai-ghcli secret set <name>${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "secret" && ctx.Action == "set" },
		templates: []string{
			"gai-ghcli secret list${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "secret" && ctx.Action == "delete" },
		templates: []string{
			"gai-ghcli secret list${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "variable" && ctx.Action == "list" && (ctx.IsEmpty == nil || !*ctx.IsEmpty) },
		templates: []string{
			"gai-ghcli variable set <name> --body <value>${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "variable" && ctx.Action == "list" && ctx.IsEmpty != nil && *ctx.IsEmpty },
		templates: []string{
			"gai-ghcli variable set <name> --body <value>${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "variable" && ctx.Action == "set" },
		templates: []string{
			"gai-ghcli variable list${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "variable" && ctx.Action == "delete" },
		templates: []string{
			"gai-ghcli variable list${repoFlag(c)}",
		},
		useCtx: true,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "search" },
		templates: []string{
		},
		useCtx: false,
	},
	{
		match: func(ctx Context) bool { return ctx.Domain == "api" },
		templates: []string{
		},
		useCtx: false,
	},
}

func expandLine(tmpl string, ctx Context) string {
	s := strings.ReplaceAll(tmpl, "${repoFlag(c)}", repoFlag(ctx))
	s = strings.ReplaceAll(s, "${c.id}", ctx.ID)
	if ctx.Repo != nil {
		s = strings.ReplaceAll(s, "${c.repo ? ` --repo ${c.repo.nwo}` : \"\"}", " --repo "+ctx.Repo.NWO)
	} else {
		s = strings.ReplaceAll(s, "${c.repo ? ` --repo ${c.repo.nwo}` : \"\"}", "")
	}
	return s
}

// Get returns contextual suggestion lines for a command result.
func Get(ctx Context) []string {
	for _, e := range table {
		if e.match(ctx) {
			var lines []string
			for _, tmpl := range e.templates {
				line := tmpl
				if e.useCtx {
					line = expandLine(tmpl, ctx)
				}
				lines = append(lines, "Run `"+line+"`")
			}
			out := make([]string, len(lines))
			for i, l := range lines {
				out[i] = normalizeRepoFlagLine(l)
			}
			return out
		}
	}
	return nil
}

