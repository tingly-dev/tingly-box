# Claude Code — design index

Claude Code has two independent design surfaces. Choose the document for the
code you are changing; subprocess lifecycle and upstream OAuth identity do not
share an implementation contract.

| Document | Scope | Status |
|---|---|---|
| [claude-code-session.md](./claude-code-session.md) | `@cc` persistent subprocess sessions, isolation, pool and rollout | P0–P2 implemented; opt-in, default off; P3 observability remains |
| [claude-code-oauth-compat.md](./claude-code-oauth-compat.md) | Claude OAuth wire identity, `claude_code_version`, headers, betas and billing hash | Implemented profiles; limits and upgrade checklist in the document |
| [claude-code-config.md](./claude-code-config.md) | Quick Config, model slots and generated client settings | Current design |

The original session sections §1–§7 now live in `claude-code-session.md`;
the original compatibility sections §B0–§B8 live in `claude-code-oauth-compat.md`.
