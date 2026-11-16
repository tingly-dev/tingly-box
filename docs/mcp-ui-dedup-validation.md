# MCP display cleanup: independent validation

Base: `feat/mcp-responsibility-split` (`1af3a70a`).
Branch: `feat/mcp-ui-dedup`. Earlier version documents are unchanged.

- MCP suite: 59 passing tests in 9 files. Meaningful coverage includes grouped
  actions, neutral assets, independent policy patches, stale-response rejection,
  structured test errors, runtime gating and Advisor-only save fields.
- Full frontend: 435 passing, 3 existing translation-key coverage failures;
  exactly the same 108 missing keys per locale, no additions.
- Typecheck, production build and lint pass with the existing repository warnings.
- Complete protocol harness: 5,640 cases — 5,524 pass, 114 skip, 2 existing recording
  gaps; no status changes against the preceding version.
- Actual saved-client SDK flow verifies neutral registration (wildcard client
  remains at 4 tools), Tool tests before publication, explicit grants (4→5),
  revoke/denied calls (5→4), separate gateway eligibility, shared disablement and
  repair, Advisor configuration, old links and all three mobile layouts.
- Browser request records confirm no separate `/mcp/catalog` request: catalog,
  source workspace and routing graph consume the same applied snapshot.
- An actual Advisor model save retains a disabled shared connection, its usage
  and tool policies. There is no shared enable switch in that model form.

Review artifacts retain before, draft, validation and final production captures,
including any failed capture. Manifests record timestamps, Git heads, checks and
SHA-256 hashes. The final production flow is captured from the pushed commit.
No private preview credentials or configuration are included.
