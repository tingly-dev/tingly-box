# MCP, Tool and Server Tool: ownership revision

Base: `fix/mcp-overview-title` (`41885459`). This independent document records
only the ownership change; prior version documents and evidence remain frozen.

- Tool owns source onboarding, connection/authentication, discovery, the complete
  tool catalog, schemas, isolated tool testing and shared enablement/policies.
  There are no client grants, installation commands or purpose switches here.
- MCP owns publication through the gateway, client grants, connection commands,
  actual saved-client SDK verification and the effective usage relationship view.
  It has no connection/authentication editor or shared/global tool switches.
- Server Tool owns model-execution eligibility and Advisor consultation settings.
  Adding or repairing a shared source opens Tool; publication is independent.

New sources created from Tool have explicit `{client:false,gateway:false}` usage.
Onboarding discovers definitions but does not publish them or change any client
profile. Wildcard clients therefore do not acquire a newly registered asset until
MCP publication is enabled. Direct administrative tool testing is still possible
for globally enabled ordinary tools, without granting client access. Advisor
requires conversation context and has no standalone asset test.

All settings use existing independent backend tool policies. Source edits omit
unrelated usage/policy fields; global tool toggles preserve both purposes; MCP
publication preserves execution eligibility; execution changes preserve MCP
publication and all grants. Shared connection deletion is available only in Tool
and explicitly explains effects on both consuming paths.

Existing profiles, grants, IDs and commands are retained. Saved client/profile/
install/grant-source links resolve to MCP; source links resolve to Tool; Server
Tool links resolve to its scope. MCP publication uses `?publish=<source-id>`,
Tool connection editing uses `?source=<source-id>`, and usage relationships remain
`/mcp?relationship=<source-id>`. Node actions open the owning page. Closing a
panel stays on that owner's canonical page. The same routing snapshot drives
MCP publication diagnostics and respects failed/disabled/unknown states.

Verify the three page boundaries, neutral onboarding against a wildcard SDK
client, Tool tests before publication, subsequent MCP publication and explicit
grants, independent purpose changes, shared disablement, source repair, Advisor,
old bookmarks and mobile. Archive all drafts/failures and exact production
captures in a separate review Page with full protocol harness evidence.
