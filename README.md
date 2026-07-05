# gai-ghcli

GoAI-compliant GitHub CLI wrapper for AI agents. Token-efficient TOON output, contextual suggestions, structured errors, and idempotent mutations — a Go port of [gh-axi](https://github.com/kunchenguid/gh-axi) built with [gaisdk](https://github.com/cyphix/gaisdk).

## Credits

This project is a Go reimplementation of [gh-axi](https://github.com/kunchenguid/gh-axi) by [kunchenguid](https://github.com/kunchenguid) (Kun Chen), which defined the agent-ergonomic GitHub CLI patterns this tool follows: TOON output, contextual suggestions, structured errors, and idempotent mutations.

It builds on the broader [AXI (Agent eXperience Interface)](https://github.com/kunchenguid/axi) ecosystem:

- **[gh-axi](https://github.com/kunchenguid/gh-axi)** — the original TypeScript reference implementation
- **[AXI](https://github.com/kunchenguid/axi)** — design principles and the [axi-sdk-js](https://github.com/kunchenguid/axi/tree/main/packages/axi-sdk-js) SDK that [gaisdk](https://github.com/cyphix/gaisdk) mirrors in Go

gai-ghcli is not a fork of gh-axi. Thank you to Kun Chen for creating gh-axi and the AXI paradigm.

## Install

```bash
go install github.com/cyphix/gaighcli/cmd/gai-ghcli@latest
```

Ensure `$HOME/go/bin` is on your `PATH`.

**Prerequisites:** [GitHub CLI (`gh`)](https://cli.github.com/) installed and authenticated (`gh auth login`).

## Quick start (recommended)

Install the CLI and the agent skill:

```bash
# CLI
go install github.com/cyphix/gaighcli/cmd/gai-ghcli@latest

# Agent skill
npx skills add cyphix/gaighcli --skill gai-ghcli
```

The skill teaches agents to prefer `gai-ghcli` over raw `gh`. It is not a user-facing slash command (`user-invocable: false`) — agents load it when a task touches GitHub. See [skills/gai-ghcli/SKILL.md](skills/gai-ghcli/SKILL.md) for full workflow guidance.

Project-level agent instructions are also in [AGENTS.md](AGENTS.md).

## Usage

```bash
gai-ghcli                              # dashboard (open issues/PRs)
gai-ghcli issue list --state open
gai-ghcli pr view 42
gai-ghcli run list --workflow ci.yml
gai-ghcli secret list
gai-ghcli setup hooks                  # install agent session hooks
gai-ghcli update --check
```

## Commands

| Command | Description |
|---------|-------------|
| *(none)* | Dashboard with repo context, open issues, and PRs |
| `issue` | Issues (list, view, create, edit, close, subissue, …) |
| `pr` | Pull requests (list, view, merge, review, checks, …) |
| `run` | Workflow runs (list, view, watch, rerun, cancel, …) |
| `workflow` | Workflows (list, view, run, enable, disable) |
| `release` | Releases (list, view, create, edit, delete, …) |
| `repo` | Repositories (view, create, clone, fork, list) |
| `label` | Labels |
| `project` | GitHub Projects (list, view, items, fields, link, …) |
| `secret` | Actions secrets (stdin-only values) |
| `variable` | Actions variables |
| `search` | GitHub search (issues, prs, repos, commits, code) |
| `api` | Raw GitHub REST API |
| `setup` | Agent integration (`setup hooks`) |
| `update` | Self-update via `go install` |

Use `-R owner/repo` or `--repo owner/repo` after the command to target a repository. Resolution priority: flag → `GH_REPO` env → git remote origin.

## Agent hooks

```bash
gai-ghcli setup hooks
```

Installs SessionStart hooks for **Claude Code**, **Codex**, and **Cursor**. Restart your agent session after installing.

## Agent skill

The installable skill lives at [skills/gai-ghcli/SKILL.md](skills/gai-ghcli/SKILL.md) and is generated from the same CLI constants as `gai-ghcli --help` so command lists never drift.

```bash
npx skills add cyphix/gaighcli --skill gai-ghcli
```

Regenerate after changing CLI help:

```bash
go run ./cmd/gen-skill
```

## Parity with gh-axi

| Feature | gh-axi | gai-ghcli |
|---------|--------|-----------|
| Core commands & flags | Yes | Yes |
| TOON output + suggestions | Yes | Yes |
| Idempotent mutations | Yes | Yes |
| Session hooks (Claude/Codex/Cursor) | Yes | Yes |
| OpenCode hooks | Yes | No |
| Agent Skill (`npx skills add`) | Yes | Yes (`npx skills add cyphix/gaighcli --skill gai-ghcli`) |
| Install | `npm` / `npx` | `go install` |
| Binary name | `gh-axi` | `gai-ghcli` |

## Development

```bash
go test ./...
go build -o gai-ghcli ./cmd/gai-ghcli
go run ./cmd/gen-skill              # regenerate skills/gai-ghcli/SKILL.md
go run ./cmd/gen-skill --check      # verify committed skill is current
```

## License

MIT — see [LICENSE](LICENSE).
