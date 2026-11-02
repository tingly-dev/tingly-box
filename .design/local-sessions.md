# Read-only local Claude Code sessions

The optional `local_sessions` Power-up browses native history on the computer
running Tingly Box. It replaces none of the agent runtime, Projects threads,
Skills management, or gateway recording. The retired `/prompt/user` page stays
retired. This feature never starts/resumes an agent, executes a recorded tool,
or imports transcripts into Projects or a model context.

## Entry and scope

`/agent/claude_code/sessions` is a lazy route, linked from the Claude Code
sidebar and Power-ups. The feature flag defaults to false and is enforced by
both the route gate and every content endpoint. Closing the page cancels its
requests. No discovery runs at startup or on unrelated pages.

The source is `$CLAUDE_CONFIG_DIR/projects`, or `~/.claude/projects` when the
runtime environment has no override. The browser cannot set a root or supply a
filesystem path. The first version lists main `.jsonl` transcripts immediately
inside project directories. Subagent directories and session attachments are
not traversed. Sidechain/parent metadata present in a main transcript is kept;
records remain in file order rather than being presented as a reconstructed
single conversation or a claim about which branch is current.

## Local access policy

All endpoints live on the existing user-authenticated V2 group and are marked
authenticated in OpenAPI. Content additionally requires:

- an explicitly loopback-only server bind (`localhost`, `127.0.0.1`, `::1`);
- an actual loopback socket peer, checked without forwarded-IP trust;
- a loopback HTTP Host, no proxy forwarding headers, and no foreign Origin or
  cross-site fetch context. The known Wails desktop origins are accepted.

The GUI binds locally by default. CLI users must start with `--host 127.0.0.1`.
An instance listening on all interfaces cannot expose this viewer, even to a
request whose peer happens to be local. `status` returns only capability and
feature booleans and performs no filesystem discovery.

Loopback is a transport boundary, not proof of browser identity. A bridge that
fully disguises remote traffic as local traffic cannot be identified by these
HTTP checks. Do not publish this local API through a proxy, tunnel, or other
relay. Remote session browsing and multi-user permissions need a separate
capability/authorization design; this Power-up does not implement them.

## Filesystem and content handling

`internal/localsessions` is a separate reader, not the legacy
`agentboot/claude/session` store (whose pagination still reads entire files).
It uses `os.Root` for confinement, rejects symlinks and non-regular transcript
files, and checks opened descriptors against path metadata. Unix opens are
nonblocking so a regular-file-to-FIFO race cannot block a worker. On-disk
indexes supply display labels only: `cwd`, `fullPath`, tool paths and attachment
URLs never authorize a file read.

Project/session identifiers and cursors are authenticated with a per-process
random HMAC key. They are opaque API identifiers, not encrypted storage. List
cursors expire with their directory snapshot; content cursors are bound to a
specific file identity, size and modification time. Appends, truncation,
replacement or changes during a page read require refresh (HTTP 409). No
background watcher continuously reads an active session.

The frontend renders plain React text only: no HTML/Markdown execution, remote
images, file links, terminal escape interpretation, or attachment decoding.
Tools and thinking use collapsed details. Structured image/document payloads and unrecognized raw events are omitted.
Tool inputs and results remain capped text; they are not treated as attachments
or automatically redacted. No transcript text is copied into localStorage,
IndexedDB, a search database, telemetry, or model requests. Responses are
`Cache-Control: no-store`; the HTTP access logger records no response bodies.

## Explicit resource bounds

| Resource | Limit |
| --- | --- |
| Directory entries examined for one snapshot | 10,000 |
| Directory snapshots cached | 4, expiring after 5 minutes |
| Project/session entries returned | 25 per page |
| Session title prefix read | 8 KiB per displayed session |
| Project display index read | at most 8 KiB + one overflow-detection byte |
| Transcript bytes scanned per request | 2 MiB, including boundary look-behind |
| Record parsed | at most 1 MiB |
| Events returned/rendered | 50 per page |
| Block text | 4 KiB, with a truncation marker |
| Content blocks per event | 8 plus an omission notice |
| Encoded event JSON | 192 KiB per page; full envelope below 256 KiB |
| Concurrent content requests | 4 per handler; excess receives 429 |
| Content snapshots | 64, expiring after 10 minutes |
| Request deadline | 2 seconds, checked while enumerating/parsing |
| Frontend cursor history | 50 cursors per list/preview; pages replace content |

Directory discovery reads metadata and sorts a bounded snapshot by modification
time; it is O(number of entries), not a constant-time global index. Directories
above the limit return an explicit limit error. Further catalog scaling should
add incremental metadata indexing, not remove the cap or parse all transcripts.
The reader has no persistent database, extra dependency, or background worker.

Preview starts from the tail or beginning and navigates with byte cursors.
Oversized lines are skipped across bounded windows, with cursor state retaining
whether a window cuts through a record. A page can contain zero displayable
events and still have a next cursor. Metadata, malformed records and oversize
records are disclosed as omissions; the UI never claims a complete transcript,
exact turn count, completion status, cost, or full-text search result.

## HTTP contract and validation

- `GET /api/v2/local-sessions/status`
- `GET /api/v2/local-sessions/projects?cursor=...`
- `GET /api/v2/local-sessions/projects/:project/sessions?cursor=...`
- `GET /api/v2/local-sessions/sessions/:session/events?cursor=...&direction=backward|forward`

Generate `openapi.json` through the CLI swagger command and regenerate the
frontend OpenAPI types; never edit generated contracts by hand.

Tests cover normal pagination in both directions, 1 GiB sparse files, oversized
single records across windows, malformed/unrecognized records, bounded JSON,
file changes, cursor replay/scope/tampering, cancellation, symlinks and descriptor
types, untrusted index paths, direct/remote/proxy/origin guards, disabled access,
revocation, stale UI responses, escaped rendering, collapsed tools, and recovery.
`BenchmarkPreviewLarge` reports bounded per-request allocations independently of
the sparse transcript's total size; benchmark timings are machine-dependent.
