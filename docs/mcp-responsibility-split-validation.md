# MCP responsibility split — independent validation record

Stack base: `fix/mcp-overview-title` (`41885459`).
Branch: `feat/mcp-responsibility-split`.
Previous design and validation documents remain unchanged.

| Page | Owns |
| --- | --- |
| Tool | Built-in/external source onboarding, complete catalog, schemas, connection/authentication, standalone tests and shared switches |
| MCP | Publication, client grants, commands, saved-client SDK verification, usage relationships and runtime configuration |
| Server Tool | Model execution eligibility and Advisor consultation provider/model |

New Tool sources explicitly set both usage purposes false. Existing source usage,
per-tool policies and client grants are retained. Connection patches omit purpose
and policy fields. Purpose switches preserve the other purpose. Existing source
bookmarks resolve to Tool; client/profile/install links resolve to MCP; execution
links remain in Server Tool. Closing panels stays in the owning secondary layout.

## Verified boundaries

The browser flow uses actual HTTP and stdio MCP fixtures and a saved-client SDK,
not mocked tool counts. Every mutation is restored after each capture stage.

- The wildcard client still discovers 4 tools after a neutral source is registered.
- Tool can discover and test that source before publication or client grants.
- MCP publication alone does not grant an explicitly scoped client; saving a
  reviewed client grant changes actual discovery from 4 to 5 and permits a call.
- Adding a Server Tool changes only execution purpose; MCP access stays unchanged.
- Tool shared disablement blocks both paths and retains policies; re-enablement
  restores discovery. Connection repair preserves tool policies and grants.
- Revoking MCP publication returns discovery to 4 and rejects the revoked call,
  while retaining the tool definition and gateway execution assignment.
- Advisor model settings appear in Server Tool. It has no standalone asset test.
- The MCP relationship graph links to each owning page and explains blocked tools.
- Old source/client links resolve to their canonical pages and close correctly.
- All three mobile pages fit without horizontal document overflow.

## Checks

- MCP frontend suite: **54 passed** in 7 files.
- Full frontend suite: **430 passed**, 3 pre-existing `tKeyCoverage` failures;
  the same 108 missing keys in each locale, with no added missing keys.
- Typecheck, production build and lint pass; existing lint and bundle warnings remain.
- Complete protocol harness: **5,640 cases** — 5,524 pass, 114 skip, 2 existing gaps;
  no status differences from the previous version. The gaps are nonstream recording
  for OpenAI Responses → Anthropic Beta and OpenAI Responses → OpenAI Chat.

All process screenshots, including early automation selector/wait failures,
are retained alongside their manifests, timestamps, Git heads and SHA-256 hashes.
The successful production capture is run against the committed implementation.
A separate review Page contains previews and downloadable screenshot/validation
artifacts; private preview credentials and configuration are excluded.
