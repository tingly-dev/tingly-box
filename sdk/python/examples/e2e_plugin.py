"""E2E plugin: served as an upstream, and calls BACK into tb's echo-model.

Demonstrates the full hub:
  client → tb (rule plugin/rag-demo) → THIS plugin → plugin.use("experiment")
         → tb (rule echo-model → vmodel) → echoed text → back to client
"""

from tingly import Plugin

plugin = Plugin(name="rag-demo", scenario="experiment")

CORPUS = {
    "tingly-box": "tingly-box is a personal intelligence orchestrator.",
    "plugin": "A plugin is an OpenAI-compatible upstream tb can route to.",
}


def retrieve(q: str) -> str:
    hits = [t for k, t in CORPUS.items() if k in q.lower()]
    return " ".join(hits) or "(no docs)"


@plugin.chat
def handle(req):
    q = req.last_user_text()
    docs = retrieve(q)
    # Call back into tb against the echo-model rule (no real network needed).
    echoed = plugin.use("experiment").ask(
        f"[plugin-rag] docs={docs!r} q={q!r}", model="echo-model"
    )
    return f"RAG via plugin → tb echo returned: {echoed}"


if __name__ == "__main__":
    # Short lease so the e2e can show auto-removal on death without a long wait.
    plugin.serve(port=8765, ttl_seconds=4)
