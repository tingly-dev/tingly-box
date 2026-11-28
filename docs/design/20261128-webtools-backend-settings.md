# Independent Search and Fetch backend settings

Approved scope (updated after user clarification): no Auto choice. Configure each
operation as source first, then authentication. No Jina Search or billing changes.

- Search: DeepSeek / Serper. Fetch: DeepSeek / Jina Reader.
- DeepSeek shows a dropdown of configured, enabled first-party API-key providers.
  Store only the selected UUID in `WEB_SEARCH_PROVIDER_UUID` or
  `WEB_FETCH_PROVIDER_UUID`; resolve credentials live on every call. Missing,
  disabled, or invalid selected providers must not fall back to another account.
- Backend choices use `WEB_SEARCH_BACKEND` and `WEB_FETCH_BACKEND` independently.
  Unset legacy choices retain the original Serper Search / Jina Fetch defaults;
  the obsolete `auto` value also maps to those defaults, never switches on keys.
  Direct DeepSeek passthrough endpoint resolution is unchanged.
- Serper key is source-local. Preserve hidden keys and provider selections when
  switching backends. Jina Reader requires no configuration in this integration.
- Show saved backend, credential source, and credential readiness per operation
  from the routing snapshot. Draft edits must not masquerade as applied state.
  Unknown discovery state is shown as unknown, not ready. Source/tool activation
  remains separate from credential readiness.
- Preserve unrelated env values, source enabled state and tool policies. Use
  existing Provider and MCP APIs; no generated schema changes required.
- Verify independent backend/account selection, credential rotation/disable,
  no fallback or secret duplication, UI persistence/error states, localized copy,
  screenshots, and `task cli:build`.
