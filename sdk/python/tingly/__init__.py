"""tingly — Python SDK for tingly-box.

Two independent halves.

**Consume the box** — reuse its routing, fallback, guard rails, quota and
logging for your own calls:

    >>> import tingly
    >>> tb = tingly.connect(scenario="experiment")
    >>> tb.ask("Say hello", model="auto")

**Be a provider** — write a model server the box can route to, added in the tb
UI like any other self-hosted upstream:

    >>> srv = tingly.Server(name="rag")
    >>> @srv.chat
    ... def handle(req):
    ...     return srv.tb.ask(req.last_user_text())
    >>> srv.run()

Either half stands alone; using both is what lets a Python provider compose the
whole box.
"""

from __future__ import annotations

from ._version import __version__
from .client import Client, connect
from .config import Connection, configure
from .errors import (
    APIStatusError,
    AuthError,
    GatewayUnreachableError,
    GuardrailBlockedError,
    ScenarioNotFoundError,
    SchemaMismatchError,
    TinglyError,
    UpstreamError,
)
from .server import ChatRequest, Server

__all__ = [
    "__version__",
    "connect",
    "configure",
    "Connection",
    "Client",
    "Server",
    "ChatRequest",
    "TinglyError",
    "GatewayUnreachableError",
    "AuthError",
    "ScenarioNotFoundError",
    "GuardrailBlockedError",
    "UpstreamError",
    "APIStatusError",
    "SchemaMismatchError",
]
