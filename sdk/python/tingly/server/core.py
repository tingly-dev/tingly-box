"""The ``Server`` class — write a tingly-box provider in Python.

A tingly ``Server`` is an ordinary LLM upstream: it answers Anthropic Messages
(``POST /v1/messages``) and OpenAI chat completions
(``POST /v1/chat/completions``), so tingly-box can point at it exactly the way
it points at Ollama, vLLM or LM Studio — a self-hosted provider with a base URL
and no key. There is no registration protocol, no manifest, and no plugin
lifecycle: you add it in the tb UI under **Connect AI → Self-hosted**, and you
retire it by deleting the provider.

What makes it more than a dumb model is the other direction: the handler may
call **back into** tingly-box via :attr:`Server.tb`, so its own LLM work
inherits routing, tiers, guard rails, quota and logging — and can target *any*
rule or model configured in the box.

    from tingly import Server

    srv = Server(name="rag")

    @srv.chat
    def handle(req):
        docs = retrieve(req.last_user_text())
        return srv.tb.ask(f"Using {docs}, answer: {req.last_user_text()}")

    if __name__ == "__main__":
        srv.run()
"""

from __future__ import annotations

import threading
from typing import TYPE_CHECKING, Callable, Optional

from .http import Dispatch, HandlerResult, make_server
from .types import ChatRequest

if TYPE_CHECKING:
    from ..config import Connection

ChatHandler = Callable[[ChatRequest], HandlerResult]

# Shared with the `tingly-python` provider template in tingly-box
# (internal/data/providers.json) so the pre-filled base URL in Connect AI and
# the port a Server actually binds cannot drift apart.
DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 8765


class Server:
    """A protocol-compliant LLM server that tingly-box consumes as a provider."""

    def __init__(
        self,
        name: str,
        *,
        model_id: Optional[str] = None,
        version: str = "0.1.0",
        description: str = "",
        api_key: str = "",
        scenario: str = "experiment",
    ):
        self.name = name
        # The provider is already the namespace, so the model id is just the
        # name — no prefix to type into the rule editor.
        self.model_id = model_id or name
        self.version = version
        self.description = description
        self.api_key = api_key
        self.scenario = scenario

        self._handler: Optional[ChatHandler] = None
        self._clients: dict = {}  # scenario -> lazily-connected client
        self._httpd = None

    # -- authoring -------------------------------------------------------

    def chat(self, fn: ChatHandler) -> ChatHandler:
        """Register the server's chat handler. Decorator form.

        The handler receives a :class:`ChatRequest` and returns either a string
        (buffered) or an iterator of strings (streamed). Both wire protocols
        are shaped from that one return value, so the handler never sees a
        request or response envelope.
        """
        self._handler = fn
        return fn

    @property
    def tb(self):
        """A lazily-connected client for calling back into tingly-box.

        Bound to this server's default calling context (``self.scenario``).
        Reusing the gateway for its own model calls is why a Python provider
        never hard-codes an upstream or a key — ``ask(model=...)`` can target
        any model tb routes. To drive a *different* rule-set, use :meth:`use`.
        """
        return self.use(self.scenario)

    def use(self, scenario: str):
        """Return a client bound to a specific scenario (rule-set) in tb.

        This is what composing the box looks like: hold clients to several
        scenarios and pick a model on each, so "use any other rule / model
        configured in tb" is one call:

            self.use("claude_code").ask("…", model="claude-sonnet-4-6")
            self.use("experiment").ask("…", model="auto")
        """
        client = self._clients.get(scenario)
        if client is None:
            from ..client import connect

            client = connect(scenario=scenario, name=f"server:{self.name}")
            self._clients[scenario] = client
        return client

    # -- dispatch --------------------------------------------------------

    def _dispatch(self, req: ChatRequest) -> HandlerResult:
        if self._handler is None:
            raise RuntimeError(
                f"server {self.name!r} has no chat handler; decorate one with @srv.chat"
            )
        return self._handler(req)

    # -- serving ---------------------------------------------------------

    def run(
        self,
        host: str = DEFAULT_HOST,
        port: int = DEFAULT_PORT,
        *,
        verbose: bool = True,
        block: bool = True,
        advertise_host: Optional[str] = None,
        tb: Optional["Connection"] = None,
    ) -> int:
        """Serve this model over HTTP.

        ``tb`` may be a :class:`tingly.config.Connection` to point the callback
        client at a specific gateway / inject credentials (containers / CI /
        remote). It affects only :attr:`tb` / :meth:`use` — serving itself
        needs nothing from tingly-box.

        Returns the bound port (resolved even when ``port=0``). ``block=False``
        runs the server on a daemon thread and returns immediately.
        """
        if tb is not None:
            from ..config import configure

            configure(url=tb.url, admin_token=tb.admin_token, admin_token_env=tb.admin_token_env)

        httpd, bound = make_server(
            self._dispatch,
            self.model_id,
            host=host,
            port=port,
            api_key=self.api_key,
            verbose=verbose,
        )
        self._httpd = httpd
        if verbose:
            print(self.connect_hint(advertise_host or host, bound))

        if not block:
            t = threading.Thread(target=httpd.serve_forever, daemon=True)
            t.start()
            return bound
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            pass
        finally:
            self.stop()
        return bound

    def connect_hint(self, host: str, port: int) -> str:
        """The exact values to paste into tb's Connect AI dialog.

        Printed on startup so the next action is in the same field of view as
        the thing that just started, rather than left as an exercise.
        """
        return "\n".join(
            [
                f"[tingly] serving model {self.model_id!r} on http://{host}:{port}",
                "[tingly] add it in tingly-box → Connect AI → Self-hosted → Python Server (tingly):",
                f"[tingly]   Base URL (Anthropic) : http://{host}:{port}",
                f"[tingly]   Base URL (OpenAI)    : http://{host}:{port}/v1",
                f"[tingly]   API key              : {self.api_key or '(none required)'}",
                f"[tingly]   Model                : {self.model_id}",
            ]
        )

    def stop(self) -> None:
        if self._httpd is not None:
            self._httpd.shutdown()
            self._httpd = None
