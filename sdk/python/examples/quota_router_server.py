"""A quota-aware routing provider: several Codex accounts, one model id.

If you have more than one Codex subscription wired into tingly-box, the
question on every request is "which of them can still take work". This provider
answers it: it reads each account's quota, drops the ones that are nearly spent,
and dispatches to whichever has the most headroom left.

    client --model=codex--> tb --> THIS server
                                     |
                                     +-- GET /api/v1/provider-quota
                                     |     codex-personal   12% left   ok
                                     |     codex-work        2% left   skip
                                     |     codex-team       61% left   <- pick
                                     |
                                     +-- srv.use("openai").ask(model=<that account's rule>)

Two things make this work without any new tb mechanism:

- **Headroom is comparable across accounts.** `tb.quota` reduces each provider
  to its *tightest countable window* — the one that runs out first — which is
  the same reduction tb does internally. Accounts whose usage is unknown are
  skipped rather than assumed empty; see `tingly/helpers/quota.py`.
- **Each account is reachable as a rule.** A rule is tb's
  `(scenario, request_model) -> services` binding, so pointing one rule at each
  Codex provider is all the wiring needed, and this file picks between them by
  matching `service.provider` against the provider uuid the quota came from.

Set up (once, in the tb UI):

  1. Connect each Codex account as its own provider.
  2. Under the `openai` scenario, add one rule per account, each with a single
     service pointing at that provider. Name them whatever you like — this
     file discovers them.
  3. Add this server as a self-hosted provider and bind a rule to model
     `codex`.

Run it:

    pip install -e .                 # from sdk/python
    python examples/quota_router_server.py

Every call to `codex` now lands on the account with the most room left. When
they are all below the threshold the request still goes through — to the least
exhausted one — rather than failing; running low is not the same as being out,
and tb's own tier-failover is the right place to handle actually being out.
"""

from __future__ import annotations

from typing import Optional

from tingly import ChatRequest, Server

# Which upstream family to balance across. tb classifies providers by OAuth
# issuer (see ai/quota inferProviderType), so every Codex account shares this
# value — change it to "anthropic" to balance Claude Code accounts instead.
PROVIDER_TYPE = "codex"

# Accounts below this much remaining headroom are skipped while any healthier
# one exists. 5% is deliberately not 0: an account that close to its limit will
# usually run out mid-conversation.
MIN_HEADROOM_PERCENT = 5.0

TARGET_SCENARIO = "openai"

srv = Server(
    name="codex",
    scenario=TARGET_SCENARIO,
    description=f"Routes to whichever {PROVIDER_TYPE} account has quota left",
)


@srv.chat
def handle(req: ChatRequest) -> str:
    target, why = pick_account()
    if target is None:
        # Nothing to route to. Say so rather than failing opaquely — the fix is
        # a configuration one and the user needs to know which.
        return f"[quota-router] no usable {PROVIDER_TYPE} rule: {why}"

    answer = srv.use(TARGET_SCENARIO).ask(req.last_user_text(), model=target)
    return f"[quota-router: {why}]\n{answer}"


def pick_account() -> tuple[Optional[str], str]:
    """Return (request_model to dispatch to, a one-line reason).

    The reason is returned rather than logged because it is the thing you want
    to see while setting this up — which account was chosen and on what number.
    """
    ranked = srv.tb.quota.usable(PROVIDER_TYPE, min_headroom=MIN_HEADROOM_PERCENT)

    if not ranked:
        # Either every account is nearly spent, or none reports usable quota.
        # Fall back to the least-exhausted one so requests keep flowing.
        fallback = _least_exhausted()
        if fallback is None:
            return None, (
                f"no {PROVIDER_TYPE} provider reports quota — check that the "
                f"accounts are connected and that quota tracking is enabled"
            )
        usage, left = fallback
        model = _rule_for_provider(usage.provider_uuid)
        if model is None:
            return None, f"{usage.provider_name} has no rule under {TARGET_SCENARIO}"
        return model, f"all accounts low, using {usage.provider_name} ({left:.0f}% left)"

    for usage, left in ranked:
        model = _rule_for_provider(usage.provider_uuid)
        if model is not None:
            return model, f"{usage.provider_name} ({left:.0f}% left)"

    names = ", ".join(u.provider_name for u, _ in ranked)
    return None, f"{names} have quota but no rule under {TARGET_SCENARIO} points at them"


def _least_exhausted():
    """The account with the most headroom even if it is under the threshold."""
    from tingly.helpers.quota import headroom_percent

    scored = []
    for usage in srv.tb.quota.of_type(PROVIDER_TYPE):
        left = headroom_percent(usage)
        if left is not None:
            scored.append((usage, left))
    return max(scored, key=lambda pair: pair[1]) if scored else None


def _rule_for_provider(provider_uuid: str) -> Optional[str]:
    """The request_model of an active rule whose service uses this provider.

    This is the join between "which account has quota" (a provider uuid) and
    "what do I ask for" (a model id) — tb addresses upstreams by rule, so a
    provider is only reachable if some rule points at it.
    """
    for rule in srv.tb.rules(TARGET_SCENARIO):
        if not rule.active:
            continue
        for service in rule.services:
            if service.provider == provider_uuid and service.active:
                return rule.request_model
    return None


if __name__ == "__main__":
    srv.run(port=8768)
