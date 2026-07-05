package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/body"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
	"github.com/cyphix/gaisdk"
)

const ReleaseHelp = `usage: gai-ghcli release <subcommand> [flags]
subcommands[7]:
  list, view <tag>, create <tag>, edit <tag>, delete <tag>, download <tag>, upload <tag>
flags{list}:
  --exclude-drafts, --exclude-pre-releases, --limit (default 10)
flags{view}:
  --full (show complete release notes without truncation)
flags{create}:
  --title/-t, --notes/-n or --body, --notes-file/-F or --body-file, --draft/-d, --prerelease/-p, --target, --generate-notes, --discussion-category, --notes-start-tag, --verify-tag, --notes-from-tag, --fail-on-no-commits, --latest[=true|false], <files...>
flags{edit}:
  --title, --notes/-n or --body, --notes-file/-F or --body-file, --draft, --prerelease
flags{download}:
  --pattern, --dir
examples:
  gai-ghcli release list --exclude-drafts
  gai-ghcli release view v1.2.0 --full
  gai-ghcli release create v1.3.0 --body-file notes.md --draft dist/app.zip`

var (
	releaseListSchema = []toon.FieldDef{
		toon.Field("tagName", "tag"),
		toon.Field("name", ""),
		toon.BoolYesNo("isDraft", "draft"),
		toon.BoolYesNo("isPrerelease", "prerelease"),
		toon.RelativeTime("publishedAt", "published"),
	}
	releaseViewSchema = []toon.FieldDef{
		toon.Field("tagName", "tag"),
		toon.Field("name", ""),
		toon.RelativeTime("publishedAt", "published"),
		toon.Pluck("author", "login", "author"),
		toon.Custom("body", func(item map[string]any) any {
			return body.TruncateBody(item["body"], 1000)
		}),
	}
	releaseViewSchemaFull = []toon.FieldDef{
		toon.Field("tagName", "tag"),
		toon.Field("name", ""),
		toon.RelativeTime("publishedAt", "published"),
		toon.Pluck("author", "login", "author"),
		toon.Custom("body", func(item map[string]any) any {
			if s, ok := item["body"].(string); ok {
				return s
			}
			return ""
		}),
	}
	releaseNotesFlags = []string{"--notes", "-n", "--notes-file", "-F"}
)

// Release handles release subcommands.
func Release(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	if len(cmdArgs) == 0 || cmdArgs[0] == "--help" {
		return ReleaseHelp, nil
	}
	sub := cmdArgs[0]
	switch sub {
	case "list":
		return listReleases(cmdArgs, ctx)
	case "view":
		return viewRelease(cmdArgs, ctx)
	case "create":
		return createRelease(cmdArgs, ctx)
	case "edit":
		return editRelease(cmdArgs, ctx)
	case "delete":
		return deleteRelease(cmdArgs, ctx)
	case "download":
		return downloadRelease(cmdArgs, ctx)
	case "upload":
		return uploadRelease(cmdArgs, ctx)
	default:
		return toon.RenderError("Unknown subcommand: "+sub, "VALIDATION_ERROR",
			"Available subcommands: list, view, create, edit, delete, download, upload")
	}
}

func takeFirstFlag(cmdArgs *[]string, flags []string) (string, bool) {
	for _, flag := range flags {
		prefix := flag + "="
		a := *cmdArgs
		for i, arg := range a {
			if arg == flag {
				var val string
				if i+1 < len(a) {
					val = a[i+1]
					*cmdArgs = append(a[:i], a[i+2:]...)
				} else {
					*cmdArgs = append(a[:i], a[i+1:]...)
				}
				return val, true
			}
			if strings.HasPrefix(arg, prefix) {
				val := arg[len(prefix):]
				*cmdArgs = append(a[:i], a[i+1:]...)
				return val, true
			}
		}
	}
	return "", false
}

func appendValueFlag(ghArgs *[]string, cmdArgs *[]string, outputFlag string, inputFlags ...string) {
	if len(inputFlags) == 0 {
		inputFlags = []string{outputFlag}
	}
	if value, ok := takeFirstFlag(cmdArgs, inputFlags); ok {
		*ghArgs = append(*ghArgs, outputFlag, value)
	}
}

func appendBoolFlag(ghArgs *[]string, cmdArgs *[]string, outputFlag string, inputFlags ...string) {
	if len(inputFlags) == 0 {
		inputFlags = []string{outputFlag}
	}
	for _, flag := range inputFlags {
		if args.TakeBoolFlag(cmdArgs, flag) {
			*ghArgs = append(*ghArgs, outputFlag)
			return
		}
	}
}

func appendOptionalValueBoolFlag(ghArgs *[]string, cmdArgs *[]string, outputFlag string, inputFlags ...string) {
	if len(inputFlags) == 0 {
		inputFlags = []string{outputFlag}
	}
	a := *cmdArgs
	for _, flag := range inputFlags {
		prefix := flag + "="
		for i, arg := range a {
			if strings.HasPrefix(arg, prefix) {
				*ghArgs = append(*ghArgs, outputFlag+"="+arg[len(prefix):])
				a = append(a[:i], a[i+1:]...)
				*cmdArgs = a
				return
			}
		}
	}
	appendBoolFlag(ghArgs, cmdArgs, outputFlag, inputFlags...)
}

func findProvidedFlags(cmdArgs []string, flags []string) []string {
	var found []string
	for _, flag := range flags {
		for _, arg := range cmdArgs {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				found = append(found, flag)
				break
			}
		}
	}
	return found
}

func takeReleaseBodyAlias(cmdArgs *[]string) (string, error) {
	return body.TakeBody(cmdArgs, body.TakeBodyOptions{
		Label:              "release notes",
		ValueBoundaryFlags: releaseNotesFlags,
	})
}

func assertNoReleaseNotesConflict(bodyText string, cmdArgs []string, flags []string) error {
	if bodyText == "" {
		return nil
	}
	conflicts := findProvidedFlags(cmdArgs, flags)
	if len(conflicts) == 0 {
		return nil
	}
	return errors.NewGoAIError(
		"Use only one release notes source: --body/--body-file cannot be combined with "+strings.Join(conflicts, ", "),
		"VALIDATION_ERROR",
		"Use --body-file <path> for file-backed release notes, or remove --body-file and use --notes-file <path>",
	)
}

func listReleases(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	limit := args.GetFlag(cmdArgs, "--limit")
	if limit == "" {
		limit = "10"
	}
	ghArgs := []string{
		"release", "list",
		"--json", "tagName,name,isDraft,isPrerelease,publishedAt",
		"--limit", limit,
	}
	if args.HasFlag(cmdArgs, "--exclude-drafts") {
		ghArgs = append(ghArgs, "--exclude-drafts")
	}
	if args.HasFlag(cmdArgs, "--exclude-pre-releases") {
		ghArgs = append(ghArgs, "--exclude-pre-releases")
	}
	releases, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	isEmpty := len(releases) == 0
	limitNum, _ := strconv.Atoi(limit)
	var countLine string
	if len(releases) == limitNum {
		countLine = fmt.Sprintf("count: %d (showing first %d; run `gai-ghcli repo view` for total count)", len(releases), len(releases))
	} else {
		countLine = fmt.Sprintf("count: %d", len(releases))
	}
	list, err := toon.RenderList("releases", releases, releaseListSchema)
	if err != nil {
		return "", err
	}
	isEmptyVal := isEmpty
	return renderWithHelp([]string{countLine, list}, suggestions.Context{
		Domain: "release", Action: "list", IsEmpty: &isEmptyVal, Repo: ctx,
	})
}

func viewRelease(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	full := args.HasFlag(cmdArgs, "--full")
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Tag is required: gai-ghcli release view <tag>", "VALIDATION_ERROR")
	}
	tag := pos[1]
	release, err := gh.JSON[map[string]any](Runner,
		[]string{"release", "view", tag, "--json", "tagName,name,publishedAt,author,body"}, ctx)
	if err != nil {
		return "", err
	}
	schema := releaseViewSchema
	if full {
		schema = releaseViewSchemaFull
	}
	detail, err := toon.RenderDetail("release", release, schema)
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(detail), nil
}

func createRelease(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	remaining := append([]string(nil), cmdArgs[1:]...)
	optionArgs := []string{}
	bodyText, err := takeReleaseBodyAlias(&remaining)
	if err != nil {
		return "", err
	}
	if err := assertNoReleaseNotesConflict(bodyText, remaining, releaseNotesFlags); err != nil {
		return "", err
	}
	if bodyText != "" {
		optionArgs = append(optionArgs, "--notes", bodyText)
	}
	appendValueFlag(&optionArgs, &remaining, "--title", "--title", "-t")
	appendValueFlag(&optionArgs, &remaining, "--notes", "--notes", "-n")
	appendValueFlag(&optionArgs, &remaining, "--notes-file", "--notes-file", "-F")
	appendValueFlag(&optionArgs, &remaining, "--target")
	appendValueFlag(&optionArgs, &remaining, "--discussion-category")
	appendValueFlag(&optionArgs, &remaining, "--notes-start-tag")
	appendBoolFlag(&optionArgs, &remaining, "--draft", "--draft", "-d")
	appendBoolFlag(&optionArgs, &remaining, "--prerelease", "--prerelease", "-p")
	appendBoolFlag(&optionArgs, &remaining, "--generate-notes")
	appendBoolFlag(&optionArgs, &remaining, "--verify-tag")
	appendBoolFlag(&optionArgs, &remaining, "--notes-from-tag")
	appendBoolFlag(&optionArgs, &remaining, "--fail-on-no-commits")
	appendOptionalValueBoolFlag(&optionArgs, &remaining, "--latest")

	var tag string
	var fileArgs []string
	for _, a := range remaining {
		if !strings.HasPrefix(a, "-") {
			if tag == "" {
				tag = a
			} else {
				fileArgs = append(fileArgs, a)
			}
		}
	}
	if tag == "" {
		return "", errors.NewGoAIError("Tag is required: gai-ghcli release create <tag>", "VALIDATION_ERROR")
	}

	ghArgs := append([]string{"release", "create", tag}, optionArgs...)
	ghArgs = append(ghArgs, fileArgs...)
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"created": "ok", "tag": tag})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "release", Action: "create", ID: tag, Repo: ctx})
}

func editRelease(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	remaining := append([]string(nil), cmdArgs...)
	bodyText, err := takeReleaseBodyAlias(&remaining)
	if err != nil {
		return "", err
	}
	if err := assertNoReleaseNotesConflict(bodyText, remaining, releaseNotesFlags); err != nil {
		return "", err
	}
	title, _ := takeFirstFlag(&remaining, []string{"--title"})
	notes, _ := takeFirstFlag(&remaining, []string{"--notes", "-n"})
	notesFile, _ := takeFirstFlag(&remaining, []string{"--notes-file", "-F"})
	draft := args.TakeBoolFlag(&remaining, "--draft")
	prerelease := args.TakeBoolFlag(&remaining, "--prerelease")
	pos := positionals(remaining, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Tag is required: gai-ghcli release edit <tag>", "VALIDATION_ERROR")
	}
	tag := pos[1]

	ghArgs := []string{"release", "edit", tag}
	if title != "" {
		ghArgs = append(ghArgs, "--title", title)
	}
	if bodyText != "" {
		ghArgs = append(ghArgs, "--notes", bodyText)
	}
	if notes != "" {
		ghArgs = append(ghArgs, "--notes", notes)
	}
	if notesFile != "" {
		ghArgs = append(ghArgs, "--notes-file", notesFile)
	}
	if draft {
		ghArgs = append(ghArgs, "--draft")
	}
	if prerelease {
		ghArgs = append(ghArgs, "--prerelease")
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"edit": "ok", "tag": tag})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "release", Action: "edit", ID: tag, Repo: ctx})
}

func deleteRelease(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Tag is required: gai-ghcli release delete <tag>", "VALIDATION_ERROR")
	}
	tag := pos[1]

	_, err := gh.JSON[map[string]any](Runner,
		[]string{"release", "view", tag, "--json", "tagName"}, ctx)
	if err != nil {
		var goaiErr *gaisdk.GoAIError
		if gaisdk.AsGoAIError(err, &goaiErr) && goaiErr.Code == "NOT_FOUND" {
			enc, encErr := encode(map[string]any{"delete": "already_deleted", "tag": tag})
			if encErr != nil {
				return "", encErr
			}
			return renderWithHelp([]string{enc}, suggestions.Context{Domain: "release", Action: "delete", ID: tag, Repo: ctx})
		}
		return "", err
	}

	if _, err := gh.Exec(Runner, []string{"release", "delete", tag, "--yes"}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"delete": "ok", "tag": tag})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "release", Action: "delete", ID: tag, Repo: ctx})
}

func downloadRelease(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Tag is required: gai-ghcli release download <tag>", "VALIDATION_ERROR")
	}
	tag := pos[1]
	ghArgs := []string{"release", "download", tag}
	if pattern := args.GetFlag(cmdArgs, "--pattern"); pattern != "" {
		ghArgs = append(ghArgs, "--pattern", pattern)
	}
	if dir := args.GetFlag(cmdArgs, "--dir"); dir != "" {
		ghArgs = append(ghArgs, "--dir", dir)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"download": "ok", "tag": tag})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "release", Action: "download", ID: tag, Repo: ctx})
}

func uploadRelease(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Tag is required: gai-ghcli release upload <tag> <files...>", "VALIDATION_ERROR")
	}
	tag := pos[1]
	files := pos[2:]
	if len(files) == 0 {
		return "", errors.NewGoAIError(
			"At least one file is required: gai-ghcli release upload <tag> <files...>",
			"VALIDATION_ERROR",
		)
	}
	ghArgs := append([]string{"release", "upload", tag}, files...)
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"upload": "ok", "tag": tag, "files": len(files)})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "release", Action: "upload", ID: tag, Repo: ctx})
}
