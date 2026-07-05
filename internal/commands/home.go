package commands

import (
	"strings"
	"sync"

	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const HomeHelp = ""

var (
	homeIssueSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Lower("state", ""),
		toon.Pluck("author", "login", "author"),
	}
	homePRSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("title", ""),
		toon.Pluck("author", "login", "author"),
		toon.MapEnum("reviewDecision", map[string]string{
			"APPROVED":          "approved",
			"CHANGES_REQUESTED": "changes_requested",
			"REVIEW_REQUIRED":   "required",
		}, "none", "review"),
	}
)

// Home renders the dashboard view.
func Home(_ []string, ctx *context.RepoContext) (string, error) {
	var issues, prs []map[string]any
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		items, err := gh.JSON[[]map[string]any](Runner,
			[]string{"issue", "list", "--json", "number,title,state,author", "--limit", "3"}, ctx)
		if err == nil {
			issues = items
		}
	}()
	go func() {
		defer wg.Done()
		items, err := gh.JSON[[]map[string]any](Runner,
			[]string{"pr", "list", "--json", "number,title,author,reviewDecision", "--limit", "3"}, ctx)
		if err == nil {
			prs = items
		}
	}()
	wg.Wait()

	var blocks []string
	if ctx != nil {
		enc, _ := encode(map[string]any{"repo": ctx.NWO})
		blocks = append(blocks, enc)
	}
	if len(issues) > 0 {
		list, _ := toon.RenderList("issues", issues, homeIssueSchema)
		blocks = append(blocks, list)
	} else {
		blocks = append(blocks, "issues: 0 open")
	}
	if len(prs) > 0 {
		list, _ := toon.RenderList("prs", prs, homePRSchema)
		blocks = append(blocks, list)
	} else {
		blocks = append(blocks, "prs: 0 open")
	}

	var hints []string
	if len(issues) >= 3 {
		hints = append(hints, "Run `gai-ghcli issue list` for full issue list")
	}
	if len(prs) >= 3 {
		hints = append(hints, "Run `gai-ghcli pr list` for full PR list")
	}
	sugg := suggestions.Get(suggestions.Context{Domain: "home", Action: "home", Repo: ctx})
	all := append(hints, sugg...)
	blocks = append(blocks, toon.RenderHelp(all))
	return toon.RenderOutput(blocks...), nil
}

// Setup handles setup subcommands.
func Setup(args []string, _ *context.RepoContext) (string, error) {
	if len(args) != 1 || args[0] != "hooks" {
		return toon.RenderError("Unknown setup action", "VALIDATION_ERROR", "Run `gai-ghcli setup hooks`")
	}
	// Hook install is done from main via gaisdk - caller handles this
	return toon.RenderOutput(
		"hooks:\n  status: installed\n  integrations: Claude Code, Codex, Cursor",
		toon.RenderHelp([]string{"Restart your agent session to receive gai-ghcli ambient context"}),
	), nil
}

func positionals(args []string, skip int) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	if len(out) > skip {
		return out[skip:]
	}
	return nil
}
