"""A "critic" server: cross-model critique — the pattern behind Zen MCP and
Consult7 (an agent mid-task consults a *different* model for review) and
aider's architect/editor split (a separate model reviews before code lands).

Self-critique — a model reviewing its own output — is unreliable: Huang et
al. (ICLR 2024) found LLMs cannot reliably self-correct without external
feedback. Cross-model critique (a genuinely different model reviews) is the
more robust variant, and it shows what a Python provider is really for: this
handler does zero LLM work itself, it only forwards the artifact-to-review to
a different tb rule/model via `srv.use(...)` and shapes the structured verdict
that comes back. No hard-coded provider or key — same gateway, different rule.

Dispatch shape: **one target, deliberately not the caller's**. Where
router_server picks a rule by classifying the request, this one always routes
away from whatever asked — reviewing with the same model is the thing the
research says does not work.

Targets rules in the `openai` scenario, which is where a stock install already
has models bound. Set CRITIC_MODEL to a request_model you actually have
(`srv.tb.rules("openai")` lists them) — "auto" lets tb's smart routing pick.

Run it (serves on :8766):

    pip install -e .                 # from sdk/python
    python examples/critic_server.py

Add it in tb as a self-hosted provider (the startup banner prints the values),
bind a rule to model `critic`, then from any tb client the message is the
thing to review (a diff, a draft answer, a decision); an optional system
message adds context the critic should weigh.
"""

from __future__ import annotations

import json

from tingly import ChatRequest, Server

# Where the critique itself is delegated — point CRITIC_SCENARIO / CRITIC_MODEL
# at a rule bound to a genuinely different (ideally stronger) model than
# whatever called this server; reviewing with the same model defeats the point.
CRITIC_SCENARIO = "openai"
CRITIC_MODEL = "auto"

CRITIQUE_PROMPT = """You are reviewing the following for correctness, risk and \
missing considerations. Respond with ONLY JSON matching:
{{"verdict": "approve" | "revise", "issues": ["..."], "suggestion": "..."}}

--- context ---
{context}

--- to review ---
{content}
"""

srv = Server(
    name="critic",
    scenario=CRITIC_SCENARIO,  # which rule-set the delegation below runs against
    description="Cross-model critique — delegates review to a different rule/model",
)


@srv.chat
def handle(req: ChatRequest) -> str:
    content = req.last_user_text()
    context = req.system_text() or "(none)"
    prompt = CRITIQUE_PROMPT.format(context=context, content=content)

    # The one line that matters: hand the review to a DIFFERENT tb rule.
    raw = srv.use(CRITIC_SCENARIO).ask(prompt, model=CRITIC_MODEL, max_tokens=1024)
    return _format_verdict(_parse_verdict(raw))


def _parse_verdict(raw: str) -> dict:
    text = raw.strip()
    if text.startswith("```"):
        text = text.strip("`")
        text = text.split("\n", 1)[1] if "\n" in text else text
    try:
        return json.loads(text)
    except ValueError:
        # The critic model didn't follow the JSON contract — degrade to a
        # plain "revise" verdict carrying its raw text as the suggestion,
        # rather than crashing the request.
        return {"verdict": "revise", "issues": ["critic model did not return JSON"], "suggestion": raw}


def _format_verdict(verdict: dict) -> str:
    lines = [f"verdict: {verdict.get('verdict', 'unknown')}"]
    for issue in verdict.get("issues") or []:
        lines.append(f"- {issue}")
    if verdict.get("suggestion"):
        lines.append(f"suggestion: {verdict['suggestion']}")
    return "\n".join(lines)


if __name__ == "__main__":
    srv.run(port=8766)
