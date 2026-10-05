# MCP usage relationships validation

This revision builds on `d0bf86fc`. Existing MCP protocol behavior is unchanged;
the relation view consumes the existing effective routing API and saved profiles.

Run frontend MCP tests and the route grant utilities, TypeScript, lint and a
production build. Compare full frontend results with the existing translation
coverage baseline. Run the protocol matrix harness and retain its raw results.

Browser verification should use an isolated SDK HTTP/stdio source and model
fixture. Retain all capture attempts without overwriting earlier stages:

- Overview destination and Tool/Server Tool cards open a focused source bookmark.
- Both paths and each tool's client/gateway state agree with actual SDK discovery.
- Explicit tool-subset grants explain the missing tool while preserving gateway
  use. Client, source and MCP disablement do not show positive counts.
- Revoking either tool purpose preserves the other; saved client association is
  distinct from currently available access.
- Failed discovery stays visible, with no fabricated tool catalog. Repair and
  refresh replace the graph state from the shared snapshot.
- Advisor shows its consultation provider/model and opens Server Tool settings.
- Scoped node actions return to their own configuration page. Deleted connection
  bookmarks are explicit, existing source/routes/install links remain valid.
- Mobile keeps page width bounded; horizontal path scrolling stays inside the
  diagram. Relationship actions are usable with keyboard and accessible names.

Publish an independent revision review document and a checksummed artifact with
all screenshots, scripts, raw logs and an HTML index. Prior review documents and
packages remain frozen and linked for comparison.
