package commands

import (
	"fmt"
	"strings"

	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/secretvalue"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const SecretHelp = `usage: gai-ghcli secret <subcommand> [flags]
subcommands[3]:
  list, set <name>, delete <name>
flags{set}:
  value is read only from piped stdin; --body/-b is not accepted for secrets
values are never printed: ` + "`list`" + ` only exposes name and update time, matching ` + "`gh secret list`" + `
examples:
  gai-ghcli secret list
  echo -n "sk-..." | gai-ghcli secret set OPENAI_API_KEY
  gai-ghcli secret delete OPENAI_API_KEY`

var secretListSchema = []toon.FieldDef{
	toon.Field("name", ""),
	toon.RelativeTime("updatedAt", "updated"),
}

func hasBodyFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--body" || strings.HasPrefix(arg, "--body=") ||
			arg == "-b" || strings.HasPrefix(arg, "-b=") {
			return true
		}
	}
	return false
}

func listSecrets(_ []string, ctx *context.RepoContext) (string, error) {
	secrets, err := gh.JSON[[]map[string]any](Runner, []string{"secret", "list", "--json", "name,updatedAt"}, ctx)
	if err != nil {
		return "", err
	}
	isEmpty := len(secrets) == 0
	listOut, err := toon.RenderList("secrets", secrets, secretListSchema)
	if err != nil {
		return "", err
	}
	return renderWithHelp(
		[]string{formatCount(secrets), listOut},
		suggestions.Context{Domain: "secret", Action: "list", IsEmpty: boolPtr(isEmpty), Repo: ctx},
	)
}

func setSecret(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	remaining := cmdArgs[1:]
	if hasBodyFlag(remaining) {
		return "", errors.NewGoAIError(
			"Secret values must be piped via stdin; --body/-b is not accepted because flags are visible in process argv",
			"VALIDATION_ERROR",
			`echo -n "<value>" | gai-ghcli secret set <name>`,
		)
	}

	pos := positionals(remaining, 0)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Secret name is required: gai-ghcli secret set <name>",
			"VALIDATION_ERROR",
		)
	}
	name := pos[0]

	value, err := secretvalue.ResolveValue("", secretvalue.NounSecret)
	if err != nil {
		return "", err
	}
	if _, err := gh.ExecWithStdin(Runner, []string{"secret", "set", name}, value, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"set": "ok", "secret": name})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{
		Domain: "secret", Action: "set", ID: name, Repo: ctx,
	})
}

func deleteSecret(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 1)
	if len(pos) == 0 {
		return "", errors.NewGoAIError(
			"Secret name is required: gai-ghcli secret delete <name>",
			"VALIDATION_ERROR",
		)
	}
	name := pos[0]

	if _, err := gh.Exec(Runner, []string{"secret", "delete", name}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"delete": "ok", "secret": name})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{
		Domain: "secret", Action: "delete", ID: name, Repo: ctx,
	})
}

// Secret handles secret subcommands.
func Secret(args []string, ctx *context.RepoContext) (string, error) {
	if len(args) == 0 || args[0] == "--help" {
		return SecretHelp, nil
	}
	switch args[0] {
	case "list":
		return listSecrets(args, ctx)
	case "set":
		return setSecret(args, ctx)
	case "delete":
		return deleteSecret(args, ctx)
	default:
		return toon.RenderError("Unknown subcommand: "+args[0], "VALIDATION_ERROR",
			"Available subcommands: list, set, delete")
	}
}

func formatCount(items []map[string]any) string {
	return fmt.Sprintf("count: %d", len(items))
}
