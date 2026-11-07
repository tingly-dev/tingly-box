# Claude Code — design index

Claude Code has two independent design surfaces. Choose the document for the
code you are changing; subprocess lifecycle and upstream OAuth identity do not
share an implementation contract.

| Document | Scope | Status |
|---|---|---|
| [claude-code-session.md](./claude-code-session.md) | `@cc` persistent subprocess sessions, isolation, pool and rollout | P0–P2 implemented; opt-in, default off; P3 observability remains |
| [claude-code-oauth-compat.md](./claude-code-oauth-compat.md) | Claude OAuth wire identity, `claude_code_version`, headers, betas and billing hash | Implemented profiles; limits and upgrade checklist in the document |
| [claude-code-config.md](./claude-code-config.md) | Quick Config, model slots and generated client settings | Current design |

## Auto mode and `CLAUDE_CODE_AUTO_MODE_SERVER`

> TEP-2463. The option below is the default until the gateway can pass the
> server review through; the pass-through follows in a stacked change.

In auto mode Claude Code can have the Anthropic server review actions inside
the session's own model requests at no charge. That needs the gateway to pass
the `anthropic-beta` header and the `safeguards` body field through, and
`safeguard_results` back
([doc](https://code.claude.com/docs/en/auto-mode-classifier-billing)). tb
decodes requests into typed SDK params and converts across providers, so it
cannot yet, and sessions pointed at it would show the "session isn't eligible"
notice (holding the first checked action in the interactive CLI).

tb therefore sets `CLAUDE_CODE_AUTO_MODE_SERVER=0` by default: Claude Code
sends its own classifier calls, which tb routes like any other request and
bills to the user's upstream provider (the model is whatever the `tingly/cc`
rule maps Sonnet to, not Sonnet 5).

It is a user-visible option, not an Advanced one: the Quick Config page has an
"Auto mode review" row (and profile overrides list it) that writes an explicit
`1` or `0`. Off is `0`, never unset, because unset lets Claude Code ask the
server. The user's choice always wins over the default. Where the default is set:

- `ai/agent` `ClaudeCodeParams.BuildEnv` (`ExtraEnv` overrides)
- `internal/agent` `ClaudeCodePrefs` (typed field, `DefaultClaudeCodePrefs`),
  `ToEnv` and `ResolveCCProfileSettings` (the typed field, `Extra`, the main
  settings file or a profile's `Env` override)
- `agentboot/claude` `buildCleanEnv`, covering `@cc` and Projects threads
  (an exported variable, `CustomEnv`, `opts.Env` or a settings file override)

Claude Code calls the variable temporary and may remove it; revisit when it
does.

The original session sections §1–§7 now live in `claude-code-session.md`;
the original compatibility sections §B0–§B8 live in `claude-code-oauth-compat.md`.
