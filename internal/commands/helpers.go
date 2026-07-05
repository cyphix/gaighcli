package commands

import (
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

// Runner is the gh client used by commands.
var Runner gh.Runner = gh.Default

func boolPtr(v bool) *bool { return &v }

func renderWithHelp(blocks []string, sugg suggestions.Context) (string, error) {
	lines := suggestions.Get(sugg)
	help := toon.RenderHelp(lines)
	return toon.RenderOutput(append(blocks, help)...), nil
}

func encode(v any) (string, error) {
	return toon.Encode(v)
}

func repoCtx(ctx *context.RepoContext) *context.RepoContext {
	return ctx
}
