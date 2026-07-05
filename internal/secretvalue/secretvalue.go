package secretvalue

import (
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/stdin"
)

// Noun is secret or variable.
type Noun string

const (
	NounSecret   Noun = "secret"
	NounVariable Noun = "variable"
)

func valueRequiredError(noun Noun) error {
	if noun == NounSecret {
		return errors.NewGoAIError(
			"secret value is required: pipe the value via stdin",
			"VALIDATION_ERROR",
			`echo -n "<value>" | gai-ghcli secret set <name>`,
		)
	}
	return errors.NewGoAIError(
		"variable value is required: pass --body <value> or pipe the value via stdin",
		"VALIDATION_ERROR",
		"gai-ghcli variable set <name> --body <value>",
		`echo -n "<value>" | gai-ghcli variable set <name>`,
	)
}

// ResolveValue resolves a secret/variable value from flag or stdin.
func ResolveValue(flagValue string, noun Noun) (string, error) {
	if flagValue != "" {
		if noun == NounSecret {
			return "", errors.NewGoAIError(
				"Secret values must be piped via stdin; --body/-b is not accepted for secrets",
				"VALIDATION_ERROR",
				`echo -n "<value>" | gai-ghcli secret set <name>`,
			)
		}
		if flagValue == "" {
			return "", errors.NewGoAIError("--body requires a value", "VALIDATION_ERROR",
				"gai-ghcli variable set <name> --body <value>")
		}
		return flagValue, nil
	}
	if stdin.IsTTY() {
		return "", valueRequiredError(noun)
	}
	value, err := stdin.ReadAll()
	if err != nil {
		return "", err
	}
	if len(value) == 0 {
		return "", valueRequiredError(noun)
	}
	return value, nil
}
