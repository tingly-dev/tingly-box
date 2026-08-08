"""tingly.server — write a tingly-box provider in Python.

A :class:`Server` is an ordinary LLM upstream speaking Anthropic Messages and
OpenAI chat completions. Point tingly-box at it the same way you'd point it at
Ollama — Connect AI → Self-hosted — and any client can select ``model_id``,
inheriting the gateway's routing / fallback / guard rails / quota / logging.
"""

from __future__ import annotations

from .core import Server
from .types import ChatRequest, Message

__all__ = ["Server", "ChatRequest", "Message"]
