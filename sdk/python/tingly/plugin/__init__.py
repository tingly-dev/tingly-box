"""tingly.plugin — write an AI server tingly-box can route to as a model.

This is Layer 2 of the SDK. A :class:`Plugin` is an upstream tb can call as
Anthropic Messages (primary) or OpenAI chat completions (secondary); register
it as a provider in tingly-box and any client can select ``model_id``,
inheriting the gateway's routing / fallback / guard rails / quota / logging.
"""

from __future__ import annotations

from .core import Plugin
from .manifest import Manifest
from .types import ChatRequest, Message

__all__ = ["Plugin", "Manifest", "ChatRequest", "Message"]
