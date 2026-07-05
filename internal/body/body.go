package body

import (
	"os"
	"regexp"
	"strings"

	"github.com/cyphix/gaighcli/internal/errors"
)

type bodyFlagMatch struct {
	flag  string
	value string
}

// TakeBodyOptions configures body resolution.
type TakeBodyOptions struct {
	Required            bool
	InlineFlags         []string
	FileFlags           []string
	ValueBoundaryFlags  []string
	Label               string
	Suggestions         []string
}

func defaultSuggestions(label string) []string {
	return []string{
		"Use --body \"...\" for inline " + label + ", or --body-file <path> for markdown from a file",
	}
}

func isMissingValue(value string) bool {
	return strings.TrimSpace(value) == ""
}

func isValueBoundary(arg string, flags []string) bool {
	for _, flag := range flags {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

func takeFlagMatches(args *[]string, flags, valueBoundaryFlags []string) []bodyFlagMatch {
	var matches []bodyFlagMatch
	a := *args
	for index := 0; index < len(a); index++ {
		arg := a[index]
		matched := false
		for _, flag := range flags {
			equalsPrefix := flag + "="
			if arg == flag {
				var value string
				if index+1 < len(a) && !isValueBoundary(a[index+1], valueBoundaryFlags) {
					value = a[index+1]
					a = append(a[:index], a[index+2:]...)
				} else {
					a = append(a[:index], a[index+1:]...)
				}
				index--
				matches = append(matches, bodyFlagMatch{flag: flag, value: value})
				matched = true
				break
			}
			if strings.HasPrefix(arg, equalsPrefix) {
				value := arg[len(equalsPrefix):]
				a = append(a[:index], a[index+1:]...)
				index--
				matches = append(matches, bodyFlagMatch{flag: flag, value: value})
				matched = true
				break
			}
		}
		if matched {
			continue
		}
	}
	*args = a
	return matches
}

func readBodyFile(flag, path string, suggestions []string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.NewGoAIError(flag+" path not found: "+path, "VALIDATION_ERROR", suggestions...)
		}
		return "", errors.NewGoAIError("Could not read "+flag+" path: "+path, "VALIDATION_ERROR", suggestions...)
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return "", errors.NewGoAIError(
			flag+" must point to a readable UTF-8 file, not a directory: "+path,
			"VALIDATION_ERROR",
			suggestions...,
		)
	}
	return string(data), nil
}

// TakeBody resolves body from inline or file flags and removes them from args.
func TakeBody(args *[]string, opts TakeBodyOptions) (string, error) {
	inlineFlags := opts.InlineFlags
	if len(inlineFlags) == 0 {
		inlineFlags = []string{"--body"}
	}
	fileFlags := opts.FileFlags
	if len(fileFlags) == 0 {
		fileFlags = []string{"--body-file"}
	}
	valueBoundaryFlags := append([]string{}, inlineFlags...)
	valueBoundaryFlags = append(valueBoundaryFlags, fileFlags...)
	valueBoundaryFlags = append(valueBoundaryFlags, opts.ValueBoundaryFlags...)
	seen := make(map[string]bool)
	var unique []string
	for _, f := range valueBoundaryFlags {
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
		}
	}
	label := opts.Label
	if label == "" {
		label = "body"
	}
	suggestions := opts.Suggestions
	if len(suggestions) == 0 {
		suggestions = defaultSuggestions(label)
	}
	inlineMatches := takeFlagMatches(args, inlineFlags, unique)
	fileMatches := takeFlagMatches(args, fileFlags, unique)
	matches := append(inlineMatches, fileMatches...)

	if len(matches) == 0 {
		if opts.Required {
			return "", errors.NewGoAIError(
				inlineFlags[0]+" or "+fileFlags[0]+" is required",
				"VALIDATION_ERROR",
				suggestions...,
			)
		}
		return "", nil
	}
	if len(matches) > 1 {
		flags := make([]string, len(matches))
		for i, m := range matches {
			flags[i] = m.flag
		}
		return "", errors.NewGoAIError(
			"Use only one "+label+" source: "+strings.Join(flags, ", ")+" were provided",
			"VALIDATION_ERROR",
			suggestions...,
		)
	}
	match := matches[0]
	if isMissingValue(match.value) {
		noun := "text"
		for _, f := range fileFlags {
			if f == match.flag {
				noun = "path"
				break
			}
		}
		return "", errors.NewGoAIError(match.flag+" requires "+noun, "VALIDATION_ERROR", suggestions...)
	}
	for _, f := range fileFlags {
		if f == match.flag {
			return readBodyFile(match.flag, match.value, suggestions)
		}
	}
	return match.value, nil
}

var (
	prURLRe     = regexp.MustCompile(`\[([^\]]+)\]\(https://github\.com/[^/]+/[^/]+/pull/(\d+)\)`)
	issueURLRe  = regexp.MustCompile(`\[([^\]]+)\]\(https://github\.com/[^/]+/[^/]+/issues/(\d+)\)`)
	standalonePR = regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)
	standaloneIssue = regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/issues/(\d+)`)
	imageRe     = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	longLinkRe  = regexp.MustCompile(`\[([^\]]+)\]\([^)]{80,}\)`)
	longURLRe   = regexp.MustCompile(`https?://\S{100,}`)
	quoteBlockRe = regexp.MustCompile(`(?m)(^|\n)(>\s?[^\n]*\n?){3,}`)
)

// CleanBody reduces token cost of body text.
func CleanBody(text string) string {
	s := prURLRe.ReplaceAllString(text, "[$1](PR#$2)")
	s = issueURLRe.ReplaceAllString(s, "[$1](Issue#$2)")
	s = standalonePR.ReplaceAllString(s, "PR#$1")
	s = standaloneIssue.ReplaceAllString(s, "Issue#$1")
	s = imageRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := imageRe.FindStringSubmatch(m)
		if len(sub) > 1 && sub[1] != "" {
			return "[image: " + sub[1] + "]"
		}
		return "[image]"
	})
	s = longLinkRe.ReplaceAllString(s, "[$1]")
	s = longURLRe.ReplaceAllString(s, "[long URL removed]")
	s = quoteBlockRe.ReplaceAllString(s, "$1[quoted text removed]\n")
	return s
}

// TruncateBody truncates a body field for display.
func TruncateBody(body any, maxLen int) string {
	if maxLen == 0 {
		maxLen = 500
	}
	text, ok := body.(string)
	if !ok || text == "" {
		return ""
	}
	if len(text) <= maxLen {
		return text
	}
	cleaned := CleanBody(text)
	if len(cleaned) <= maxLen {
		if cleaned != text {
			return cleaned + "\n(cleaned, " + itoa(len(text)) + " chars original — use --full to see original)"
		}
		return cleaned
	}
	return cleaned[:maxLen] + "\n... (truncated, " + itoa(len(cleaned)) + " chars total — use --full to see complete body)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
