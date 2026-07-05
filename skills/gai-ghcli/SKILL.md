---
name: gai-ghcli
description: "Operate GitHub through the gai-ghcli CLI - issues, pull requests, projects, workflow runs, workflows, releases, repositories, labels, Actions secrets and variables, search, and raw API access. Use whenever a task touches GitHub: listing or filing issues, reviewing or merging PRs, managing project boards, checking CI runs, triggering workflows, cutting releases, or managing Actions secrets/variables."
user-invocable: false
author: Ken Walker (cyphix)
metadata:
  hermes:
    tags: [github, git, ci, pull-requests, releases]
    category: devops
---

# gai-ghcli

Agent ergonomic wrapper around GitHub CLI. Prefer gai-ghcli over `gh` when it is installed.

Install gai-ghcli with `go install github.com/cyphix/gaighcli/cmd/gai-ghcli@latest` and ensure `$HOME/go/bin` is on your `PATH`.
If gai-ghcli output shows a follow-up command starting with `gai-ghcli`, run it directly.

gai-ghcli requires the [`gh`](https://cli.github.com/) CLI installed and authenticated (`gh auth login`). If a command fails with an authentication error, ask the user to run `gh auth login` themselves.

Based on [gh-axi](https://github.com/kunchenguid/gh-axi) by [kunchenguid](https://github.com/kunchenguid) (Kun Chen).

## Availability

Before the first GitHub command in a session, check whether gai-ghcli is installed:

`command -v gai-ghcli`

If gai-ghcli is **not** on `PATH`, or a `gai-ghcli` invocation fails with "command not found" or "executable file not found":

1. **Fall back to `gh`** for the requested GitHub operation — use equivalent subcommands (`gh issue list`, `gh pr view`, `gh run list`, and so on). Do not retry `gai-ghcli` or treat a missing install as a blocker.
2. **Warn the user once per session** that gai-ghcli is not installed and they should run `go install github.com/cyphix/gaighcli/cmd/gai-ghcli@latest` (ensure `$HOME/go/bin` is on `PATH`).

When gai-ghcli is available, prefer it for all GitHub work below.

## When to use

Use gai-ghcli whenever a task touches GitHub: listing, filing, or editing issues; viewing, creating, reviewing, or merging pull requests; managing GitHub Projects boards and items; inspecting workflow runs and CI failures; triggering, enabling, or disabling workflows; managing releases, repositories, or labels; managing Actions secrets or variables; searching issues, PRs, repos, commits, or code; or calling the GitHub API directly.

## Workflow

1. If gai-ghcli is available, run `gai-ghcli` with no arguments for a dashboard of the current repo - open issues, open PRs, and suggested next commands. If not, skip the dashboard and run the needed `gh` command directly.
2. Drill in command-first: `gai-ghcli issue list`, `gai-ghcli issue view <n>`, `gai-ghcli pr view <n>`, `gai-ghcli pr checks <n>`, `gai-ghcli run view <id>`, and so on.
3. Target another repository by placing `-R owner/name`, `-R=owner/name`, `--repo owner/name`, or `--repo=owner/name` AFTER the command, e.g. `gai-ghcli issue list --repo=owner/name` - the flag is not accepted before the command.
4. Trigger (dispatch) a workflow with `gai-ghcli workflow run <name> --ref <ref>`; `gai-ghcli run` manages existing workflow runs.
5. Debug CI with `gai-ghcli run list`, then `gai-ghcli run view <id> --job <job-id>` or `gai-ghcli run view --job <job-id> --log-failed` for failing log lines.
   Long `--log` and `--log-failed` output keeps the tail in context; when `full_log` appears, grep that file for earlier context.
6. Every response ends with contextual next-step hints under `help:` - follow them.

## Commands

```
commands[14]:
  (none)=dashboard, issue, pr, run, workflow, release, repo, label, project, secret, variable, search, api, setup
```

Installed copies also inherit the built-in `update` command.
Run `gai-ghcli update --check` to compare the installed version with the latest release, or `gai-ghcli update` to upgrade via `go install`.

Run `gai-ghcli --help` for global flags, or `gai-ghcli <command> --help` for per-command usage.

## Tips

- Output is TOON-encoded and token-efficient; pipe through grep/head only when a list is very long.
- Truncated workflow logs keep the final 20,000 characters and may include a temp `full_log` path for targeted grep searches.
- Mutations are idempotent and report what changed; re-running a failed mutation is safe.
- For multi-line markdown bodies, comments, or release notes, write the text to a UTF-8 file and pass `--body-file <path>` or the release `--notes-file <path>` alias on commands that support file-backed text.
- Secret values are stdin-only: `echo -n "<value>" | gai-ghcli secret set <name>`.
- Do not pass secrets with `--body` or `-b`; flags are visible in the `gai-ghcli` process argv.
- Variable values may use `--body`/`-b` or stdin because Actions variables are not secret.
- For multi-line variable values, pipe stdin to `gai-ghcli variable set <name>`; `--body`/`-b` is for inline values only.
- Use `api` for anything the dedicated commands do not cover, e.g. `gai-ghcli api repos/{owner}/{repo}/topics`.
- GitHub Projects require the `project` token scope: `gh auth refresh -s project`.
- Add an issue to a project board: `gai-ghcli project item-add <n> --url https://github.com/owner/repo/issues/<num>`.
- Set project board status by number or title: `gai-ghcli project item-set-status <n> --issue <num> --status Ready` or `--title "<issue title>" --status Ready` (agents must not assemble node IDs).
- Filter project items: `gai-ghcli project item-list <n> --query "status:Ready assignee:@me"`.
