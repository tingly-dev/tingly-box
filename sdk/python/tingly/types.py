"""Shapes a `Server` handler sees for the OpenAI (`@srv.chat`) protocol.
Deliberately minimal: `raw` always carries the full decoded request body, so
a handler is never blocked on a field this module didn't think to model.

This is OpenAI-only. There is no shared shape across protocols — an
Anthropic (`@srv.messages`) handler gets the raw parsed body directly (see
`server.py`); this prototype doesn't build a normalization layer between
wire protocols, and a handler that wants Anthropic semantics (content
blocks, `system`, tool defs, ...) works with them directly rather than
through an abstraction that would hide them.
"""

from dataclasses import dataclass, field


@dataclass
class Message:
    role: str
    content: str


@dataclass
class ChatRequest:
    model: str
    messages: list[Message]
    raw: dict = field(default_factory=dict)

    @classmethod
    def from_body(cls, body: dict) -> "ChatRequest":
        messages = [
            Message(role=m.get("role", "user"), content=m.get("content", ""))
            for m in body.get("messages", [])
        ]
        return cls(model=body.get("model", ""), messages=messages, raw=body)
