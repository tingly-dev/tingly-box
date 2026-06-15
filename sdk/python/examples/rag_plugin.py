"""A RAG plugin served as an OpenAI-compatible upstream for tingly-box.

Run it:

    pip install -e .                 # from sdk/python
    python examples/rag_plugin.py    # serves on http://127.0.0.1:8765/v1

Then wire it into tb in one step (creates the provider + a rule) so any client
can select model `plugin/rag-demo`:

    tingly plugin register rag-demo --url http://127.0.0.1:8765/v1 \
        --model-id plugin/rag-demo --scenario experiment

Now `model="plugin/rag-demo"` from Claude Code, Cursor, the tb UI, or another
`tingly.connect()` experiment routes here — with tb's guard rails, quota,
logging and tier-failover applied. The handler itself calls *back* into tb via
`plugin.llm` for the generation step.
"""

from tingly import Plugin

plugin = Plugin(name="rag-demo", description="Answers from a toy in-memory corpus")

CORPUS = {
    "tingly-box": "tingly-box is a personal intelligence orchestrator: an LLM "
    "gateway with remote control and guard rails.",
    "plugin": "A tingly plugin is an OpenAI-compatible upstream that tingly-box "
    "can route to as a model.",
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
