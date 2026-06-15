"""Register a running plugin with tingly-box in one step.

Calls the first-class ``POST /api/v2/plugins`` endpoint, which creates a
plugin-kind provider pointing at the plugin's HTTP server and — when a scenario
is given — the rule whose upstream is that plugin. The plugin then composes with
tb's routing, fallback, guard rails, quota and logging like any other model.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

import httpx

from .. import config as _config
from ..errors import AuthError, GatewayUnreachableError


@dataclass
class RegisterResult:
    provider_uuid: Optional[str]
    name: str
    api_base: str
    model_id: str
    scenario: Optional[str]
    rule_uuid: Optional[str]
    ready: bool
    note: str


def register_with_tb(
    name: str,
    plugin_url: str,
    model_id: str,
    *,
    scenario: Optional[str] = None,
    gateway_url: Optional[str] = None,
    admin_token: Optional[str] = None,
    token: str = "",
    tier: int = 0,
    timeout: float = 30.0,
) -> RegisterResult:
    """Wire a plugin into tingly-box in one call (provider + optional rule).

    Args:
        name: plugin / provider name to create in tb.
        plugin_url: the plugin's OpenAI base, e.g. ``http://127.0.0.1:8765/v1``.
        model_id: the model id the plugin advertises (e.g. ``plugin/my-rag``).
        scenario: bind a rule under this scenario so the model is selectable
            immediately; omit to create only the provider.
        gateway_url / admin_token: tb gateway + admin token; auto-discovered if
            omitted (same precedence as ``connect()``).
        token: optional API token tb should send to the plugin (matches the
            plugin's ``api_key`` if it enforces one).
        tier: tier for the bound service (0 = highest priority).
    """
    resolved = _config.resolve(base_url=gateway_url, token=admin_token)
    headers = {"Authorization": f"Bearer {resolved.token or ''}"}
    url = resolved.base_url.rstrip("/") + "/api/v2/plugins"
    payload = {
        "name": name,
        "endpoint": plugin_url,
        "model_id": model_id,
        "token": token,
        "scenario": scenario or "",
        "tier": tier,
    }

    try:
        resp = httpx.post(url, json=payload, headers=headers, timeout=timeout)
    except httpx.HTTPError as exc:
        raise GatewayUnreachableError(
            f"could not reach tingly-box at {resolved.base_url}: {exc}"
        ) from exc

    if resp.status_code == 401:
        raise AuthError("tingly-box rejected the admin token while registering the plugin")

    payload_data = _safe_json(resp) or {}
    if resp.status_code not in (200, 201) or not payload_data.get("success"):
        raise GatewayUnreachableError(
            f"plugin registration failed: HTTP {resp.status_code} {resp.text[:200]}"
        )

    data = payload_data.get("data") or {}
    return RegisterResult(
        provider_uuid=data.get("provider_uuid"),
        name=name,
        api_base=plugin_url,
        model_id=data.get("model_id", model_id),
        scenario=data.get("scenario") or None,
        rule_uuid=data.get("rule_uuid") or None,
        ready=bool(data.get("ready", False)),
        note=data.get("note", ""),
    )


def _safe_json(resp: httpx.Response):
    try:
        return resp.json()
    except ValueError:
        return None
