# Bundled agent skills

Skills Tingly-Box ships to agents (Claude Code, Codex, ...) so they can use
gateway capabilities without the user writing integration code.

## Layout

- Source of truth: `internal/skills/<name>/` (`SKILL.md` + `scripts/`).
- Embedded next to the web assets: `internal.SkillAssets` (`internal/assets.go`).
  `go:embed` does not follow symlinks, so the real files live here.
- The repo uses them through symlinks: `.claude/skills/<name>` →
  `../../internal/skills/<name>` (`.agents/skills` already links to `.claude/skills`).
- Install primitive: `config.InstallSkill` / `InstallClaudeSkill`
  (`internal/server/config/apply_skill.go`), same managed-write semantics as the
  hook scripts: rewrite only changed files, keep files the user added.
  Not wired into any flow yet — where it is offered (agent apply flow, CLI) is a
  separate UX decision.

## imagegen

One skill for both generation and edit, not two: users think "an image", and
generate → edit → edit chains are the common case; both share endpoint, token,
model and output handling, and near-duplicate descriptions would compete when
the agent picks a skill.

Credential resolution is deliberately shallow: flags → `TINGLY_IMAGE_*` → the
agent's own Tingly-Box connection (`ANTHROPIC_BASE_URL`/`OPENAI_BASE_URL` with
`/tingly/<scenario>`, token reused, scenario swapped to `imagegen` because agent
scenarios such as `claude_code` do not declare `TransportImageGen`). Anything
missing or rejected exits 2 with `NEED_INPUT:` and the agent asks the user — the
script does not read config files, port files or guess defaults.
