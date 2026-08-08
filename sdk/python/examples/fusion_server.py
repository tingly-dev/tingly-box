"""A "fusion" server: parallel multi-model consensus, then a judge model
synthesizes — the pattern behind Consult7's 2026 Fusion feature (a panel of
frontier models answers in parallel; a judge model merges the answers; a
panel that already agrees skips the judge call).

This is the clearest illustration of why a Python provider is worth more than
the sum of its parts: it is a perfectly ordinary upstream from tb's side, yet
its handler calls BACK into tb more than once, against DIFFERENT rules/models,
concurrently, before answering once. One provider, orchestrating the box.

Run it (serves on :8767):

    pip install -e .                 # from sdk/python
    python examples/fusion_server.py

Add it in tb as a self-hosted provider (the startup banner prints the values),
bind a rule to model `fusion`, then from any tb client the message is the
question.
"""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor

from tingly import ChatRequest, Server

PANEL_SCENARIO = "openai"
JUDGE_SCENARIO = "openai"

# How many rules to poll. The panel members are *discovered*, not listed here:
# a panel is only worth its latency if its members are genuinely different
# models, and only this box knows which rules it actually has. Hard-coding
# entries is how you end up polling the same model twice and calling it a
# second opinion.
PANEL_SIZE = 3

JUDGE_PROMPT = """Multiple models answered the same question independently. \
Synthesize the single best answer, resolving disagreements and noting when \
the panel disagreed.

--- question ---
{question}

--- panel answers ---
{answers}
"""

srv = Server(
    name="fusion",
    scenario=JUDGE_SCENARIO,  # which rule-set the panel/judge calls run against
    description="Multi-model consensus — panel of rules/models + judge synthesis",
)


@srv.chat
def handle(req: ChatRequest) -> str:
    question = req.last_user_text()
    panel = _panel()

    if len(panel) < 2:
        # One model is not a panel. Answer directly rather than paying for a
        # judge call that has nothing to reconcile.
        only = panel[0] if panel else "auto"
        return srv.use(PANEL_SCENARIO).ask(question, model=only)

    answers = _poll_panel(question, panel)

    if len(set(answers)) == 1:
        # The panel already agreed — the judge call would just restate this,
        # so skip it and save a hop (mirrors Consult7 skipping the panel
        # entirely for trivial prompts).
        return answers[0]

    answers_block = "\n\n".join(f"[{i + 1}] {a}" for i, a in enumerate(answers))
    return srv.use(JUDGE_SCENARIO).ask(
        JUDGE_PROMPT.format(question=question, answers=answers_block),
        model=_judge(panel),
    )


def _panel() -> list:
    """Up to PANEL_SIZE distinct active rules under PANEL_SCENARIO."""
    seen, models = set(), []
    for rule in srv.tb.rules(PANEL_SCENARIO):
        if not rule.active or rule.request_model in seen:
            continue
        seen.add(rule.request_model)
        models.append(rule.request_model)
        if len(models) == PANEL_SIZE:
            break
    return models


def _judge(panel: list) -> str:
    """Prefer a rule the panel did not use, so the judge is a fresh opinion.

    Falls back to the first panel member when the box has nothing else — a
    judge that also answered is still better than no synthesis.
    """
    for rule in srv.tb.rules(JUDGE_SCENARIO):
        if rule.active and rule.request_model not in panel:
            return rule.request_model
    return panel[0]


def _poll_panel(question: str, panel: list) -> list:
    with ThreadPoolExecutor(max_workers=len(panel)) as pool:
        futures = [
            pool.submit(srv.use(PANEL_SCENARIO).ask, question, model=model)
            for model in panel
        ]
        return [f.result() for f in futures]


if __name__ == "__main__":
    srv.run(port=8767)
