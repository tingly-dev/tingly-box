# MCP: overview and two separate tool layouts

The navigation has three secondary entries: connections overview, Tool and
Server Tool. The overview describes shared connections and their actual use;
the two tool pages retain separate work surfaces for their execution purposes.

## Responsibilities

- Overview (`/mcp`): connect and repair built-in/external services, show health
  and actual client/gateway destinations, and inspect the effective route graph.
  Scope summary cards link to the tool pages; client access editors and full tool
  inventories are not rendered in this overview.
- Tool (`/mcp/tools`): tools called by MCP clients, client permission review,
  executable installation commands and real SDK verification.
- Server Tool (`/mcp/server-tools`): tools executed during model requests by the
  gateway, plus Advisor consultation model configuration. No client access editor
  or installation commands are rendered here.

## Shared connection, independent purpose

Store each connection once. Origin (built-in/external), transport and execution
purpose remain independent dimensions. An ordinary tool can be used in both paths.
The tool pages and their source panels expose only the current purpose control;
changing one preserves the other. Shared connection enablement explicitly says
it affects both pages. Global per-tool enablement stays in the overview source
panel. Removing a shared connection explains that both tool groups are affected.
Adding a source directly from Server Tool defaults to gateway-only usage.

Advisor requires model conversation context. It remains excluded from Tool and
from client grants. In Server Tool, Configure Advisor model opens its dedicated
consultation settings directly; standalone testing remains disabled. Model
continuation must be verified separately from saved-client SDK discovery.

## Navigation and state

Overview graph actions and summary links open the appropriate secondary page.
Connection-to-client assignment opens Tool with a named, unsaved grant review.
The user saves before any client grant takes effect. Commands and the real probe
remain in that client panel. Closing a panel stays on its current page.

Legacy source/route links open the overview; client links open Tool; old section
queries select the corresponding page. Existing identifiers stay fixed so saved
commands remain valid. Navigation exposes the selected page with aria-current.
Persistent tool catalogs refresh when saved source settings change; generation
checks ignore late discovery results. Connection edits preserve usage, global
limits and policies by omitting those unrelated fields.

New client access starts empty. Explicit empty grants do not revert to legacy
shared access. Wildcard source revocation preserves other grants. Failed sources
remain alongside healthy ones, and disabled execution retains configuration while
preventing runtime verification.

## Verification and artifacts

Verify the actual three-entry layout and selected navigation, page-specific
inventories and controls, source changes that preserve the other purpose, catalog
refresh after a source-panel edit, real SDK grants/calls/revocation, Advisor context,
failed/disabled/empty states, legacy links and mobile navigation. Keep all screenshot
stages, including failed captures, with checksums. Archive this round separately;
previous review packages and previews remain available as historical evidence.
