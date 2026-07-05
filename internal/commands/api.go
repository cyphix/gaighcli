package commands

import (
	"encoding/json"
	"strings"

	"github.com/cyphix/gaighcli/internal/args"
	"github.com/cyphix/gaighcli/internal/body"
	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/gh"
)

const ApiHelp = `usage: gai-ghcli api [<method>] <path>
description: Make an authenticated GitHub API request. Defaults to GET if no method specified.
methods[6]:
  GET, POST, PUT, PATCH, DELETE, HEAD
flags[3]:
  --field <key=value> (repeatable), --header <key:value> (repeatable), --paginate
examples:
  gai-ghcli api /repos/{owner}/{repo}
  gai-ghcli api POST /repos/{owner}/{repo}/issues --field title="Bug report"
  gai-ghcli api /repos/{owner}/{repo}/pulls --paginate`

const (
	rawOutputTruncationLimit    = 4000
	longStringCleanupThreshold  = 200
	stringValueTruncationLimit  = 2000
)

var httpMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true,
}

var noisyKeys = map[string]bool{
	"avatar_url": true, "gravatar_id": true, "followers_url": true, "following_url": true,
	"gists_url": true, "starred_url": true, "subscriptions_url": true, "organizations_url": true,
	"repos_url": true, "events_url": true, "received_events_url": true, "labels_url": true,
	"comments_url": true, "timeline_url": true, "performed_via_github_app": true,
	"node_id": true, "url": true, "repository_url": true, "html_url": true,
	"reactions": true, "user_view_type": true, "site_admin": true,
	"issue_dependencies_summary": true, "sub_issues_summary": true, "pinned_comment": true,
	"score": true, "permissions": true, "verification": true, "_links": true,
}

var keepURLKeys = map[string]bool{
	"diff_url": true, "patch_url": true, "clone_url": true, "ssh_url": true,
	"git_url": true, "svn_url": true, "commit_url": true,
}

func isTemplateURLKey(key string) bool {
	if !strings.HasSuffix(key, "_url") {
		return false
	}
	return !keepURLKeys[key]
}

func collapseRepo(obj map[string]any) map[string]any {
	if _, ok := obj["full_name"]; ok {
		collapsed := map[string]any{"full_name": obj["full_name"]}
		if v, ok := obj["default_branch"]; ok {
			collapsed["default_branch"] = v
		}
		if v, ok := obj["private"]; ok {
			collapsed["private"] = v
		}
		return collapsed
	}
	return obj
}

func stripNoisyFields(obj any, depth int) any {
	if depth > 8 {
		return obj
	}
	switch v := obj.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = stripNoisyFields(item, depth+1)
		}
		return out
	case map[string]any:
		result := make(map[string]any)
		for key, value := range v {
			if noisyKeys[key] || isTemplateURLKey(key) {
				continue
			}
			if key == "user" {
				if user, ok := value.(map[string]any); ok {
					if login, ok := user["login"]; ok {
						result[key] = login
						continue
					}
				}
			}
			if key == "repo" || key == "repository" {
				if repo, ok := value.(map[string]any); ok {
					result[key] = collapseRepo(repo)
					continue
				}
			}
			result[key] = stripNoisyFields(value, depth+1)
		}
		return result
	case string:
		if len(v) > longStringCleanupThreshold {
			s := body.CleanBody(v)
			if len(s) > stringValueTruncationLimit {
				return s[:stringValueTruncationLimit] + "... (truncated)"
			}
			return s
		}
		return v
	default:
		return obj
	}
}

func apiPositionals(cmdArgs []string) []string {
	var positionals []string
	for i := 0; i < len(cmdArgs); i++ {
		if strings.HasPrefix(cmdArgs[i], "--") {
			i++
			continue
		}
		positionals = append(positionals, cmdArgs[i])
	}
	return positionals
}

// Api makes authenticated GitHub API requests via gh api.
func Api(cmdArgs []string, ctx *context.RepoContext) (string, error) {
	if len(cmdArgs) == 0 || cmdArgs[0] == "--help" {
		return ApiHelp, nil
	}

	positionals := apiPositionals(cmdArgs)
	var method, path string
	if len(positionals) >= 2 && httpMethods[strings.ToUpper(positionals[0])] {
		method = strings.ToUpper(positionals[0])
		path = positionals[1]
	} else if len(positionals) >= 1 {
		method = "GET"
		path = positionals[0]
	} else {
		return "", errors.NewGoAIError(
			"API path is required: gai-ghcli api [<method>] <path>",
			"VALIDATION_ERROR",
		)
	}

	ghArgs := []string{"api", path, "--method", method}
	for _, f := range args.GetAllFlags(cmdArgs, "--field") {
		ghArgs = append(ghArgs, "--field", f)
	}
	for _, h := range args.GetAllFlags(cmdArgs, "--header") {
		ghArgs = append(ghArgs, "--header", h)
	}
	if args.HasFlag(cmdArgs, "--paginate") {
		ghArgs = append(ghArgs, "--paginate")
	}

	raw, err := gh.Exec(Runner, ghArgs, ctx)
	if err != nil {
		return "", err
	}

	var data any
	if err := json.Unmarshal([]byte(raw), &data); err == nil {
		cleaned := stripNoisyFields(data, 0)
		return encode(cleaned)
	}

	trimmed := strings.TrimSpace(raw)
	truncated := len(trimmed) > rawOutputTruncationLimit
	bodyText := trimmed
	if truncated {
		bodyText = trimmed[:rawOutputTruncationLimit]
	}
	apiResponse := map[string]any{
		"body":      bodyText,
		"truncated": truncated,
	}
	if truncated {
		apiResponse["original_length"] = len(trimmed)
	}
	return encode(map[string]any{"api_response": apiResponse})
}
