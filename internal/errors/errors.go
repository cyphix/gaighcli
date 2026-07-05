package errors

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cyphix/gaisdk"
)

// NewGoAIError is a convenience alias for structured errors.
func NewGoAIError(message, code string, suggestions ...string) *gaisdk.GoAIError {
	return gaisdk.NewGoAIError(message, code, suggestions...)
}

type errorPattern struct {
	pattern     *regexp.Regexp
	code        string
	message     func(match []string, stderr string) string
	suggestions func(match []string) []string
}

var patterns = []errorPattern{
	{
		pattern: regexp.MustCompile(`Could not resolve to a Repository with the name '([^']+)'`),
		code:    "REPO_NOT_FOUND",
		message: func(m []string, _ string) string { return `Repository "` + m[1] + `" not found` },
		suggestions: func(_ []string) []string {
			return []string{"Run `gai-ghcli repo list` to see your repositories"}
		},
	},
	{
		pattern: regexp.MustCompile(`Could not resolve to an? .+? with the number of (\d+)`),
		code:    "NOT_FOUND",
		message: func(m []string, _ string) string { return "Item #" + m[1] + " does not exist in this repository" },
	},
	{
		pattern: regexp.MustCompile(`(?i)issue (\d+) not found`),
		code:    "NOT_FOUND",
		message: func(m []string, _ string) string { return "Issue #" + m[1] + " does not exist" },
	},
	{
		pattern: regexp.MustCompile(`(?i)pull request (\d+) not found`),
		code:    "NOT_FOUND",
		message: func(m []string, _ string) string { return "Pull request #" + m[1] + " does not exist" },
	},
	{
		pattern: regexp.MustCompile(`release with tag "([^"]+)" not found`),
		code:    "NOT_FOUND",
		message: func(m []string, _ string) string { return `Release "` + m[1] + `" not found` },
		suggestions: func(_ []string) []string {
			return []string{"Run `gai-ghcli release list` to see available releases"}
		},
	},
	{
		pattern: regexp.MustCompile(`(?i)run (\d+) not found`),
		code:    "NOT_FOUND",
		message: func(m []string, _ string) string { return "Run " + m[1] + " not found" },
		suggestions: func(_ []string) []string {
			return []string{"Run `gai-ghcli run list` to see recent runs"}
		},
	},
	{
		pattern: regexp.MustCompile(`gh auth login`),
		code:    "AUTH_REQUIRED",
		message: func(_ []string, _ string) string { return "GitHub auth required — run `gh auth login` first" },
	},
	{
		pattern: regexp.MustCompile(`(?i)(project scope|scope.*project|token.*project)`),
		code:    "AUTH_REQUIRED",
		message: func(_ []string, _ string) string {
			return "GitHub project scope required — run `gh auth refresh -s project`"
		},
	},
	{
		pattern: regexp.MustCompile(`(?i)secondary rate limit`),
		code:    "RATE_LIMITED",
		message: func(_ []string, _ string) string { return "GitHub secondary rate limit hit — wait ~60s and retry" },
		suggestions: func(_ []string) []string {
			return []string{
				"Wait 60s before retrying",
				"Use `gh api` (REST) for read-only ops, which has a separate budget",
			}
		},
	},
	{
		pattern: regexp.MustCompile(`(?i)API rate limit (?:already )?exceeded`),
		code:    "RATE_LIMITED",
		message: func(_ []string, _ string) string { return "GitHub API rate limit exceeded" },
		suggestions: func(_ []string) []string {
			return []string{
				"Wait until the hourly window resets (run `gh api rate_limit` to check)",
				"Use a different identity with `gh auth switch` if available",
			}
		},
	},
	{
		pattern: regexp.MustCompile(`(?i)sub-issue is already a sub-issue of issue with number (\d+)`),
		code:    "VALIDATION_ERROR",
		message: func(m []string, _ string) string { return "Issue is already a sub-issue of #" + m[1] },
	},
	{
		pattern: regexp.MustCompile(`(?i)sub-?issue.*?(cycle|circular)`),
		code:    "VALIDATION_ERROR",
		message: func(_ []string, _ string) string { return "Cannot add sub-issue: would create a cycle" },
	},
	{
		pattern: regexp.MustCompile(`(?i)issue cannot be a sub-?issue of itself`),
		code:    "VALIDATION_ERROR",
		message: func(_ []string, _ string) string { return "An issue cannot be a sub-issue of itself" },
	},
	{
		pattern: regexp.MustCompile(`HTTP 403`),
		code:    "FORBIDDEN",
		message: func(_ []string, _ string) string { return "Insufficient permissions for this action" },
	},
	{
		pattern: regexp.MustCompile(`HTTP 422`),
		code:    "VALIDATION_ERROR",
		message: func(_ []string, stderr string) string {
			re := regexp.MustCompile(`"message"\s*:\s*"([^"]+)"`)
			if m := re.FindStringSubmatch(stderr); len(m) == 2 {
				return m[1]
			}
			return "Validation error"
		},
	},
}

var notFoundRe = regexp.MustCompile(`(?i)not found`)

func firstErrorLine(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

// MapGhError maps gh stderr to a structured GoAI error.
func MapGhError(stderr string, exitCode int) *gaisdk.GoAIError {
	for _, p := range patterns {
		if m := p.pattern.FindStringSubmatch(stderr); m != nil {
			suggestions := []string{}
			if p.suggestions != nil {
				suggestions = p.suggestions(m)
			}
			return gaisdk.NewGoAIError(p.message(m, stderr), p.code, suggestions...)
		}
	}
	if notFoundRe.MatchString(stderr) {
		return gaisdk.NewGoAIError(firstErrorLine(stderr), "NOT_FOUND")
	}
	msg := firstErrorLine(stderr)
	if msg == "" {
		msg = fmt.Sprintf("gh exited with code %d", exitCode)
	}
	return gaisdk.NewGoAIError(msg, "UNKNOWN")
}

// GhNotInstalled returns an error when gh is not installed.
func GhNotInstalled() *gaisdk.GoAIError {
	return gaisdk.NewGoAIError(
		"gh CLI is not installed — see https://cli.github.com",
		"GH_NOT_INSTALLED",
	)
}
