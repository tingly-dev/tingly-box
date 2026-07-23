"""The tingly Client and the ``connect()`` entrypoint.

``connect()`` is the whole surface area for "experiment ASAP": auto-discover the
local gateway, mint a scenario-bound session, and hand back a Client whose
``.openai`` / ``.anthropic`` SDK objects are already pointed at tingly-box — so
every request inherits the gateway's routing, fallback, guard rails, quota and
logging without the experiment knowing anything about them.
"""

from __future__ import annotations

from typing import Any, Iterator, Optional

from . import config as _config
from . import discovery as _discovery
from . import scenarios as _scenarios
from .errors import TinglyError
from .helpers.guardrails import GuardrailsView
from .helpers.usage import UsageView
from .transports import anthropic_compat, openai_compat


class Client:
    """A tingly-box-bound LLM client.

    Construct via :func:`connect`, not directly. Holds the minted session (for
    LLM calls) plus the gateway root + admin token (for management views).
    """

    def __init__(
        self,
        session: "_discovery.Session",
        gateway_url: str,
        admin_token: str,
        name: str,
        timeout: float,
    ):
        self._session = session
        self._gateway_url = gateway_url
        self._admin_token = admin_token
        self.name = name
        self._timeout = timeout

        self._openai: Optional[Any] = None
        self._anthropic: Optional[Any] = None

    # -- identity --------------------------------------------------------

    @property
    def base_url(self) -> str:
        return self._session.base_url

    @property
    def scenario(self) -> str:
        return self._session.scenario

    @property
    def transport(self) -> str:
        return self._session.transport

    @property
    def ready(self) -> bool:
        """True when the scenario has an active rule with at least one service."""
        return self._session.ready

    # -- SDK pass-throughs ----------------------------------------------

    @property
    def openai(self) -> Any:
        """A lazily-built ``openai.OpenAI`` bound to this scenario."""
        if not _scenarios.supports_openai(self._session.transport):
            raise TinglyError(
                f"scenario {self.scenario!r} does not accept the OpenAI transport "
                f"(transport={self.transport!r})"
            )
        if self._openai is None:
            self._openai = openai_compat.build_openai(
                self._session.base_url, self._session.token, self._timeout
            )
        return self._openai

    @property
    def anthropic(self) -> Any:
        """A lazily-built ``anthropic.Anthropic`` bound to this scenario."""
        if not _scenarios.supports_anthropic(self._session.transport):
            raise TinglyError(
                f"scenario {self.scenario!r} does not accept the Anthropic transport "
                f"(transport={self.transport!r})"
            )
        if self._anthropic is None:
            self._anthropic = anthropic_compat.build_anthropic(
                self._session.base_url, self._session.token, self._timeout
            )
        return self._anthropic

    # -- convenience -----------------------------------------------------

    def ask(
        self,
        prompt: str,
        *,
        model: str = "auto",
        system: Optional[str] = None,
        max_tokens: int = 1024,
        stream: bool = False,
        **kwargs: Any,
    ):
        """One-shot prompt → text, routed through tingly-box.

        Picks the transport from the scenario: Anthropic messages is tried
        first (tb's native protocol); OpenAI-only scenarios fall back to chat
        completions. ``model="auto"`` lets the gateway route.
        """
        if _scenarios.supports_anthropic(self._session.transport):
            return self._ask_anthropic(prompt, model, system, max_tokens, stream, **kwargs)
        return self._ask_openai(prompt, model, system, stream, **kwargs)

    def _ask_openai(self, prompt, model, system, stream, **kwargs):
        messages = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})
        resp = self.openai.chat.completions.create(
            model=model, messages=messages, stream=stream, **kwargs
        )
        if stream:
            return self._stream_openai(resp)
        return resp.choices[0].message.content

    @staticmethod
    def _stream_openai(resp) -> Iterator[str]:
        for chunk in resp:
            delta = chunk.choices[0].delta.content
            if delta:
                yield delta

    def _ask_anthropic(self, prompt, model, system, max_tokens, stream, **kwargs):
        params = dict(
            model=model,
            max_tokens=max_tokens,
            messages=[{"role": "user", "content": prompt}],
            **kwargs,
        )
        if system:
            params["system"] = system
        if stream:
            return self._stream_anthropic(params)
        resp = self.anthropic.messages.create(**params)
        return "".join(block.text for block in resp.content if hasattr(block, "text"))

    def _stream_anthropic(self, params) -> Iterator[str]:
        with self.anthropic.messages.stream(**params) as stream:
            for text in stream.text_stream:
                yield text

    # -- management views ------------------------------------------------

    @property
    def usage(self) -> UsageView:
        return UsageView(self._gateway_url, self._admin_token, self.name, self._timeout)

    @property
    def guardrails(self) -> GuardrailsView:
        return GuardrailsView(self._gateway_url, self._admin_token, self._timeout)

    # -- lifecycle -------------------------------------------------------

    def close(self) -> None:
        for c in (self._openai, self._anthropic):
            closer = getattr(c, "close", None)
            if callable(closer):
                try:
                    closer()
                except Exception:
                    pass

    def __enter__(self) -> "Client":
        return self

    def __exit__(self, *exc) -> None:
        self.close()


def connect(
    scenario: str = _scenarios.EXPERIMENT,
    *,
    base_url: Optional[str] = None,
    token: Optional[str] = None,
    timeout: float = 60.0,
    name: Optional[str] = None,
) -> Client:
    """Connect to a local tingly-box gateway and return a bound Client.

    Args:
        scenario: the rule scenario to bind to. Defaults to ``"experiment"``.
        base_url: gateway root, e.g. ``http://127.0.0.1:12580``. Auto-discovered
            if omitted (env → ``~/.tingly-box/sdk.json`` → ``config.json`` →
            localhost probe).
        token: admin token used to provision the session. Auto-discovered if
            omitted.
        timeout: per-request timeout in seconds for the LLM clients.
        name: a label that identifies this experiment in tingly-box logs.

    Raises:
        GatewayUnreachableError: no gateway responded.
        AuthError: the admin token was rejected.
        ScenarioNotFoundError: the scenario is unknown or not bindable.
    """
    resolved = _config.resolve(base_url=base_url, token=token)

    if _discovery.probe_version(resolved.base_url) is None:
        from .errors import GatewayUnreachableError

        raise GatewayUnreachableError(
            f"no tingly-box gateway responding at {resolved.base_url} "
            f"(resolved via {resolved.source}). Is `tb` running? "
            f"Set TINGLY_BOX_URL or run `tingly doctor`."
        )

    caller = name or "tingly-sdk"
    session = _discovery.create_session(
        base_url=resolved.base_url,
        admin_token=resolved.token or "",
        scenario=scenario,
        name=caller,
        timeout=timeout,
    )

    return Client(
        session=session,
        gateway_url=resolved.base_url,
        admin_token=resolved.token or "",
        name=caller,
        timeout=timeout,
    )
