# MCP Gateway Testing Guide

The MCP center has five pages: `/mcp/routes` shows effective routes,
`/mcp/tools` manages ordinary tools exposed to clients, `/mcp/server-tools`
manages tools executed in the gateway's model tool loop, `/mcp/sources` manages
upstream connections, and `/mcp/clients` manages downstream profiles.
The legacy `/tools/servertool` page redirects to `/mcp/server-tools`. Sources can use stdio, Streamable HTTP or SSE. Configuration and
connection checks remain available when MCP execution is disabled; tool calls
and downstream transport require the MCP scenario flag to be enabled.

## Route graph and actual client diagnostics

`GET /api/v1/mcp/routing` derives its client and server branches from the same
source allow lists, tool policies and explicit client grants used at execution.
`origin: builtin/external` is independent of these usages and of stdio/HTTP/SSE.
One external source can supply both ordinary tools and Server Tools; built-in
sources can do the same. Advisor requires model conversation context and is
restricted to Server Tools even when legacy client usage is configured. It is
not offered as an ordinary tool or standalone tool test. It is the special in-process branch and expands
into its configured consultation provider/model and return to the model loop.
Failed intended sources remain visible with no fabricated reachable tools.
The display projection omits connection credentials, environment values, command
arguments, URL user information and query strings, and raw transport diagnostics.
Source configuration and full tool test results remain in the authenticated
management API. Discovery indicates tool availability, not a successful model call.

`POST /api/v1/mcp/client-profiles/reader/probe` with `{}` checks initialize and
paginated tools/list through the actual authenticated local MCP HTTP endpoint.
It uses the configured server port and saved user authentication, never a caller
supplied Host or target URL. Sending a normalized `tool_name` and `arguments`
additionally executes that tool only when it is listed for this client. Revoked
or disabled grants cannot reach the upstream. A successful tool transport can
still return `result.isError: true`; inspect both fields.
This probe does not run model continuation. The harness verifies that separately.

Click an entry for installation instructions, the client gateway node for its
grants, a source for its status and editor, or the server execution node for the
separate Server Tools page. “Choose tools” adds one usage while preserving the
other. Execution-disabled routes remain configurable but cannot be probed.

## Configure a source without replacing existing sources

Use the existing user token for management and downstream MCP access:

```bash
TOKEN=$(jq -r '.user_token' ~/.tingly-box/config.json)
BASE=http://localhost:12580/api/v1
curl -s -X POST "$BASE/mcp/sources" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":"remote","transport":"http","endpoint":"https://example.com/mcp","tools":["*"],"usage":{"client":true,"gateway":true}}'
```

`usage.client` exposes eligible tools to MCP clients. `usage.gateway` injects
eligible tools into model requests and executes them in gateway tool loops.
Both can be enabled. Legacy `visibility: client/server` is the default when
`usage` is absent. The deprecated global `mode` is no longer a routing switch.

Use `PATCH /mcp/sources/remote` for partial edits and
`DELETE /mcp/sources/remote` to remove it. Omitted fields survive a patch;
explicit empty headers/env maps clear those maps. Source IDs cannot change.
`PUT /mcp/config` still supports bulk source replacement; omitted sources are
preserved when changing only timeout or stripping settings.

Connection fields (command, args, cwd, env, endpoint, headers and proxy) trigger
replacement of the old connection. Policy edits retain the connection.
Environment references resolve from the source's `env` map; to inherit a
process variable, explicitly set `env: {"TOKEN":"${TOKEN}"}` and reference
`${TOKEN}` in the header. Referenced variables must exist for enabled sources.

## Discover, check and reconnect

```bash
curl -s "$BASE/mcp/catalog" -H "Authorization: Bearer $TOKEN"
curl -s -X POST "$BASE/mcp/sources/remote/check" -H "Authorization: Bearer $TOKEN"
curl -s -X POST "$BASE/mcp/sources/remote/reconnect" -H "Authorization: Bearer $TOKEN"
```

The catalog returns status/errors per source and tool input/output schemas,
annotations, normalized names and effective usage. A failed source leaves
working sources visible. Reconnect closes the previous session and performs a
new connection and discovery.

Per-tool policy is keyed by upstream tool name:

```json
{"tool_policies":{"echo":{"enabled":true,"usage":{"client":true,"gateway":false}}}}
```

The source's enabled flag and `tools` allow list remain the outer limit.
An empty source allow list retains the legacy meaning of all tools.

## Configure downstream profiles

```bash
curl -s -X PUT "$BASE/mcp/client-profiles/reader" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":"reader","name":"Reader","enabled":true,"sources":["remote"],"tools":["tingly_box_mcp__remote__echo"]}'
```

A profile requires both a matching source grant and a matching normalized tool
grant. Empty lists grant no access; `*` explicitly grants all eligible entries.
Connect a standard MCP client to `$BASE/mcp/reader` using the user token.

Before any explicit profile is configured, legacy `tb`, `all` and known source
ID endpoints continue to work. Unknown endpoint names fail closed. Creating a
profile switches access to configured profiles only; deleting the last profile
keeps that mode and does not restore aggregate access. Disabled tools and
revoked grants are checked again at execution. Profiles currently share the
existing user-token authentication: profile IDs select grants and are not
separate authenticated identities. Independent credentials/OAuth are separate
future work.

Codex registration uses an environment variable for the bearer token:

```bash
export TINGLY_MCP_TOKEN="$TOKEN"
codex mcp add tb --url "$BASE/mcp/reader" --bearer-token-env-var TINGLY_MCP_TOKEN
```

## Test structured results

```bash
curl -s -X POST "$BASE/mcp/tools/call" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"source_id":"remote","tool_name":"echo","arguments":{"q":"hello"}}'
```

The response retains `result.content`, `result.structuredContent`, `result._meta`
and `result.isError`. Transport success and a tool-reported `isError` are
separate. The MCP downstream bridge preserves the supported content blocks;
model continuations receive structured content as JSON text and native image
blocks where supported. APIs without matching content types receive JSON text.

## Automated regression checks

```bash
go test ./internal/config ./internal/mcp/... ./internal/server/module/mcp
go build -o harness ./cli/harness
./harness matrix --mode=all --json
go test -race ./internal/...
cd frontend
pnpm gen:api
pnpm typecheck
pnpm test
pnpm build
```

`servertool` includes 48 real HTTP/SSE MCP loop cases spanning Anthropic/OpenAI
sources and targets, streaming modes, structured continuation and tool errors.
Runtime tests additionally spawn a real stdio server, replace connection/env
configuration, preserve policy-only connections, and reject retired sources.
Management tests exercise persistence, partial edits, validation, downstream
SDK interoperability and live grant revocation. Frontend tests cover form
round trips, partial discovery failure, tool tests, routes and failed-save
retention. Generate OpenAPI through the CLI before `pnpm gen:api`; do not edit
generated schemas manually.

The routing exposure harness adds 14 HTTP/SSE cases covering client-only,
server-only, dual, neither, revoked grant, disabled tool and source allow-list
exclusion. It compares the graph against a real authenticated SDK connection
and tools/list, then proves blocked calls never reach the real upstream handler.
An additional Advisor case verifies a legacy dual-use configuration exposes it
only in the model loop, with no callable tools in the ordinary SDK client.
Run `go test ./internal/protocoltest -run TestMCPRoutingExposure` for this subset.
