# Independent Search and Fetch backend settings

Approved scope: expose the existing `WEB_SEARCH_BACKEND` and `WEB_FETCH_BACKEND`
configuration in Server Tools. No new backend API or billing changes.

- Search: Auto / DeepSeek / Serper; Fetch: Auto / DeepSeek / Jina Reader.
- Preserve the existing Auto resolution for compatibility and explain it explicitly:
  a source Serper key selects Serper/Jina; otherwise an available DeepSeek provider
  selects DeepSeek; without either, Search is unavailable and Fetch uses Jina.
  Explicit selections are independent of the other tool and its credentials.
- Show the applied backend and credential readiness per tool from the routing
  snapshot, never guess from draft settings. Mark edits as unsaved and label the
  snapshot as saved configuration. Missing discovery metadata means unknown.
- Serper credentials remain source-local; switching backends must not erase them.
  Show the key field for Serper/Auto; DeepSeek links to the Credentials page and
  reuses enabled first-party credentials. Jina uses the existing anonymous Reader.
- Preserve unrelated environment values, source enabled state and tool policies.
- Keep the overview explicit: Search → backend / Fetch → backend.
- Verify independent persistence, unknown/disabled/not-ready states, save errors,
  preserved credentials, localized copy, frontend screenshots and `task cli:build`.
