"""A "router" plugin: quota-aware dispatch — a different shape from
rag/critic/fusion. Those all *generate* an answer themselves (one or more
calls back into tb feed a response the plugin composes). A router generates
nothing; its only job is to DECIDE which one candidate model actually serves
the request, then forward to just that one — and to GUARANTEE the provider
it checked quota for is the provider that actually serves it.

That guarantee is why this isn't just "pick a model and call .ask(model=)":
a model name resolves to a *rule*, and a rule can have more than one active
service (tiers, load-balanced) — tb, not this plugin, decides which one of
those actually runs. Checking quota for one provider and then calling
`.ask(model=X)` would silently mean nothing if tb's own load balancer picks
a different service within that rule. So each candidate here must resolve
(via `Client.rules`) to a rule with exactly ONE active service — a model
name dedicated to one specific provider — and the forwarded call passes
`pin_provider=` (`X-Tingly-Pin-Provider`, see .design/python-sdk.md) to force
that exact provider, closing the loop between "what was checked" and "what
was used".

Same idea as LiteLLM Router's `usage-based-routing` strategy — route to
whichever deployment has the most remaining rate-limit/quota headroom right
now — implemented as a plugin instead of gateway config.

Run it (serves on :8768 AND registers with tb on startup):

    pip install -e .                 # from sdk/python
    python examples/router_plugin.py

Then from any tb client: model="plugin/router", the message is the question.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import List

from tingly import ChatRequest, Plugin

# Each entry must be a model name, in ROUTER_SCENARIO, whose rule you've
# configured to point at exactly ONE provider — e.g. an "anthropic" scenario
# with a "sonnet1" rule bound only to provider A and a "sonnet2" rule bound
# only to provider B. That 1:1 binding is what makes a quota-based pick mean
# something (see the module docstring); a rule with more than one active
# service is skipped as a candidate, not guessed at.
ROUTER_SCENARIO = "experiment"
CANDIDATE_MODELS = ["sonnet1", "sonnet2"]

plugin = Plugin(
    name="router",
    scenario=ROUTER_SCENARIO,  # bind a rule under this scenario on register
    description="Quota-aware dispatch — forwards to whichever candidate has the most headroom",
)


@dataclass
class ResolvedCandidate:
    model: str
    provider_uuid: str  # the ONE provider this model's rule is pinned to


@plugin.chat
def handle(req: ChatRequest) -> str:
    question = req.last_user_text()
    chosen = _pick_candidate()
    # pin_provider is what makes this a real decision rather than a guess:
    # the provider that was quota-checked is GUARANTEED to be the one that
    # serves this request.
    return plugin.use(ROUTER_SCENARIO).ask(
        question, model=chosen.model, pin_provider=chosen.provider_uuid
    )


def _resolve_candidates() -> List[ResolvedCandidate]:
    """Resolve each candidate model to its rule's single pinned provider.
    Skips (rather than guesses at) any candidate whose rule has zero or more
    than one active service — quota can't mean anything for a model tb
    itself load-balances across multiple providers."""
    rules = plugin.llm.rules
    resolved = []
    for model in CANDIDATE_MODELS:
        rule = rules.for_model(ROUTER_SCENARIO, model)
        if rule is None:
            continue
        services = rule.active_services
        if len(services) != 1:
            continue  # not a pinned single-provider rule — not routable by quota
        resolved.append(ResolvedCandidate(model=model, provider_uuid=services[0].provider))
    return resolved


def _pick_candidate() -> ResolvedCandidate:
    """Cached quota (tb refreshes lazily, ~20 min TTL) is enough for most
    routing decisions and costs nothing extra per request. Call
    `plugin.llm.quota.refresh(uuid)` first, for a specific candidate, if a
    request genuinely needs a number fresher than that — LiteLLM's own
    usage-based-routing docs warn that a live check on every single request
    adds real latency, so that should be the exception, not the default."""
    candidates = _resolve_candidates()
    if not candidates:
        raise RuntimeError(
            "no router candidate resolved to a single-provider rule — each "
            "entry in CANDIDATE_MODELS must name a model whose rule has "
            "exactly one active service (see the module docstring)"
        )
    quotas = plugin.llm.quota.batch([c.provider_uuid for c in candidates])
    return max(
        candidates,
        key=lambda c: quotas[c.provider_uuid].headroom_percent if c.provider_uuid in quotas else 100.0,
    )


if __name__ == "__main__":
    plugin.serve(port=8768)
