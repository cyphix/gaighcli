package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/fields"
	"github.com/cyphix/gaighcli/internal/format"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/suggestions"
	"github.com/cyphix/gaighcli/internal/toon"
)

const RunHelp = `usage: gai-ghcli run <subcommand> [flags]
subcommands[7]:
  list, view <id>, watch <id>, rerun <id>, cancel <id>, delete <id>, download <id>
note:
  manages existing runs; to trigger (dispatch) a workflow, use ` + "`gai-ghcli workflow run <name> --ref <ref>`" + `
flags{list}:
  --workflow, --branch, --status, --event, --user, --commit, --limit (default 10), --fields <a,b,c>
flags{view}:
  --job <job-id>, --log, --log-failed, --conclusion <success|failure|cancelled|skipped> (filter jobs by conclusion)
  long --log/--log-failed output keeps the tail and may include full_log for grep searches
flags{rerun}:
  --failed, --debug, --job
flags{download}:
  --name, --dir
examples:
  gai-ghcli run list --workflow ci.yml --status failure
  gai-ghcli run view 123456 --log-failed
  gai-ghcli run rerun 123456 --failed`

const logTruncateLimit = 20000

const runListBaseJSON = "databaseId,displayTitle,status,conclusion,workflowName,headBranch,event,createdAt"

var (
	runListSchema = []toon.FieldDef{
		toon.Field("databaseId", "id"),
		toon.Field("displayTitle", "title"),
		toon.Lower("status", ""),
		toon.Lower("conclusion", ""),
		toon.Field("workflowName", "workflow"),
		toon.Field("headBranch", "branch"),
		toon.Field("event", ""),
		toon.RelativeTime("createdAt", "created"),
	}
	runViewSchema = []toon.FieldDef{
		toon.Field("databaseId", "id"),
		toon.Field("displayTitle", "title"),
		toon.Lower("status", ""),
		toon.Lower("conclusion", ""),
		toon.Field("workflowName", "workflow"),
		toon.Field("headBranch", "branch"),
		toon.RelativeTime("createdAt", "created"),
	}
	runJobSchema = []toon.FieldDef{
		toon.Field("databaseId", "id"),
		toon.Field("name", ""),
		toon.Lower("status", ""),
		toon.Lower("conclusion", ""),
	}
	runStepSchema = []toon.FieldDef{
		toon.Field("number", ""),
		toon.Field("name", ""),
		toon.Lower("status", ""),
		toon.Lower("conclusion", ""),
	}
	runListExtraFields = map[string]fields.ExtraFieldSpec{
		"headSha":   {JSONKey: "headSha", Def: toon.Field("headSha", "sha")},
		"number":    {JSONKey: "number", Def: toon.Field("number", "")},
		"url":       {JSONKey: "url", Def: toon.Field("url", "")},
		"updatedAt": {JSONKey: "updatedAt", Def: toon.RelativeTime("updatedAt", "updated_at")},
	}
	logSafeRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)
)

// Run handles run subcommands.
func Run(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	if len(cmdArgs) == 0 || cmdArgs[0] == "--help" {
		return RunHelp, nil
	}
	sub := cmdArgs[0]
	switch sub {
	case "list":
		return listRuns(cmdArgs, ctx)
	case "view":
		return viewRun(cmdArgs, ctx)
	case "watch":
		return watchRun(cmdArgs, ctx)
	case "rerun":
		return rerunRun(cmdArgs, ctx)
	case "cancel":
		return cancelRun(cmdArgs, ctx)
	case "delete":
		return deleteRun(cmdArgs, ctx)
	case "download":
		return downloadRun(cmdArgs, ctx)
	default:
		return toon.RenderError("Unknown subcommand: "+sub, "VALIDATION_ERROR",
			"Available subcommands: list, view, watch, rerun, cancel, delete, download")
	}
}

func takeViewFlagValue(cmdArgs *[]string, flag string) (string, error) {
	present := args.HasFlag(*cmdArgs, flag)
	value := args.TakeFlag(cmdArgs, flag)
	if !present {
		return "", nil
	}
	if value == "" || strings.HasPrefix(value, "--") {
		return "", errors.NewGoAIError("Missing value for "+flag, "VALIDATION_ERROR")
	}
	return value, nil
}

func logFileName(run, mode, job string) string {
	safe := func(s string) string { return logSafeRe.ReplaceAllString(s, "_") }
	name := safe(run)
	if job != "" {
		name += "-job-" + safe(job)
	}
	return name + "-" + safe(mode) + ".log"
}

func saveFullLog(run, mode, output, job string) string {
	dir, err := os.MkdirTemp("", "gai-ghcli-logs-")
	if err != nil {
		return ""
	}
	file := filepath.Join(dir, logFileName(run, mode, job))
	if err := os.WriteFile(file, []byte(output), 0o600); err != nil {
		return ""
	}
	return file
}

func wrapLogOutput(run, mode, output, job string) (string, error) {
	truncated := len(output) > logTruncateLimit
	display := output
	if truncated {
		display = output[len(output)-logTruncateLimit:]
	}
	runLog := map[string]any{
		"run":       run,
		"mode":      mode,
		"output":    display,
		"truncated": truncated,
	}
	if !truncated {
		return encode(map[string]any{"run_log": runLog})
	}
	runLog["original_length"] = len(output)
	fullLogPath := saveFullLog(run, mode, output, job)
	hint := fmt.Sprintf("Output shows the last %d of %d chars", logTruncateLimit, len(output))
	if fullLogPath != "" {
		runLog["full_log"] = fullLogPath
		enc, err := encode(map[string]any{"run_log": runLog})
		if err != nil {
			return "", err
		}
		return toon.RenderOutput(enc, toon.RenderHelp([]string{
			hint + "; full log saved to " + fullLogPath + " - grep it for earlier context",
		})), nil
	}
	enc, err := encode(map[string]any{"run_log": runLog})
	if err != nil {
		return "", err
	}
	return toon.RenderOutput(enc, toon.RenderHelp([]string{hint})), nil
}

func matchesConclusionFilter(item map[string]any, conclusionFilter string) bool {
	conclusion, _ := item["conclusion"].(string)
	return strings.EqualFold(conclusion, conclusionFilter)
}

func resolveLogRunID(id, jobFlag string, ctx *context.RepoContext) (string, error) {
	if id != "" {
		return id, nil
	}
	run, err := gh.JSON[map[string]any](Runner,
		[]string{"run", "view", "--job", jobFlag, "--json", "databaseId"}, ctx)
	if err != nil {
		return "", err
	}
	if run["databaseId"] == nil {
		return "", errors.NewGoAIError("Unable to resolve run for job "+jobFlag, "UNKNOWN")
	}
	return fmt.Sprint(run["databaseId"]), nil
}

func resolveLogEnvelopeRunID(id, jobFlag string, ctx *context.RepoContext) (string, error) {
	if id != "" {
		return id, nil
	}
	resolved, err := resolveLogRunID(id, jobFlag, ctx)
	if err == nil {
		return resolved, nil
	}
	if jobFlag != "" {
		return jobFlag, nil
	}
	return "", errors.NewGoAIError("Unable to resolve run id for logs", "UNKNOWN")
}

func listRuns(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	work := append([]string(nil), cmdArgs...)
	fieldsArg := args.TakeFlag(&work, "--fields")
	parsed, err := fields.ParseFields(fieldsArg, runListExtraFields)
	if err != nil {
		return "", err
	}
	limit := args.GetFlag(work, "--limit")
	if limit == "" {
		limit = "10"
	}
	jsonFields := runListBaseJSON
	if len(parsed.ExtraJSONKeys) > 0 {
		jsonFields += "," + strings.Join(parsed.ExtraJSONKeys, ",")
	}
	ghArgs := []string{"run", "list", "--json", jsonFields, "--limit", limit}
	if v := args.GetFlag(work, "--workflow"); v != "" {
		ghArgs = append(ghArgs, "--workflow", v)
	}
	if v := args.GetFlag(work, "--branch"); v != "" {
		ghArgs = append(ghArgs, "--branch", v)
	}
	if v := args.GetFlag(work, "--status"); v != "" {
		ghArgs = append(ghArgs, "--status", v)
	}
	if v := args.GetFlag(work, "--event"); v != "" {
		ghArgs = append(ghArgs, "--event", v)
	}
	if v := args.GetFlag(work, "--user"); v != "" {
		ghArgs = append(ghArgs, "--user", v)
	}
	if v := args.GetFlag(work, "--commit"); v != "" {
		ghArgs = append(ghArgs, "--commit", v)
	}

	runs, err := gh.JSON[[]map[string]any](Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}
	isEmpty := len(runs) == 0
	limitNum, _ := strconv.Atoi(limit)
	countLine := format.CountLine(format.CountLineOptions{Count: len(runs), Limit: &limitNum})
	schema := runListSchema
	if len(parsed.ExtraDefs) > 0 {
		schema = append(append([]toon.FieldDef{}, runListSchema...), parsed.ExtraDefs...)
	}
	list, err := toon.RenderList("runs", runs, schema)
	if err != nil {
		return "", err
	}
	isEmptyVal := isEmpty
	return renderWithHelp([]string{countLine, list}, suggestions.Context{
		Domain: "run", Action: "list", IsEmpty: &isEmptyVal, Repo: ctx,
	})
}

func viewRun(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	viewArgs := append([]string(nil), cmdArgs...)
	jobFlag, err := takeViewFlagValue(&viewArgs, "--job")
	if err != nil {
		return "", err
	}
	conclusionFilter, err := takeViewFlagValue(&viewArgs, "--conclusion")
	if err != nil {
		return "", err
	}
	pos := positionals(viewArgs, 0)
	id := ""
	if len(pos) > 1 {
		id = pos[1]
	}
	if id == "" && jobFlag == "" {
		return "", errors.NewGoAIError("Run ID is required: gai-ghcli run view <id>", "VALIDATION_ERROR")
	}

	ghSelector := []string{"run", "view"}
	if id != "" {
		ghSelector = append(ghSelector, id)
	}

	if args.HasFlag(cmdArgs, "--log") || args.HasFlag(cmdArgs, "--verbose") {
		ghArgs := append(append([]string{}, ghSelector...), "--log")
		if jobFlag != "" {
			ghArgs = append(ghArgs, "--job", jobFlag)
		}
		output, err := gh.Exec(Runner, ghArgs, ctx)
		if err != nil {
			return "", err
		}
		envelopeID, err := resolveLogEnvelopeRunID(id, jobFlag, ctx)
		if err != nil {
			return "", err
		}
		return wrapLogOutput(envelopeID, "log", output, jobFlag)
	}
	if args.HasFlag(cmdArgs, "--log-failed") {
		ghArgs := append(append([]string{}, ghSelector...), "--log-failed")
		if jobFlag != "" {
			ghArgs = append(ghArgs, "--job", jobFlag)
		}
		output, err := gh.Exec(Runner, ghArgs, ctx)
		if err != nil {
			return "", err
		}
		envelopeID, err := resolveLogEnvelopeRunID(id, jobFlag, ctx)
		if err != nil {
			return "", err
		}
		return wrapLogOutput(envelopeID, "log-failed", output, jobFlag)
	}

	viewGhArgs := append(append([]string{}, ghSelector...),
		"--json", "databaseId,displayTitle,status,conclusion,workflowName,headBranch,createdAt,jobs")
	if jobFlag != "" {
		viewGhArgs = append(viewGhArgs, "--job", jobFlag)
	}
	run, err := gh.JSON[map[string]any](Runner, viewGhArgs, ctx)
	if err != nil {
		return "", err
	}

	detail, err := toon.RenderDetail("run", run, runViewSchema)
	if err != nil {
		return "", err
	}
	blocks := []string{detail}

	var typedJobs []map[string]any
	if jobsArr, ok := run["jobs"].([]any); ok {
		for _, j := range jobsArr {
			if m, ok := j.(map[string]any); ok {
				typedJobs = append(typedJobs, m)
			}
		}
	}

	runLabel := id
	if runLabel == "" {
		runLabel = fmt.Sprint(run["databaseId"])
	}
	if runLabel == "" {
		runLabel = "unknown"
	}

	if jobFlag != "" {
		var job map[string]any
		for _, j := range typedJobs {
			if fmt.Sprint(j["databaseId"]) == jobFlag {
				job = j
				break
			}
		}
		if job == nil {
			return "", errors.NewGoAIError(
				fmt.Sprintf("Job %s not found in run %s", jobFlag, runLabel),
				"VALIDATION_ERROR",
			)
		}
		if conclusionFilter != "" && !matchesConclusionFilter(job, conclusionFilter) {
			return "", errors.NewGoAIError(
				fmt.Sprintf("Job %s does not match conclusion=%s", jobFlag, conclusionFilter),
				"VALIDATION_ERROR",
			)
		}
		jobDetail, err := toon.RenderDetail("job", job, runJobSchema)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, jobDetail)
		if stepsArr, ok := job["steps"].([]any); ok && len(stepsArr) > 0 {
			steps := make([]map[string]any, 0, len(stepsArr))
			for _, s := range stepsArr {
				if m, ok := s.(map[string]any); ok {
					steps = append(steps, m)
				}
			}
			if len(steps) > 0 {
				stepList, err := toon.RenderList("steps", steps, runStepSchema)
				if err != nil {
					return "", err
				}
				blocks = append(blocks, stepList)
			}
		}
	} else if len(typedJobs) > 0 {
		jobs := typedJobs
		if conclusionFilter != "" {
			var filtered []map[string]any
			for _, j := range typedJobs {
				if matchesConclusionFilter(j, conclusionFilter) {
					filtered = append(filtered, j)
				}
			}
			jobs = filtered
			blocks = append(blocks, fmt.Sprintf("jobs: %d of %d with conclusion=%s",
				len(jobs), len(typedJobs), conclusionFilter))
		}
		jobList, err := toon.RenderList("jobs", jobs, runJobSchema)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, jobList)
	}

	return toon.RenderOutput(blocks...), nil
}

func watchRun(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Run ID is required: gai-ghcli run watch <id>", "VALIDATION_ERROR")
	}
	id := pos[1]
	output, err := gh.Exec(Runner, []string{"run", "watch", id, "--exit-status"}, ctx)
	if err != nil {
		return "", err
	}
	return encode(map[string]any{"run_watch": map[string]any{"run": id, "output": strings.TrimSpace(output)}})
}

func rerunRun(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Run ID is required: gai-ghcli run rerun <id>", "VALIDATION_ERROR")
	}
	id := pos[1]
	ghArgs := []string{"run", "rerun", id}
	if args.HasFlag(cmdArgs, "--failed") {
		ghArgs = append(ghArgs, "--failed")
	}
	if args.HasFlag(cmdArgs, "--debug") {
		ghArgs = append(ghArgs, "--debug")
	}
	if job := args.GetFlag(cmdArgs, "--job"); job != "" {
		ghArgs = append(ghArgs, "--job", job)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"rerun": "ok", "run": id})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "run", Action: "rerun", ID: id, Repo: ctx})
}

func cancelRun(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Run ID is required: gai-ghcli run cancel <id>", "VALIDATION_ERROR")
	}
	id := pos[1]

	run, err := gh.JSON[map[string]any](Runner,
		[]string{"run", "view", id, "--json", "status,conclusion"}, ctx)
	if err != nil {
		return "", err
	}

	if status, _ := run["status"].(string); status == "completed" {
		conclusion := "unknown"
		if c, ok := run["conclusion"].(string); ok {
			conclusion = strings.ToLower(c)
		}
		enc, err := encode(map[string]any{
			"cancel":     "already_completed",
			"run":        id,
			"conclusion": conclusion,
		})
		if err != nil {
			return "", err
		}
		return renderWithHelp([]string{enc}, suggestions.Context{Domain: "run", Action: "cancel", ID: id, Repo: ctx})
	}

	if _, err := gh.Exec(Runner, []string{"run", "cancel", id}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"cancel": "ok", "run": id})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "run", Action: "cancel", ID: id, Repo: ctx})
}

func deleteRun(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Run ID is required: gai-ghcli run delete <id>", "VALIDATION_ERROR")
	}
	id := pos[1]
	if _, err := gh.Exec(Runner, []string{"run", "delete", id}, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"delete": "ok", "run": id})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "run", Action: "delete", ID: id, Repo: ctx})
}

func downloadRun(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	pos := positionals(cmdArgs, 0)
	if len(pos) < 2 || pos[1] == "" {
		return "", errors.NewGoAIError("Run ID is required: gai-ghcli run download <id>", "VALIDATION_ERROR")
	}
	id := pos[1]
	ghArgs := []string{"run", "download", id}
	if name := args.GetFlag(cmdArgs, "--name"); name != "" {
		ghArgs = append(ghArgs, "--name", name)
	}
	if dir := args.GetFlag(cmdArgs, "--dir"); dir != "" {
		ghArgs = append(ghArgs, "--dir", dir)
	}
	if _, err := gh.Exec(Runner, ghArgs, ctx); err != nil {
		return "", err
	}
	enc, err := encode(map[string]any{"download": "ok", "run": id})
	if err != nil {
		return "", err
	}
	return renderWithHelp([]string{enc}, suggestions.Context{Domain: "run", Action: "download", ID: id, Repo: ctx})
}
