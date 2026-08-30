#!/usr/bin/env python3
"""The plainest possible SDK provider: relay every request to a different
tb rule/model unchanged. Register this server with tb as a dual provider
(Connect AI -> Self-hosted -> Dual endpoint, OpenAI URL
http://localhost:8765/v1, Anthropic URL http://localhost:8765, no key),
then route any scenario/rule at it — whichever protocol the caller used,
everything it receives it forwards to the model named below and hands the
answer straight back.

Run:
    TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=... python relay.py
"""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import Server  # noqa: E402

TARGET_MODEL = os.environ.get("TINGLY_RELAY_TARGET_MODEL", "claude-opus-4-8")

srv = Server("relay-to-" + TARGET_MODEL)


@srv.chat
def handle(req):
    return srv.tb.chat(model=TARGET_MODEL, messages=req.as_openai_messages())


if __name__ == "__main__":
    srv.run(port=int(os.environ.get("PORT", 8765)))
