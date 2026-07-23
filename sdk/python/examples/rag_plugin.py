"""A RAG plugin served as an upstream for tingly-box (Anthropic primary, OpenAI secondary).

Run it (serves on :8765 AND registers with tb on startup):

    pip install -e .                 # from sdk/python
    python examples/rag_plugin.py

Registration is a one-shot, idempotent upsert by name — tb creates or updates
this plugin's provider (and the rule, since `scenario` is set below). There is
no heartbeat or lease; liveness is handled by tb's existing per-service circuit
breaker like any other provider.

Now `model="plugin/rag-demo"` from Claude Code, Cursor, the tb UI, or another
`tingly.connect()` experiment routes here — with tb's guard rails, quota,
logging and tier-failover applied. The handler itself calls *back* into tb via
`plugin.llm` for the generation step.
"""

from tingly import Plugin

plugin = Plugin(
    name="rag-demo",
    scenario="experiment",  # bind a rule under this scenario on register
    description="Answers from a toy in-memory corpus",
)

CORPUS = {
    "tingly-box": "tingly-box is a personal intelligence orchestrator: an LLM "
    "gateway with remote control and guard rails.",
    "plugin": "A tingly plugin is an Anthropic/OpenAI-compatible upstream that "
    "tingly-box can route to as a model.",
}


def retrieve(question: str) -> str:
    q = question.lower()
    hits = [text for key, text in CORPUS.items() if key in q]
    return "\n".join(hits) or "(no matching documents)"


@plugin.chat
def handle(req):
    question = req.last_user_text()
    docs = retrieve(question)
    # Generation goes back through tingly-box — no provider/key hard-coded here.
    return plugin.llm.ask(
        f"Using only these documents:\n{docs}\n\nAnswer: {question}",
        model="auto",
    )


if __name__ == "__main__":
    plugin.serve()
