#!/usr/bin/env python3
"""The plainest possible provider: relay every request to a different tb
rule/model unchanged. Register this server with tb as a dual provider
(Connect AI -> Self-hosted -> Dual endpoint, OpenAI URL
http://localhost:8765/v1, Anthropic URL http://localhost:8765, a
placeholder key such as `not-required`), then route any scenario/rule at
the model name below. openai_responses shares the same OpenAI URL (it's a
different path, /responses, not a different base URL).

One function per protocol, nothing shared between them (see
tingly/server.py). Each gets its protocol's own fields exactly as the
caller sent them and forwards them to tb over `.tb.chat()` (which always
speaks OpenAI), then hands the result back in its own protocol's shape.

Run:
    TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=... python relay.py
"""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import Server, text_of  # noqa: E402

TARGET_MODEL = os.environ.get("TINGLY_RELAY_TARGET_MODEL", "claude-opus-4-8")
MODEL = "relay-to-" + TARGET_MODEL

srv = Server()  # .tb from TINGLY_BASE_URL / TINGLY_TOKEN


@srv.openai_chat(MODEL)
def relay_chat(messages):
    return srv.tb.chat(model=TARGET_MODEL, messages=messages)


@srv.anthropic_message(MODEL)
def relay_message(messages):
    # messages is the Anthropic list as sent; plain-string content forwards
    # fine as OpenAI messages, but content blocks or tool defs would need
    # their own handling here — this prototype doesn't do it for you.
    return text_of(srv.tb.chat(model=TARGET_MODEL, messages=messages))


@srv.openai_responses(MODEL)
def relay_responses(input):
    # input is a plain string for a simple text turn (Responses also allows
    # a list of structured input items — those would need their own
    # handling here, same caveat as relay_message above).
    return text_of(srv.tb.chat(model=TARGET_MODEL, messages=[{"role": "user", "content": input}]))


if __name__ == "__main__":
    srv.run(port=int(os.environ.get("PORT", 8765)))
