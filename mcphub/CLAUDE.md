# Mcphub

The shared MCP hub: the GitHub client and toolset over the host's `gh` (the
built-in default GitHub MCP), the generic MCP tool definition core, and the
loopback streamable-HTTP serving helper. It is its own Go module
(`github.com/tingly-dev/tingly-box/mcphub`), imported by Tingly Box (the root
module) and by Projects.

# Boundary

- Mcphub imports no local module — only the MCP Go SDK and stdlib. It never
  imports the root module, Projects, Remote, imbot or agentboot
  (`task mcphub:boundary`, also run in CI).
- Host concerns (thread identity, permissions, scopes, persistence) stay with
  the embedder: mcphub exposes the client and generic registration, and the
  hosts adapt. A tool never learns who called it beyond what the host passes in.
- The GitHub transport is the host's own `gh` CLI, signed in as the user: no
  tokens are managed here. Anything that looks like a credential is redacted
  before it leaves (`github.Redact`).

Run `task mcphub:test` from the repo root.
