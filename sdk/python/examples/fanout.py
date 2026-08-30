#!/usr/bin/env python3
"""Fan a single request out to several tb models and merge the replies.
Demonstrates the case a pure relay can't: custom logic between the two
tb calls, not just a single pass-through.

Run:
    TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=... python fanout.py
"""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import Server, text_of  # noqa: E402

MODELS = os.environ.get("TINGLY_FANOUT_MODELS", "claude-opus-4-8,gpt-5").split(",")

srv = Server("fanout")


@srv.chat
def handle(req):
    replies = [
        text_of(srv.tb.chat(model=model, messages=req.as_openai_messages()))
        for model in MODELS
    ]
    merged = "\n\n".join(f"[{model}]\n{reply}" for model, reply in zip(MODELS, replies))
    return merged


if __name__ == "__main__":
    srv.run(port=int(os.environ.get("PORT", 8766)))
