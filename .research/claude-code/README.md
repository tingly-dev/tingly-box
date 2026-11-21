# Claude Code release analyses

One file per analysed Claude Code release (read-only reverse engineering of the npm package: request construction, OAuth, gateway behaviour), each with a
"changes since <previous version>" section and a gap list against tingly-box. Older files are never overwritten; a new release gets a new file.

| Version | File | Scope |
|---|---|---|
| 2.1.328 | [2.1.328.md](./2.1.328.md) | Requests (headers, betas, billing header, body), OAuth, embedded gateway protocol, gap list vs tingly-box; §9 = live `-p` wire captures (proxy/direct × OAuth/API key), `cch` verified against our code |

Earlier analyses live in the design docs rather than here:

| Version(s) | Where |
|---|---|
| 2.1.86, 2.1.258, 2.1.280 | `.design/claude-code-oauth-compat.md` §B3 (2.1.258 vs 2.1.86), §B8 (2.1.258 → 2.1.280); method §B2, upgrade checklist §B6 |
| 2.1.259 | `.research/slash-commands-claude-code.md` (slash commands over stream-json) |

Method for a new release: `.design/claude-code-oauth-compat.md` §B2 (get the package, slice the bundle), then diff string sets against the previous release (see 2.1.328.md §1).
