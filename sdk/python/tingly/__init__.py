"""tingly — Python SDK for tingly-box.

Write an LLM experiment or plugin in a handful of lines and reuse the gateway's
routing, fallback, guard rails, quota and logging:

    >>> import tingly
    >>> tb = tingly.connect(scenario="experiment")
    >>> tb.ask("Say hello", model="auto")
"""

from __future__ import annotations

from ._version import __version__
from .client import Client, connect
from .config import Connection, configure
from .errors import (
    AuthError,
    GatewayUnreachableError,
    GuardrailBlockedError,
    ScenarioNotFoundError,
    TinglyError,
    UpstreamError,
)
from .plugin import ChatRequest, Plugin

__all__ = [
    "__version__",
    "connect",
    "configure",
    "Connection",
    "Client",
    "Plugin",
    "ChatRequest",
    "TinglyError",
    "GatewayUnreachableError",
    "AuthError",
    "ScenarioNotFoundError",
    "GuardrailBlockedError",
    "UpstreamError",
]
