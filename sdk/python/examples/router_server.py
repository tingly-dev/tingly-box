"""A routing provider: one model id in, many tb rules out.

This is the shape the other examples are variations on. From tingly-box's side
it is an ordinary self-hosted provider — you add it in Connect AI → Self-hosted
and bind a rule to the model `router`. From the inside it is a dispatcher: it
inspects each request and forwards it to a *different rule* depending on what
the request needs, so a single model id in your editor fans out across
everything you have configured.

The dispatch targets are rules in the **openai** scenario, because that is
where a normal install already has real models bound. A rule is tb's
`(scenario, request_model) -> services` binding, so "pick a rule" is just
"pick a scenario + model" — and the provider discovers which ones exist rather
than hard-coding names that may not be configured on this box:

    srv.tb.rules("openai")   # -> [Rule(request_model=..., services=[...]), ...]

Run it:

    pip install -e .                 # from sdk/python
    python examples/router_server.py

Then add it in tb as a self-hosted provider (the startup banner prints the
values) and bind a rule to model `router`. Every call to `router` now gets
classified and dispatched — with guard rails, quota, logging and tier-failover
applied on *both* hops, the inbound one and the one this file originates.
"""

from __future__ import annotations

from tingly import ChatRequest, Server

# Where the dispatch targets live. Rules under this scenario are what this
# provider fans out to; `openai` is the one a stock install already populates.
TARGET_SCENARIO = "openai"

# How a request is classified. Order matters — first match wins. Each entry is
# (label, predicate, preferred model substrings). The preferred names are hints
# matched against the rules that actually exist, not requirements.
LONG_REQUEST_CHARS = 4000
CODE_MARKERS = ("```", "def ", "class ", "import ", "func ", "SELECT ")


def classify(req: ChatRequest) -> str:
    """Bucket a request. Deliberately dumb — the point is the dispatch, not this.

    A real router would use token counts, tool declarations, or a cheap
    classifier call (which would itself be `srv.tb.ask(...)` against another
    rule — the recursion is free).
    """
    text = req.last_user_text()
    if len(text) >= LONG_REQUEST_CHARS:
        return "long"
    if any(marker in text for marker in CODE_MARKERS):
        return "code"
    return "short"


# Preferred model substrings per bucket, best first. Anything not configured on
# this box is skipped, so the same file works on a machine with only one model.
PREFERENCES = {
    "long": ("opus", "gpt-5", "sonnet"),
    "code": ("sonnet", "gpt-5", "coder"),
    "short": ("haiku", "mini", "flash"),
}

srv = Server(
    name="router",
    scenario=TARGET_SCENARIO,
    description="Classifies each request and dispatches it to a different tb rule",
)


@srv.chat
def handle(req: ChatRequest) -> str:
    bucket = classify(req)
    target = pick_rule(bucket)

    answer = srv.use(TARGET_SCENARIO).ask(req.last_user_text(), model=target)
    # Prefixed so the routing decision is visible while you are trying this out;
    # drop the prefix once you trust it.
    return f"[router: {bucket} -> {target}]\n{answer}"


def pick_rule(bucket: str) -> str:
    """Choose a request_model from the rules tb actually has.

    Falls back to "auto" — which lets tb's own smart routing decide — when
    nothing matches, so an unconfigured box degrades to "still works" rather
    than "no such model".
    """
    available = [r.request_model for r in srv.tb.rules(TARGET_SCENARIO) if r.active]
    for hint in PREFERENCES.get(bucket, ()):
        for model in available:
            if hint in model.lower():
                return model
    return available[0] if available else "auto"


if __name__ == "__main__":
    srv.run()
