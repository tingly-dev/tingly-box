"""Request/response value types passed to a plugin's chat handler.

The handler sees a normalized :class:`ChatRequest` regardless of wire details,
and returns either a ``str`` (buffered) or an iterator of ``str`` (streamed) —
the server shapes those into OpenAI ``chat.completion`` / ``chat.completion.chunk``
payloads so tingly-box (or any OpenAI client) can consume the plugin as an
upstream model.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional


@dataclass
class Message:
    role: str
    content: str


@dataclass
class ChatRequest:
    """A normalized chat request handed to a plugin handler."""

    model: str
    messages: List[Message]
    stream: bool = False
    # Everything else from the wire body, untouched (temperature, tools, …).
    extra: Dict[str, Any] = field(default_factory=dict)

    def last_user_text(self) -> str:
        """The text of the most recent user message (empty string if none)."""
        for msg in reversed(self.messages):
            if msg.role == "user":
                return msg.content
        return ""

    def system_text(self) -> Optional[str]:
        for msg in self.messages:
            if msg.role == "system":
                return msg.content
        return None

    @classmethod
    def from_openai_body(cls, body: Dict[str, Any]) -> "ChatRequest":
        raw_messages = body.get("messages") or []
        messages = [
            Message(role=m.get("role", "user"), content=_content_to_text(m.get("content")))
            for m in raw_messages
        ]
        known = {"model", "messages", "stream"}
        extra = {k: v for k, v in body.items() if k not in known}
        return cls(
            model=body.get("model", ""),
            messages=messages,
            stream=bool(body.get("stream", False)),
            extra=extra,
        )

    @classmethod
    def from_anthropic_body(cls, body: Dict[str, Any]) -> "ChatRequest":
        """Normalize an Anthropic Messages API body (POST /v1/messages).

        Anthropic keeps the system prompt in a top-level ``system`` field
        rather than a message with ``role: "system"``; it is folded into the
        message list as a leading system message so handlers can use
        :meth:`system_text` / :meth:`last_user_text` the same way regardless
        of which wire protocol the caller used.
        """
        raw_messages = body.get("messages") or []
        messages = [
            Message(role=m.get("role", "user"), content=_content_to_text(m.get("content")))
            for m in raw_messages
        ]
        system_text = _content_to_text(body.get("system"))
        if system_text:
            messages.insert(0, Message(role="system", content=system_text))
        known = {"model", "messages", "system", "stream"}
        extra = {k: v for k, v in body.items() if k not in known}
        return cls(
            model=body.get("model", ""),
            messages=messages,
            stream=bool(body.get("stream", False)),
            extra=extra,
        )


def _content_to_text(content: Any) -> str:
    """Flatten OpenAI content (str or list of parts) to plain text."""
    if content is None:
        return ""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts = []
        for part in content:
            if isinstance(part, dict):
                parts.append(part.get("text", ""))
            else:
                parts.append(str(part))
        return "".join(parts)
    return str(content)
