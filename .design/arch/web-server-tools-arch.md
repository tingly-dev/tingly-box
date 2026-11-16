---
date: 2026-10-26
---
# Web Server Tools architecture

**Cache Level:** Sub-module
**Branch:** bugfix/fetch-search
**Baseline Hash:** 137c58e97

## Existing architecture

- `internal/mcp/builtin_server` owns Serper search / Jina Reader fetch, served by `mcp-builtin` over stdio.
- `internal/mcp/runtime/builtin_registry.go` registers `webtools` (MCP) and `advisor` (virtual Server Tool).
- Runtime policy, catalog and routing determine discovery and availability; source changes reconcile runtime caches.
- `internal/protocolserver/protocol_transform.go` assembles injection and native web-tool stripping. Supported server-loop routes must remain unchanged.
- `frontend/src/pages/mcp` shares cards, settings drawers and test dialogs across MCP Tools and Server Tools.

## Restoration boundary

The user confirmed separate Server Tool and MCP versions are acceptable, and the provided Server Tool version must be explicitly configured like Advisor. The implementation adds an independent `server-webtools` source, reusing the existing executable and handlers. It preserves the existing MCP source and configuration. Both web Server Tool policies start off; saving a search key does not enable them. Search readiness requires Serper credentials, while Fetch is independent.

Server Tool semantics must not be inferred from transport: stdio web tools can be gateway-owned without being virtual. Native definitions may be stripped only after the replacement was actually injected on a supported execution path. No protocol support expansion is part of this change.

## UX review

- One settings surface with actionable missing-key feedback, not a transport editor.
- Per-tool switches are independent; settings saves do not enable siblings.
- Show Serper / Jina Reader rather than the internal binary command.
- Explain external query/URL disclosure before enabling.
- Keep the MCP version's settings and profile behavior independent.
