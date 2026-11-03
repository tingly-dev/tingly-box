# Coordinator as a Claude Code session

The coordinator currently reconstructs a bounded model conversation from UI
events for every turn. Tool calls and results disappear between turns and the
changing system prompt makes prefix reuse difficult. Each project should instead
own one Claude Code session, resumed in a stable project directory. Claude Code
owns the transcript, tool loop and compaction; Projects owns project state, the
visible conversation, routing, and the serial queue.

## Tool boundary

Extend the existing loopback Projects MCP service with a capability scoped to
the main thread. Register the existing coordinator tools from their schemas and
execute them through the existing service implementation. Validate project
ownership, pause/archive state and configuration at execution, and retain the
existing citation validation and UI events. Workers retain only their existing
thread-scoped tools. The capability URL must not be placed in process arguments
or exposed in transcripts.

The coordinator reads shared library and memory files through scoped MCP tools.
They accept attachment paths, return bounded UTF-8 text or supported images, and
use rooted filesystem access so symlinks cannot escape the shared directory.
The coordinator does not gain general filesystem or shell access.

`no_update` currently terminates the custom loop immediately. An MCP tool
response alone does not terminate Claude Code's turn: the session adapter must
record the outcome and suppress subsequent text for a report-only turn, while
rejecting it for a user-triggered turn. Do not enable this path until that
behavior and the active-turn settings snapshot are wired through the server.

## Session and delivery

Persist a coordinator session identity and use a stable cwd. On first use,
bootstrap existing projects with clearly marked recent history and current
project state; subsequent turns send only newly queued inputs. Resume the same
session after process eviction and server restart. Preserve source envelopes for
user messages, settings changes and untrusted thread reports. Project events are
a UI projection, never replayed as the authoritative Claude transcript.

Queue delivery needs durable input identifiers and an explicit recovery policy:
the Claude transcript, project database and external tool side effects cannot be
committed atomically. Do not silently replay an ambiguous interrupted turn or
claim exactly-once execution; make ambiguity visible and add idempotency for
side-effecting tools before automatic retry. Preserve settings-change checks,
streaming, decisions, citations, notifications, cancellation and host profile
selection. A missing CLI or incompatible connection must yield a visible error,
not silently switch to a new empty session.

## Verification

Exercise real MCP schema discovery and calls, scope and configuration checks,
session resume after restart, concurrent incoming messages, interrupted tool
calls and migration of existing projects. Verify cache read/write usage against a
real compatible upstream: a persistent session does not guarantee a cache hit.
The rollout must retain the standalone Projects module boundary and avoid new
user-facing mode switches.

## Implementation status

The coordinator runs via agentboot with a per-main-thread session ID. The
loopback service gates its tools on an explicitly active turn. User messages,
thread reports and settings changes are recorded with ordered delivery entries;
startup marks interrupted deliveries uncertain and resumes pending queues. The
recovery endpoint resolves an uncertain input by explicit retry or skip. The
project conversation shows the recovery choices and explains that retry may
repeat prior actions. The offline probe runs the real Claude Code CLI against a
scripted Anthropic endpoint, exercising MCP and session resume. Actual upstream
cache metrics still depend on the configured gateway and provider.
The standalone coordinator requires an Anthropic-compatible connection or a
Claude Code profile; an OpenAI-style connection is rejected explicitly rather
than silently falling back to a different login. The scoped reader supports
UTF-8 text up to 256 KiB and images up to 5 MiB; delegate other files to a
worker thread.
