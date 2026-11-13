# Frontend

Supplements the root `CLAUDE.md`. Covers frontend-specific build/bundle conventions.

## Code-splitting

Every route element in `src/routes/appRoutes.tsx` (the route table `App.tsx` renders) must be `React.lazy(() => import('./pages/...'))`, not a static import — except `Login`, the only screen reachable before auth. Even `Onboarding` is a normal post-auth route (picked at runtime by `OnboardingGate`), so it's lazy too. Adding a new page means adding a new `lazy()` entry, not a static `import`.

**Never export nav-level shared state from a page file.** If something outside `pages/**` (layout, sidebar, contexts, a widget used on another page) needs a constant/hook that happens to live in a page component's file, importing that one named export still pulls in the *whole module* — including the page component and everything it imports — because ES modules are the code-splitting unit. This silently defeats `lazy()` for that page.

- Example fixed in this codebase: `layout/useActivityItems.tsx` (nav) needed `SCENARIOS`/`useHiddenScenarios`, which used to live in `pages/scenario/AgentOverviewPage.tsx`. That single import kept the whole page (`PageLayout`/`PageHeader` chain included) in the eager bundle. Fix: moved the shared metadata into `pages/scenario/scenarioRegistry.tsx`, a page-free module both the page and the nav import from.
- If a page needs to expose something to eager/shared code, put that thing in its own small module next to the page, not in the page's own file.

**`vite.config.ts`'s `manualChunks` should only group code that's genuinely needed eagerly** (currently: MUI, since `Layout`/`App` use it directly). Do not add a manual grouping for a library that's only ever reached through a lazy page (e.g. `recharts`, only used by `DashboardPage`/`UserUsagePage`) — forcing it into a named vendor chunk makes the bundler treat that chunk as always-needed and preload it from `index.html` on every page load, even for users who never visit that page. Let Rollup fold route-only dependencies into automatic shared chunks instead; those are only fetched when a page that needs them is actually visited.

**How to verify a page (or a dependency) is actually lazy:** `pnpm build`, then check `dist/index.html`'s `<link rel="modulepreload">` list. Anything in that list downloads on every single page load, regardless of whether the code that uses it is wrapped in `lazy()` — that list is the real "what ships on first paint" answer, not the presence of `lazy()` in the source.

**`vite.config.wails.ts`'s `manualChunks`/`optimizeDeps` must stay in sync with `vite.config.ts`.** They're separate config files (Rollup can't share a function across two `defineConfig` calls), so a fix applied to one doesn't propagate to the other — this has drifted before (the wails config kept forcing `recharts`/`d3` into an eager `recharts-vendor` chunk after the web config was fixed to leave them lazy). When you change one file's chunking/`optimizeDeps` logic, apply the same change to the other, or explain in a comment why they intentionally differ.

## Reuse existing hooks instead of hand-rolling

These get reinvented locally often enough that it's worth naming explicitly:

- **Copy-to-clipboard feedback** (a "copied" flag that auto-resets after ~2s): use `hooks/useCopyFeedback.ts`, not a local `useState` + `setTimeout`. It also takes an optional `onCopied` callback for a toast, and a `reset()` for clearing the flag early (e.g. on dialog close).
- **Toast/snackbar notifications**: use `useNotify()` (`hooks/useNotify.ts`) — it renders globally via `NotificationProvider`, so there's no local `Snackbar`/`Alert`/open-state to wire up. Don't add a component-local `Snackbar`.

## Style literals

Colours, text sizes, font weights and corner radii come from the theme in `@tingly/ui` (palette and tokens, typography variants or `fontSizes`, `radius`) — see `.design/visual-system.md` §2. `pnpm lint` runs `scripts/check-style-literals.mjs`, which fails when a file outside the theme gains a hex / `rgba()` colour, a `rem`/`px` font size, a weight outside 400/500/600, or a literal `borderRadius`. Existing literals are frozen in `scripts/style-literals-baseline.json`. When one is genuinely deliberate (a chart series, a brand colour, an overlay on an image), run `node scripts/check-style-literals.mjs --update` and say why in the PR; after removing literals, run it too so the baseline tightens.

## Type checking

`pnpm typecheck` runs in CI (`release-cli.yml`) as a non-blocking, `continue-on-error` step — the repo has pre-existing legacy type errors that aren't being fixed as part of unrelated changes, so this only warns instead of failing the build. Still run `pnpm typecheck` locally before opening a PR and fix anything your change introduces; don't rely on CI to catch it.

`src/client/schema.d.ts` is generated and git-ignored. Run `pnpm gen:api` first in a fresh checkout or worktree: without it every API type is missing and `pnpm typecheck` reports ~140 bogus errors (implicit `any` in the MCP pages, quota types, bot types); with it the tree is clean.
