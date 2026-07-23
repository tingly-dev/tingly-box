"""Register a running plugin with tingly-box.

A plugin registers once at startup (idempotent upsert by name — calling it
again, e.g. on every restart, updates the existing provider rather than
duplicating it). There is no heartbeat or lease: liveness is handled by the
same per-service circuit breaker that already protects every other tb
provider — if the plugin goes down, the next failed request trips the breaker
and traffic tier-fails-over (when a fallback tier is configured). If a plugin
is retired, delete its provider like any other, in the tb UI.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

import httpx

from .. import config as _config
from .._http import safe_json
from ..errors import AuthError, GatewayUnreachableError


@dataclass
class RegisterResult:
    provider_uuid: str
    model_id: str
    scenario: Optional[str]
    rule_uuid: Optional[str]
    ready: bool
    note: str


def register(
    name: str,
    endpoint: str,
    model_id: str,
    *,
    scenario: Optional[str] = None,
    token: str = "",
    tier: int = 0,
    api_style: str = "anthropic",
    gateway_url: Optional[str] = None,
    admin_token: Optional[str] = None,
    timeout: float = 30.0,
) -> RegisterResult:
    """Register (or update) this plugin as a tb provider, optionally binding a rule.

    ``api_style`` tells tb which wire protocol to use when it calls this
    plugin's endpoint — ``"anthropic"`` (the plugin server's primary route,
    ``/v1/messages``) or ``"openai"`` (``/v1/chat/completions``). The plugin
    server answers both regardless; this only picks which one tb sends.
    """
    resolved = _config.resolve(base_url=gateway_url, token=admin_token)
    headers = {"Authorization": f"Bearer {resolved.token or ''}"}
    url = resolved.base_url.rstrip("/") + "/api/v2/plugins"
    body = {
        "name": name, "endpoint": endpoint, "model_id": model_id,
        "scenario": scenario or "", "token": token, "tier": tier,
        "api_style": api_style,
    }
    try:
        resp = httpx.post(url, json=body, headers=headers, timeout=timeout)
    except httpx.HTTPError as exc:
        raise GatewayUnreachableError(f"could not reach tingly-box: {exc}") from exc
    if resp.status_code == 401:
        raise AuthError("tingly-box rejected the admin token during plugin register")
    payload = safe_json(resp) or {}
    if resp.status_code != 200 or not payload.get("success"):
        raise GatewayUnreachableError(
            f"plugin register failed: HTTP {resp.status_code} {resp.text[:200]}"
        )
    d = payload.get("data") or {}
    return RegisterResult(
        provider_uuid=d.get("provider_uuid", ""),
        model_id=d.get("model_id", model_id),
        scenario=d.get("scenario") or None,
        rule_uuid=d.get("rule_uuid") or None,
        ready=bool(d.get("ready", False)),
        note=d.get("note", ""),
    )
