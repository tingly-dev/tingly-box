"""A "fusion" plugin: parallel multi-model consensus, then a judge model
synthesizes — the pattern behind Consult7's 2026 Fusion feature (a panel of
frontier models answers in parallel; a judge model merges the answers; a
panel that already agrees skips the judge call).

This is the clearest illustration of "tb is a hub of rules; a plugin can
freely originate calls against any of them": the handler calls BACK into tb
more than once, against DIFFERENT rules/models, concurrently, before
answering once.

Run it (serves on :8767 AND registers with tb on startup):

    pip install -e .                 # from sdk/python
    python examples/fusion_plugin.py

Then from any tb client: model="plugin/fusion", the message is the question.
"""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor

from tingly import ChatRequest, Plugin

# The panel: each entry is (scenario, model) called independently and
# concurrently. Point these at genuinely different rules/models — a panel of
# clones of the same model adds latency without adding a second opinion.
PANEL = [
    ("experiment", "auto"),
    ("experiment", "auto"),
]
JUDGE_SCENARIO = "experiment"
JUDGE_MODEL = "auto"

JUDGE_PROMPT = """Multiple models answered the same question independently. \
Synthesize the single best answer, resolving disagreements and noting when \
the panel disagreed.

--- question ---
{question}

--- panel answers ---
{answers}
"""

plugin = Plugin(
    name="fusion",
    scenario="experiment",  # bind a rule under this scenario on register
    description="Multi-model consensus — panel of rules/models + judge synthesis",
)


@plugin.chat
def handle(req: ChatRequest) -> str:
    question = req.last_user_text()
    answers = _poll_panel(question)

    if len(set(answers)) == 1:
        # The panel already agreed — the judge call would just restate this,
        # so skip it and save a hop (mirrors Consult7 skipping the panel
        # entirely for trivial prompts).
        return answers[0]

    answers_block = "\n\n".join(f"[{i + 1}] {a}" for i, a in enumerate(answers))
    return plugin.use(JUDGE_SCENARIO).ask(
        JUDGE_PROMPT.format(question=question, answers=answers_block),
        model=JUDGE_MODEL,
    )


def _poll_panel(question: str) -> list:
    with ThreadPoolExecutor(max_workers=len(PANEL)) as pool:
        futures = [
            pool.submit(plugin.use(scenario).ask, question, model=model)
            for scenario, model in PANEL
        ]
        return [f.result() for f in futures]


if __name__ == "__main__":
    plugin.serve(port=8767)
