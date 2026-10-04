# MCP: one workspace, continuous setup

The previous five destinations mirrored runtime entities. They required users
to understand the relationship between sources, tool policies, client profiles
and gateway execution before completing their first connection.

The workspace answers three questions in order:

1. What tools have I connected? Source cards use names as their primary label,
   show health and tool counts, and point to their actual client/model destinations.
2. Who uses them? Ordinary tools are called by downstream clients; Server Tools
   run inside the gateway model loop. These are separate scopes on the same page.
3. How do I start using them? Choosing client access and obtaining its executable
   installation command happen in the same panel, followed by the real SDK probe.

## Continuous actions

The navigation rail opens `/mcp` directly and has no secondary MCP page menu.
Connect tools opens name and remote address/local command fields. IDs are generated
and credentials remain available in advanced settings. Saving opens that source's
tools, with independent ordinary and Server Tools checkboxes. Choosing a client
preselects the source in an unsaved permission review. Nothing is granted until
the user saves. The resulting command and verification stay in that panel.

The relationship graph remains accessible below the primary work surface as a
collapsed diagnostic. Its existing actions open source/client/tool panels in place.
Legacy page paths and install/profile/source bookmarks remain compatible; closing
their panels returns to the workspace. Existing IDs never change, preserving
saved endpoints and installed client commands.

## Separate axes and truthful state

Built-in/external indicates connection origin. Ordinary/Server Tools indicates
execution purpose. A regular tool can use both paths through one shared connection.
Advisor depends on conversation context, exposes only the Server Tools path and
shows its actual consultation provider/model configuration. Standalone tool tests
are disabled for Advisor. Tool tests run through the existing authenticated runtime;
saved-client probes initialize and discover tools through the actual MCP endpoint.

Connection edits omit usage, enabled state and per-tool policy fields to avoid
overwriting changes made in the usage surface. Client wildcard source revocation
materializes grants before removing one source and preserves individual tool grants.
New clients start with no source access; explicit empty grants never fall back to
legacy shared access. Saving the first explicit client explains the legacy endpoint
transition before the save. Failed connections remain alongside working ones with
a repair action. Execution-disabled clients keep editable configuration and disable
runtime verification. Loading or stale routing is presented as pending, not success.

## Review evidence

Keep all draft, failure and final screenshots with stage manifests and SHA-256
checksums. The browser harness exercises real HTTP connect/discover/call, grant
review/save/revoke, empty access, disabled client/runtime, Advisor context, failed
connection repair, old bookmarks and responsive panels. It restores isolated demo
fixtures after each run. The protocol/model-loop matrix remains a separate check;
an SDK discovery result does not imply model continuation succeeded.
