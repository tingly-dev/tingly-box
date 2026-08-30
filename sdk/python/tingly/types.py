"""Shapes a `Server` handler sees. Deliberately minimal: `raw` always carries
the full decoded request body, so a handler is never blocked on a field this
module didn't think to model.
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
