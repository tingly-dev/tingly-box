"""Call tingly-box from Python.

`Client.chat()` reaches one tb gateway endpoint
(`/tingly/{scenario}/v1/chat/completions`). Which rule/model actually
answers is tb's existing scenario+model routing — the SDK does not
duplicate that logic.

`Client` also exposes a small, deliberately narrow slice of tb's admin
plane: quota. Unlike the gateway call above, quota's response shapes are
already precisely specified in tb's own openapi.json — there is nothing to
guess, so this uses the real generated types (`task gen:py:quota`) instead
of hand-rolled dicts. See `.design/python-sdk.md`.
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
        admin_token: tb's `UserToken` — the actual admin-plane credential
            `/api/v1/*` (quota, and any admin call added later) checks.
            Defaults to `token`: on a typical single-operator box the two
            are the same secret. Pass this explicitly when they differ.
    """

    def __init__(
        self,
        base_url: str,
        token: str,
        scenario: str = DEFAULT_SCENARIO,
        admin_token: str | None = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.scenario = scenario
        self.admin_token = admin_token if admin_token is not None else token

    def chat(
        self,
        messages: list[dict],
        model: str,
        scenario: str | None = None,
        **extra,
    ) -> dict:
        """POST an OpenAI-shaped chat request through tb; returns the parsed
        OpenAI-shaped response as-is (no streaming in v1)."""
        path = f"/tingly/{scenario or self.scenario}/v1/chat/completions"
        payload = {"model": model, "messages": messages, **extra}
        return self._request(path, self.token, method="POST", body=payload)

    def list_quota(self):
        """`GET /api/v1/provider-quota` — cached quota for every provider
        that has a quota fetcher. Returns the generated `ListQuotaResponse`
        (`task gen:py:quota`)."""
        models = _quota_models()
        return models.ListQuotaResponse.model_validate(self._request("/api/v1/provider-quota", self.admin_token))

    def get_quota(self, provider_uuid: str):
        """`GET /api/v1/provider-quota/{uuid}` — quota for one provider,
        served from cache when fresh. Returns the generated `ProviderUsage`."""
        models = _quota_models()
        data = self._request(f"/api/v1/provider-quota/{provider_uuid}", self.admin_token)
        return models.ProviderUsage.model_validate(data)

    def quota_summary(self):
        """`GET /api/v1/provider-quota/summary` — aggregate quota summary
        across all providers. Returns the generated `Summary`."""
        models = _quota_models()
        return models.Summary.model_validate(self._request("/api/v1/provider-quota/summary", self.admin_token))

    def _request(self, path: str, token: str, method: str = "GET", body: dict | None = None) -> dict:
        request = urllib.request.Request(
            f"{self.base_url}{path}",
            data=json.dumps(body).encode("utf-8") if body is not None else None,
            method=method,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {token}",
            },
        )
        try:
            with urllib.request.urlopen(request) as resp:
                return json.load(resp)
        except urllib.error.HTTPError as exc:
            raise TinglyError(exc.code, exc.read().decode("utf-8", "replace")) from exc


def _quota_models():
    """Lazily import the generated quota models — only the quota methods
    need pydantic, so `import tingly` alone never requires it or the
    generated file to exist."""
    try:
        from . import _generated_quota
    except ImportError as exc:
        raise RuntimeError(
            "quota models are not generated — run `task gen:py:quota` (needs `pydantic` installed)"
        ) from exc
    return _generated_quota


def text_of(response: dict) -> str:
    """Convenience: pull the first choice's message content out of an
    OpenAI-shaped chat completion response."""
    choices = response.get("choices") or []
    if not choices:
        return ""
    return choices[0].get("message", {}).get("content", "")
