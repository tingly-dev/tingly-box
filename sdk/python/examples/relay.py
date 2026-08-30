#!/usr/bin/env python3
"""The plainest possible SDK provider: relay every request to a different
tb rule/model unchanged. Register this server with tb as a dual provider
(Connect AI -> Self-hosted -> Dual endpoint, OpenAI URL
http://localhost:8765/v1, Anthropic URL http://localhost:8765, no key),
then route any scenario/rule at it.

Two independent handlers, one per protocol — no shared abstraction between
them (see tingly/server.py). Each gets the raw request body exactly as the
caller sent it and forwards it to tb over `.tb.chat()` (which always speaks
OpenAI), then hands the result back in its own protocol's shape.

Run:
    TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=... python relay.py
"""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import Server, text_of  # noqa: E402

TARGET_MODEL = os.environ.get("TINGLY_RELAY_TARGET_MODEL", "claude-opus-4-8")

srv = Server("relay-to-" + TARGET_MODEL)


@srv.chat
def handle_chat(body):
    return srv.tb.chat(model=TARGET_MODEL, messages=body["messages"])


@srv.messages
def handle_messages(body):
    # body is the raw Anthropic request as-is; plain-string content forwards
    # fine as an OpenAI message, but a caller sending content blocks or tool
    # defs would need its own handling here — this prototype doesn't do it
    # for you.
    return text_of(srv.tb.chat(model=TARGET_MODEL, messages=body["messages"]))


if __name__ == "__main__":
    srv.run(port=int(os.environ.get("PORT", 8765)))
