# WorkBuddy as an agent (client of Tingly-Box)

WorkBuddy (Tencent) on the *client* side: the WorkBuddy app sends its chat
requests to Tingly-Box through its custom-model setting, and Tingly-Box routes
them by the `workbuddy` scenario's rules. Same shape as Pi/Xcode: a scenario, a
guide page and an Auto Config. Independent of the WorkBuddy *provider* (signing
in to a WorkBuddy account and serving it upstream), which is
[`workbuddy-oauth.md`](./workbuddy-oauth.md). Shown by default like every agent
but Cursor (`scenarioRegistry.tsx` `DEFAULT_HIDDEN`).

- **Backend:** `typ.ScenarioWorkBuddy = "workbuddy"`, OpenAI transport,
  registered in `BuiltinScenarios()` / `scenario_registry.go`; endpoint
  `/tingly/workbuddy` (`tbclient`). Route mounting is generic
  (`/tingly/:scenario[/v1]/...`), so nothing else is needed.
- **Frontend:** `UseWorkBuddyPage` (`AgentPage` descriptor, guide-type setup),
  `WorkBuddyConfigModal`, nav entry, `/agent/workbuddy` route, `workbuddy`
  i18n (en/zh/ru), icon (`assets/icons/workbuddy.svg`, the official logo from
  workbuddy.ai, full colour), guardrail scope label, mock rule.
- **User-facing steps:** WorkBuddy → Settings → Model → Add model → Custom;
  URL = `{base}/tingly/workbuddy/v1/chat/completions`, API key = Tingly-Box
  token, Model ID = a request model from Model Rules; tick *Tool calling* only
  unless the routed model supports images/reasoning. This is the path WorkBuddy's
  own docs describe (Settings UI; with "Custom Protocol" off it validates /
  completes the `/chat/completions` path).
- **Where WorkBuddy reads custom models — evidence (re-checked 2026-10-12):**
  - *Official, `~/.codebuddy/models.json`* (user) and `<workspace>/.codebuddy/models.json`
    (project, wins on same `id`): Tencent Cloud "WorkBuddy Enterprise 模型配置"
    (cloud.tencent.com.cn/document/product/1831/137010) and workbuddy.ai
    `/docs/cli/models`. Schema: top-level `models[]` + optional `availableModels[]`;
    fields `id`(required) `name` `vendor` `apiKey` `url` `maxInputTokens`
    `maxOutputTokens` `temperature` `api` `supportsToolCall` `supportsImages`
    `supportsReasoning` `relatedModels`; `url` must be the full endpoint
    (`…/chat/completions`); `${ENV}` substitution; **hot reload** (1 s debounce).
    Release notes: custom models in `.codebuddy/models.json` since 1.2.8/1.2.9
    (2025-11); 4.9.12 added a `modelId` field; 4.10.0 (2026-07-10) added "model
    config sync" and fixed an `availableModels` whitelist bug "when backend config
    exists". **Scope caveat:** these pages describe *CodeBuddy Code* (CLI/IDE) under
    WorkBuddy Enterprise, not explicitly the personal WorkBuddy desktop app. The
    WorkBuddy desktop doc only says models "previously configured via
    `~/.codebuddy/models.json`" still work and can be managed in the UI.
  - *Third-party, `~/.workbuddy/models.json`* (LongCat, UniKey, CSDN guides):
    same `{"models":[…]}` shape, "restart WorkBuddy fully", UTF-8 no BOM, and the
    LongCat page labels the GUI route "新版推荐" (recommended for newer versions).
    Tencent Cloud's personal doc says only "`workbuddy/models.json`" (no dot).
  - **No release note or doc announcing a move/rename of the file was found.**
    The two directories most likely differ by product (CodeBuddy vs the WorkBuddy
    desktop app) rather than by version, but that is inference, not confirmation.
  - **Re-checked 2026-10-18.** Tencent Cloud's WorkBuddy model page
    (cloud.tencent.com/document/product/1831/134445) says the settings UI saves to
    "本地 workbuddy/models.json" and, separately, that models "已通过
    ~/.codebuddy/models.json 配置的自定义模型在界面升级后仍可正常使用" — so the
    desktop app has its own UI-managed file and *also* reads the legacy
    `~/.codebuddy` one. workbuddy.ai's Model page says the same in English.
    Consequences: writing `~/.codebuddy` stays the documented route; an id that
    the user added through the UI (the other file) is not seen by our
    collision check; hot reload is documented for the CLI only, so the UI copy
    tells the user to reopen WorkBuddy rather than promising a reload.
    DeepSeek's official WorkBuddy guide uses `C:\Users\<user>\.codebuddy\models.json`
    on Windows and warns that a UTF-8 BOM breaks loading, so Auto Config reads a
    BOM-prefixed file and writes without one.
  - **All ids taken → refuse.** When every rule's id already belongs to a user
    model, Auto Config writes nothing (no backup, stale managed entries kept)
    and says so, instead of reporting success for a file that gained nothing.
  - ~~Decision (2026-10-12): Auto Config writes `~/.codebuddy/models.json`~~ —
    superseded below.
  - **Verified on a real Windows install (2026-10-21), and the current decision.**
    The desktop app's settings UI keeps custom models in one file per edition:
    China edition `%USERPROFILE%\.workbuddy\models.json`, international edition
    (workbuddy.ai) `%USERPROFILE%\.workbuddy-ai\models.json`. Both editions can be
    installed side by side, each with its own folder. The file is a **top-level JSON
    array** of model entries (same fields as above, plus e.g. `useCustomProtocol`),
    not the documented `{"models": [...]}` object. So Auto Config:
    - detects installed editions by their config folder and writes **every**
      installed one (no edition picker); with neither folder present it writes
      nothing and says "WorkBuddy not found — open it once" rather than inventing
      a folder;
    - handles each file independently (one broken file doesn't block the other;
      the overall result succeeds only if all did) and returns per-file results
      (`files[]`) so the UI names exactly what was written;
    - reads/writes both shapes — keeps an existing object as an object (with the
      `availableModels` logic), keeps an array as an array, creates new files as an
      array;
    - status is "applied" only when every installed edition matches; restore rolls
      back each installed edition that has a backup and succeeds only when all of them
      were restored (a partial restore reports what was restored and what failed).
    It no longer writes `~/.codebuddy/models.json` (that is CodeBuddy CLI/IDE's
    file); entries an older build left there are not cleaned up automatically.
- **Auto Config behaviour** (`internal/config/apply_config_workbuddy.go`,
  `configapply/handler_workbuddy.go`; routes `GET /config/workbuddy/status`,
  `GET /config/preview/workbuddy`, `POST /config/apply/workbuddy`,
  `POST /config/restore/workbuddy`): one entry per active WorkBuddy rule
  (`id`/`name` = request model, `vendor` "Tingly Box", `url` =
  `{base}/tingly/workbuddy/v1/chat/completions`, `supportsToolCall` true, images and
  reasoning false). Ownership is by URL (`/tingly/workbuddy/`) — the schema has no
  metadata field — so only our entries are replaced; user entries and unknown
  top-level keys are preserved. An id already used by a user entry is skipped and
  reported, not overwritten. `availableModels`, when present and non-empty, is kept
  consistent (stale managed ids dropped, written ids added) because it filters the
  picker after merging. Refuses (touches nothing) if the file is neither a JSON array nor an object;
  backs up first; atomic write; keeps existing permissions, new file 0600 (holds an
  API key). No rules → nothing is written and the user is told to add a rule (no
  phantom default model). Not done: the `agent apply/restore workbuddy` CLI path
  (`ai/agent`, a separately versioned module that intentionally duplicates the
  writer) — add it if the CLI needs parity.
- **Unverified end to end:** SSE/tool-call behaviour against the routed models.

## Sources

- https://cloud.tencent.com/document/product/1831/134445
- https://www.workbuddy.ai/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Model
- https://api-docs.deepseek.com/zh-cn/quick_start/agent_integrations/workbuddy

- https://cloud.tencent.com.cn/document/product/1831/137010
- https://www.workbuddy.ai/docs/cli/models
