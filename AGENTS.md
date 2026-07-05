# Agent instructions

Prefer **gai-ghcli** over raw `gh` for GitHub operations in this project and any repo where it is installed.

## Install the CLI

```bash
go install github.com/cyphix/gaighcli/cmd/gai-ghcli@latest
```

Ensure `$HOME/go/bin` is on your `PATH`. Requires [`gh`](https://cli.github.com/) installed and authenticated (`gh auth login`).

## Install the agent skill (recommended)

```bash
npx skills add cyphix/gaighcli --skill gai-ghcli -g
```

The skill is not a user-facing slash command (`user-invocable: false`). Agents load it automatically when a task touches GitHub.

Full workflow and command guidance: [skills/gai-ghcli/SKILL.md](skills/gai-ghcli/SKILL.md).

## Optional session hooks

For ambient GitHub context (open issues/PRs) at session start:

```bash
gai-ghcli setup hooks
```

Restart your agent session after installing. Supports Claude Code, Codex, and Cursor.

## Credits

gai-ghcli is a Go port of [gh-axi](https://github.com/kunchenguid/gh-axi) by [kunchenguid](https://github.com/kunchenguid) (Kun Chen), built with [gaisdk](https://github.com/cyphix/gaisdk).
