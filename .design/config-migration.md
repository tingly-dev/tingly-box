# Config Migration Pipeline

`internal/config/migration.go` runs a pipeline of `Migrate` steps on
every boot to repair/evolve `Config` (`~/.tingly/config.json`) across
releases. After many iterations the step list only ever grew — this doc
defines the classification and retirement policy that keeps it bounded.

## Three kinds of step

Every entry in `migrationSteps` (`migration.go`) is tagged with a
`migrationKind` so its lifecycle is visible without reading the body:

| Kind | Runs | Purpose | Example |
|---|---|---|---|
| `kindBaseline` | every boot, forever | repairs a structural invariant of the *current* config model | `normalizeLegacyConfigBaseline`, `normalizeBuiltinRuleIdentity` |
| `kindDated` | every boot, unconditionally, idempotent via its own internal guard | a one-off data repair tied to a specific change | `migrate20260712` |
| `kindOnce` | exactly once, gated by a `MigrationsCompleted` marker | seeds/changes a value the user might deliberately override afterward | `defaultXcodeSkipUsageOnce` |

Use `kindOnce` whenever a migration sets a default a user could reasonably
turn back off — a marker-gated migration won't re-clobber that choice on the
next boot. Use `kindDated` for pure data repairs where re-running forever is
harmless and cheap (missing field backfill, renames, dropping now-invalid
data) — no marker bookkeeping needed. Baseline steps that *seed* built-in rules (`ensureCurrentBuiltinRules`) follow the
same principle: each template is offered once, tracked by a
`builtin-rule-seeded:<uuid>` entry in `MigrationsCompleted`, so a rule the user
deleted is not resurrected by every upgrade (#2402). `kindBaseline` is reserved for the
handful of normalizers that define what a *valid* config looks like today;
new ones are added rarely, when the baseline itself changes.

`Migrate(c)` runs every step and calls `c.Save()` once at the end if any step
reported a change, rather than each step saving independently.

## Retiring a `kindDated` step

A dated step is not permanent. Once it's safe to assume no config in active
use predates it, fold its logic into `normalizeLegacyConfigBaseline` (or a
future baseline snapshot) and delete the step + its dedicated tests, keeping
only baseline-level coverage of the config shape it produces.

**When to fold:** the step has been shipped for long enough that no
supported client is expected to still be on a pre-migration config — as a
rule of thumb, a couple of minor releases or a few months, whichever the
team judges safe for this product's release cadence. This is a judgment
call made at fold time, not a hard timer enforced by code.

**How to fold (checklist):**
1. Confirm the step's guard condition can no longer trigger in practice (or
   accept that it's now dead weight either way).
2. Move its effect into `normalizeLegacyConfigBaseline` (call it from there,
   or inline the logic if it composes naturally with an existing helper).
3. Remove the entry from `migrationSteps` and delete the standalone function.
4. Delete the step's dedicated test cases; keep/add a baseline-level test
   that exercises the same input shape through
   `normalizeLegacyConfigBaseline` instead.
5. If the step referenced a legacy lookup table (see below), re-check
   whether that table can now be narrowed.

`normalizeLegacyConfigBaseline` itself is the precedent: it already folds
every pre-2026-04 repair migration into one baseline pass.

**Folded so far:**
- pre-2026-04 config repair migrations (original baseline)
- 2026-04/05 batch, folded 2026-08-11: `migrate20260416` (multi-tenant
  defaults) → `normalizeMultiTenantDefaults`; `migrate20260421` (profile
  unified model `*`→`cc`) → `normalizeClaudeCodeProfileUnifiedModel`;
  `migrate20260502` (drop smart_guide wildcard rules) →
  `dropSmartGuideWildcardRules`; `migrate20260518` (Codex endpoint mode
  backfill) → `normalizeCodexEndpointMode`. `migrate20260712` was left as
  `kindDated` — too recent (shipped ~1 month prior) to assume no active
  config predates it.

## Legacy UUID lookup tables

`legacySimpleRuleUUIDs` and `legacyCCRuleUUIDs` (`builtin_rules.go`) map old
built-in rule UUIDs to their modern `builtin:<scenario>:<tier>` form. They
are read from three places: `findRuleByUUID`, `defaultRuleByUUID`, and
`canonicalRuleUUID` (used by `normalizeBuiltinRuleIdentity`, a
`kindBaseline` step — see `.design/rule-uuid.md` for the full UUID
convention).

These tables have their own, slower-moving lifecycle, decoupled from any
single dated migration:

- They stay as long as `normalizeBuiltinRuleIdentity` is a `kindBaseline`
  step self-healing renames from old configs — which is indefinite, since
  a config restored from an old backup should keep working.
- An individual entry can only be removed once **both** (a) the dated
  migration that originally performed the one-time rename for that UUID has
  been folded into baseline or deleted, **and** (b) no runtime fallback
  outside migration code (e.g. `generateCCEnv`, `tbclient` resolvers) still
  reads the legacy constant for configs loaded without migration.
- Removing a table wholesale (rather than narrowing per-entry) is only safe
  once `normalizeBuiltinRuleIdentity` itself is considered no longer
  necessary — i.e. every supported config is guaranteed to already be on
  canonical UUIDs. That is a much longer horizon than folding a single dated
  step, and should be treated as its own decision, not a side effect of
  folding one migration.

No automatic scan enforces any of this — retirement is a manual, reviewed
decision, made using the criteria above.
