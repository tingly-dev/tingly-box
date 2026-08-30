"""Shapes a `Server` handler sees. Deliberately minimal: `raw` always carries
the full decoded request body, so a handler is never blocked on a field this
module didn't think to model.

One shape covers both wire protocols `Server` accepts (OpenAI chat and
Anthropic messages) — a handler is written once and never branches on which
endpoint the request arrived on. Anthropic's `system` field, when present, is
folded in as a leading `system` message so both shapes look the same by the
time a handler sees them; `raw` still carries the untouched original body for
anything a handler needs beyond text (tool defs, `max_tokens`, ...).
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
        messages = []
        system = body.get("system")
        if system:
            messages.append(Message(role="system", content=_flatten_content(system)))
        for m in body.get("messages", []):
            messages.append(Message(role=m.get("role", "user"), content=_flatten_content(m.get("content", ""))))
        return cls(model=body.get("model", ""), messages=messages, raw=body)

    def as_openai_messages(self) -> list[dict]:
        """The normalized message list as plain OpenAI-shaped dicts — what
        `Client.chat()` expects. Use this to forward a request rather than
        `raw["messages"]`: `raw` still carries whatever shape the caller's
        wire protocol used (a string or Anthropic content blocks), while
        `.tb` always speaks OpenAI regardless of which endpoint the request
        arrived on."""
        return [{"role": m.role, "content": m.content} for m in self.messages]


def _flatten_content(content) -> str:
    """Anthropic content is a string or a list of blocks (`{"type": "text",
    "text": ...}` among others); OpenAI content is normally a plain string.
    Reduce either shape to plain text — v1 handlers work with text, not
    content blocks."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts = []
        for block in content:
            if isinstance(block, dict) and "text" in block:
                parts.append(block["text"])
            elif isinstance(block, str):
                parts.append(block)
        return "".join(parts)
    return str(content) if content else ""
