"""A "router" plugin: quota-aware dispatch — a different shape from
rag/critic/fusion. Those all *generate* an answer themselves (one or more
calls back into tb feed a response the plugin composes). A router generates
nothing; its only job is to DECIDE which one candidate rule/model actually
serves the request, then forward to just that one.

This is the same idea as LiteLLM Router's `usage-based-routing` strategy —
route to whichever deployment has the most remaining rate-limit/quota
headroom right now, instead of a fixed priority order — implemented here as
a plugin instead of gateway config, using the SDK's quota views
(`Client.quota`, `sdk/python/tingly/helpers/quota.py`) added for exactly
this.

Run it (serves on :8768 AND registers with tb on startup):

    pip install -e .                 # from sdk/python
    python examples/router_plugin.py

Then from any tb client: model="plugin/router", the message is the question.
"""

from __future__ import annotations

from dataclasses import dataclass

from tingly import ChatRequest, Plugin


@dataclass
class Candidate:
    scenario: str
    model: str
    provider_uuid: str  # the provider backing (scenario, model) — quota is per-provider


# Fill in real provider UUIDs from the tb UI (Providers page) or
# `GET /api/v2/providers` — quota is tracked per provider, not per rule, so
# there is no way to infer these from the model name alone.
CANDIDATES = [
    Candidate(scenario="experiment", model="auto", provider_uuid="REPLACE_WITH_PROVIDER_UUID_1"),
    Candidate(scenario="experiment", model="auto", provider_uuid="REPLACE_WITH_PROVIDER_UUID_2"),
]

plugin = Plugin(
    name="router",
    scenario="experiment",  # bind a rule under this scenario on register
    description="Quota-aware dispatch — forwards to whichever candidate has the most headroom",
)


@plugin.chat
def handle(req: ChatRequest) -> str:
    question = req.last_user_text()
    chosen = _pick_candidate()
    # The only call that matters: forward to the ONE chosen candidate, not
    # every candidate — a router spends one hop total, not N (contrast with
    # fusion_plugin.py, which deliberately spends N to get a second opinion).
    return plugin.use(chosen.scenario).ask(question, model=chosen.model)


def _pick_candidate() -> Candidate:
    """Cached quota (tb refreshes lazily, ~20 min TTL) is enough for most
    routing decisions and costs nothing extra per request. Call
    `plugin.llm.quota.refresh(uuid)` first, for a specific candidate, if a
    request genuinely needs a number fresher than that — LiteLLM's own
    usage-based-routing docs warn that a live check on every single request
    adds real latency, so that should be the exception, not the default."""
    quotas = plugin.llm.quota.batch([c.provider_uuid for c in CANDIDATES])
    return max(
        CANDIDATES,
        key=lambda c: quotas[c.provider_uuid].headroom_percent if c.provider_uuid in quotas else 100.0,
    )


if __name__ == "__main__":
    plugin.serve(port=8768)
