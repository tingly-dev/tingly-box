"""Call tingly-box from Python.

`Client` is deliberately small: it knows how to reach one tb gateway
endpoint (`/tingly/{scenario}/v1/chat/completions`) and nothing about tb's
admin plane. Which rule/model actually answers is tb's existing
scenario+model routing — the SDK does not duplicate that logic.
"""

from __future__ import annotations

import json
import urllib.error
import urllib.request

DEFAULT_SCENARIO = "custom"


class TinglyError(RuntimeError):
    """Raised when tb returns a non-2xx response to a `Client` call."""

    def __init__(self, status: int, body: str):
        super().__init__(f"tb returned HTTP {status}: {body}")
        self.status = status
        self.body = body


class Client:
    """A gateway client bound to one tb instance.

    Args:
        base_url: tb's address, e.g. ``http://localhost:12580``.
        token: gateway token (tb's `ModelToken`, or a scoped multi-tenant
            API token) — the same credential any other `/tingly/*` caller
            uses, obtained from tb's settings/token UI.
        scenario: default scenario path segment when a call doesn't pass
            its own.
    """

    def __init__(self, base_url: str, token: str, scenario: str = DEFAULT_SCENARIO):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.scenario = scenario

    def chat(
        self,
        messages: list[dict],
        model: str,
        scenario: str | None = None,
        **extra,
    ) -> dict:
        """POST an OpenAI-shaped chat request through tb; returns the parsed
        OpenAI-shaped response as-is (no streaming in v1)."""
        url = f"{self.base_url}/tingly/{scenario or self.scenario}/v1/chat/completions"
        payload = {"model": model, "messages": messages, **extra}
        request = urllib.request.Request(
            url,
            data=json.dumps(payload).encode("utf-8"),
            method="POST",
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.token}",
            },
        )
        try:
            with urllib.request.urlopen(request) as resp:
                return json.load(resp)
        except urllib.error.HTTPError as exc:
            raise TinglyError(exc.code, exc.read().decode("utf-8", "replace")) from exc


def text_of(response: dict) -> str:
    """Convenience: pull the first choice's message content out of an
    OpenAI-shaped chat completion response."""
    choices = response.get("choices") or []
    if not choices:
        return ""
    return choices[0].get("message", {}).get("content", "")
