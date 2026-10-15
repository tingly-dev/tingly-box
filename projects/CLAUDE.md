# Projects (tingly-project)

A local replica of Claude's redesigned Projects: one conversation with a
coordinator that runs parallel Claude Code threads in git worktrees, on top of
Tingly Box. It lives in `projects/` of the tingly-box repo as its own Go module
(`github.com/tingly-dev/tingly-box/projects`) and still builds on its own as the
`tingly-project` binary. Same stack and conventions as tingly-box; this file adds
what is specific to Projects.

Research lives in `.research/`, design in `.design/`. Run tasks from the repo
root as `task projects:<name>`, or as `task <name>` inside `projects/`.

# Guideline

## UX-First (highest priority)

The product comes first. When UX conflicts with implementation convenience, push
UX further. **Before building or changing a feature, read
`../.design/ux-principles.md`** (shared with Tingly Box) and check the change
against its checklist.

**Before changing project behaviour, read `.research/claude-projects.md`
(what Claude's Projects are and how people use them) and `.design/projects.md`
(what we replicate, the MVP cut, and the architecture).** Keep parity with the
official experience where the design marks it in scope, and keep vocabulary
fixed: Project, Conversation, Coordinator, Thread, Overview, Library, Memory,
Instructions, Goal.

## Backend

- HTTP modules live in `internal/server/module/*` and implement `module.Module`.
  `internal/server/server_modules.go` is the only place modules are wired; the
  server and OpenAPI generation both use it.
- Errors use one shape, `{"error": "..."}`, via `module/apierr`.
- Metadata edits (rename, star, move) must not bump a conversation's
  `updated_at`; use the `*Meta` / `Set*` store helpers.
- LLM access goes through `llm` only (public: an embedding host passes its model client in through it).
- `projects` is the embedding facade (`New` / `Register` / `Start` / `Host`); this module never imports Tingly Box's root module (`task projects:boundary`, also run in CI).
- Projects routes all live under `/api/v1/projects`. Settings, Chats and health sit outside it and are standalone-only (`build` in `server_modules.go`).

## Frontend

The web UI is `frontend/packages/projects` (`@tingly/projects`) in the repo's pnpm
workspace; `task projects:frontend` builds it into `web/dist`, which the binary
embeds. Imports inside the package are relative (the workspace's `@/` belongs
to Tingly Box's app).

- Use MUI for components. Every page in `src/routes/appRoutes.tsx` is `lazy()`.
- Icons come from the package's `components/icons` (Tabler via `tablerMui()`); do not import
  `@mui/icons-material` directly.
- Toasts: `useNotify()`. Copy feedback: `useCopyFeedback()`.
- Every user-facing string goes through i18n, in both `en.ts` and `zh.ts`
  (`zh.ts` is type-checked against `en.ts`).
- `localStorage` only for per-browser conveniences, via `utils/storage`.
- The look is Tingly Box's `@tingly/ui` (theme, icons, `PageHeader`, `EmptyState`,
  `ConfirmDialog`, …), from `frontend/packages/ui`. Change it there; build pages
  from its primitives.

# Cross frontend and backend

`openapi.json` and `frontend/packages/projects/src/client/schema.d.ts` are generated — never
hand-edit them. After changing API models or routes:

```bash
task codegen   # go run ./cmd/tingly-project swagger + pnpm gen:api
```

# Pull Requests

Same as the repo root: follow its `.github/PULL_REQUEST_TEMPLATE.md`.

# Verify

```bash
task projects:test   # from the repo root
```
