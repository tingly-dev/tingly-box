# MCP display and component cleanup

Independent revision on `feat/mcp-ui-dedup`, based on `1af3a70a`.
Prior version design and review documents remain frozen.

Tool, MCP and Server Tool retain their distinct secondary layouts and ownership.
The cleanup follows the UX principles of reducing visual noise, giving a name
one meaning and keeping side effects inside the current surface.

## Display

- Tool has one connection-grouped catalog instead of a connection overview and
  a second flat catalog repeating the same source names and configuration links.
  A group holds connection state, built-in/external origin, its configuration
  action and the tool definitions. Failed and empty connections remain visible.
- MCP and Server Tool group capabilities by connection. Publication/execution
  configuration and relationship actions appear once in the group header, not
  on every tool. Tool names are the subject of the individual cards.
- The page header is the sole ordinary client-creation entry. The contextual
  client-selection panel retains its own creation action because that panel is
  a separate work surface. There is one workspace refresh action.
- Advisor's model form no longer nests a second ToolCard, badges, title or shared
  connection switch inside the source workspace. Its save patch contains only
  `id` and `advisor`, retaining unrelated shared state, usage and tool policies.

## Shared implementation

`MCPToolCard` renders definitions and a single control selected by explicit
`asset`, `client` or `gateway` mode. `MCPToolParameters` is the common schema view.
`MCPToolTestDialog` replaces the catalog dialog and source panel's inline test
implementation; administrative tests now open the same window from both places.
JSON validation, execution errors and complete structured results behave alike.
`toolPresentation` centralizes restriction/Advisor recognition and minimal policy
patch construction. These components stay within the lazy MCP route module.

The catalog is controlled by the parent's applied routing snapshot, also consumed
by the relationship graph and source workspace. The list no longer fetches a
separate catalog or performs duplicate discovery after mutations. Existing
request-generation guards discard old routing responses. A failed refresh clears
confirmed tools; loading disables changes until the snapshot is current.

Neutral onboarding, per-client authorization, independent purposes, source
repair, runtime gating and canonical old links retain the previous semantics.
Use a separate review Page and retain every process screenshot with hashes.
