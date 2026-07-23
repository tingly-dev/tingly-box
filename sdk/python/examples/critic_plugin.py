"""A "critic" plugin: cross-model critique — the pattern behind Zen MCP and
Consult7 (an agent mid-task consults a *different* model for review) and
aider's architect/editor split (a separate model reviews before code lands).

Self-critique — a model reviewing its own output — is unreliable: Huang et
al. (ICLR 2024) found LLMs cannot reliably self-correct without external
feedback. Cross-model critique (a genuinely different model reviews) is the
more robust variant, and it maps directly onto what a plugin is for: this
handler does zero LLM work itself, it only forwards the artifact-to-review to
a different tb rule/model via `plugin.use(...)` and shapes the structured
verdict that comes back. No hard-coded provider or key — same gateway,
different rule.

Run it (serves on :8766 AND registers with tb on startup):

    pip install -e .                 # from sdk/python
    python examples/critic_plugin.py

Then from any tb client: model="plugin/critic", the message is the thing to
review (a diff, a draft answer, a decision); an optional system message adds
context the critic should weigh.
"""

from __future__ import annotations

import json

from tingly import ChatRequest, Plugin

# Where the critique itself is delegated — point CRITIC_SCENARIO / CRITIC_MODEL
# at a rule bound to a genuinely different (ideally stronger) model than
# whatever called this plugin; reviewing with the same model defeats the point.
CRITIC_SCENARIO = "experiment"
CRITIC_MODEL = "auto"

CRITIQUE_PROMPT = """You are reviewing the following for correctness, risk and \
missing considerations. Respond with ONLY JSON matching:
{{"verdict": "approve" | "revise", "issues": ["..."], "suggestion": "..."}}

--- context ---
{context}

--- to review ---
{content}
"""

plugin = Plugin(
    name="critic",
    scenario="experiment",  # bind a rule under this scenario on register
    description="Cross-model critique — delegates review to a different rule/model",
)


@plugin.chat
def handle(req: ChatRequest) -> str:
    content = req.last_user_text()
    context = req.system_text() or "(none)"
    prompt = CRITIQUE_PROMPT.format(context=context, content=content)

    # The one line that matters: hand the review to a DIFFERENT tb rule.
    raw = plugin.use(CRITIC_SCENARIO).ask(prompt, model=CRITIC_MODEL, max_tokens=1024)
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
    plugin.serve(port=8766)
