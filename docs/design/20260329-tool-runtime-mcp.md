# Tool Runtime MCP Refactor

## Summary

This change replaces the old `tool_interceptor` path with a generic `tool_runtime` centered on MCP-style tool sources.

The runtime now supports:
- builtin `web_search`
- builtin `web_fetch`
- stdio-backed MCP servers
- unified tool injection and follow-up execution for OpenAI, Anthropic, and Google-backed Anthropic flows

## Why

The old interceptor path had three problems:
- it was specialized around search/fetch and could not scale cleanly to arbitrary tools
- provider handling was fragmented, especially once Google support was added
- builtin tools and external tool servers had different execution models

The new runtime addresses that by normalizing tool discovery, declaration, dispatch, and result handling behind one abstraction.

## Design

The runtime has three layers:
- `ToolRuntimeConfig`: global and provider-level enablement plus source registration
- `Source`: builtin or MCP-backed tool source
- server follow-up orchestration: inject tools, detect tool calls, execute locally, continue upstream request

Builtin tools remain first-class runtime tools:
- `web_search`
- `web_fetch`

External MCP tools are namespaced as:
- `mcp__<source_id>__<tool_name>`

This keeps builtin tool names stable for models while avoiding collisions for external tools.

## Provider Routing

Provider-native tool support is decided per stable tool name rather than with a single global boolean.

Current policy:
- native `web_search` suppresses runtime injection of builtin `web_search`
- builtin `web_fetch` remains runtime-owned
- external MCP tools are still exposed unless separately filtered

This keeps the decision surface small while allowing future extension to more native tools.

## Builtin Web Tools

Builtin tools are now implemented inside `internal/toolruntime`.

### `web_search`

`web_search` uses runtime-owned search code and cache rather than the removed interceptor package.
It returns a structured payload with:
- `tool`
- `query`
- `result_count`
- `results`

### `web_fetch`

`web_fetch` now has explicit runtime-level controls:
- only `http` and `https`
- block localhost and private/link-local/loopback targets
- validate redirects on every hop
- bound response size
- bounded timeout
- structured result payload with:
  - `tool`
  - `url`
  - `final_url`
  - `status_code`
  - `content_type`
  - `truncated`
  - `content`

## Compatibility

This refactor intentionally removes the old `tool_interceptor` runtime path and naming from the active code path.

Configuration is now expressed in terms of:
- `tool_runtime`
- builtin source config
- MCP source config

A transitional read-only fallback remains for the old `tool_interceptor` key so existing installs can be read and translated into `tool_runtime`.

## Migration

Existing configuration should be migrated as follows:
- top-level `tool_interceptor` settings become `tool_runtime` with one builtin source
- provider-specific `tool_interceptor` records in `tool_configs` should be rewritten as `tool_runtime` records
- builtin search/fetch tuning fields map directly onto the builtin source config:
  - `search_api`
  - `search_key`
  - `max_results`
  - `proxy_url`
  - `max_fetch_size`
  - `fetch_timeout`
  - `max_url_length`

Legacy `tool_interceptor` config is still read for compatibility during the transition, but new writes should only use `tool_runtime`. Persisted installs should still be migrated forward so the fallback can be removed later.

## Testing

Coverage added in this change:
- runtime unit tests for builtin injection and provider-native suppression
- runtime unit tests for fetch URL policy and redirect blocking
- stdio MCP fixture for runtime tests
- server integration tests for OpenAI, Anthropic, and Google follow-up flows

## Follow-Up

Likely future work:
- richer tool filtering by provider and scenario
- native/runtime arbitration policy beyond `web_search`
- optional removal of the old persisted `tool_interceptor` records from storage through migration code
