# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

### Added

- Agent Skill at `skills/gai-ghcli/SKILL.md` installable via `npx skills add cyphix/gaighcli --skill gai-ghcli`
- `cmd/gen-skill` generator and drift check (`--check`)
- [AGENTS.md](AGENTS.md) for project-level agent instructions

## [0.1.0] - 2026-07-04

### Added

- Initial Go port of gh-axi as `gai-ghcli` using gaisdk
- All 13 command domains: issue, pr, run, workflow, release, repo, label, secret, variable, search, api, setup
- TOON output, contextual suggestions, structured errors, idempotent mutations
- Session hooks for Claude Code, Codex, and Cursor via `setup hooks`
- Self-update via built-in `update` command (`go install`)
