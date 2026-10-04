# MCP usage relationships — independent revision

Base: `feat/mcp-section-layout` (`d0bf86fc`). This document describes only the
usage-relationship change. The overview, Tool and Server Tool secondary layouts
remain the entry points; previous design and review documents are retained.

## Questions and entry points

The connection destination in the overview opens `/mcp?relationship=<source-id>`.
Tool cards and source panels provide the same contextual link. The overview's
collapsed **Usage relationships** section offers either all paths or one source.
A focused source answers: who can use it, why each tool is unavailable, and who
shares the saved configuration. A missing source bookmark is explicitly reported.

The ordinary path is client → MCP access → source. The gateway path is model
request → Server Tool execution → source; Advisor additionally shows its real
consultation provider and model. No client commands or grant editor are embedded
in the relationship diagram; node actions open their corresponding secondary
pages. Source nodes in the all-path graph focus the selected connection.

## Effective state and impact

Overview and graph consume one controlled routing snapshot. Refresh, connection
saves and client saves invalidate the same projection; the graph does not run a
second independent discovery request. Positive edges and counts come only from
the backend's effective client/server projections, gated by execution, source
health and tool policy. Saved wildcard grants cannot manufacture an available
edge. Solid nodes contain available tools; dashed nodes have none confirmed.

Per-tool explanations distinguish runtime off, shared connection off, failed or
unfinished discovery, global tool policy, source allow list, purpose, disabled
client, source grant and tool grant. Advisor needs gateway conversation context.
Unknown discovery has no inferred tool inventory or positive reachability count.

The impact section separates saved client associations from current availability.
Disabled configurations may remain associated. Connection settings and global
policies affect both paths; purpose changes affect only their own group; a client
grant affects only that client. This is configuration visibility, not call history
or a guarantee that a future tool call will succeed.

## Verification and version evidence

Verify matching real SDK tool discovery/calls and graph counts; revoke each
purpose independently, grant an explicit tool subset, disable clients/sources/MCP,
fail and repair a connection, and review Advisor. Check refresh and focused
bookmarks, missing IDs, scoped configuration links and mobile horizontal bounds.
All screenshots, including failures and drafts, stay in this revision's artifact.
Create a separate review Page for this revision, linking prior evidence without
rewriting the prior review document.
