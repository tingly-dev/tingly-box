"""Register a running plugin with tingly-box as an upstream provider.

This is the Layer 3 wiring: it creates a provider whose ``api_base`` points at
the plugin's HTTP server, so tingly-box can route a model id to it. Creating the
*rule/service* that maps the model into a scenario is left to the user (or the
tb UI) for now — the provider is the part the SDK can do safely and idempotently.
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
    created: bool
    note: str


def register_with_tb(
    name: str,
    plugin_url: str,
    model_id: str,
    *,
    gateway_url: Optional[str] = None,
    admin_token: Optional[str] = None,
    token: str = "",
    timeout: float = 30.0,
) -> RegisterResult:
    """Create (or report) a tingly-box provider pointing at the plugin.

    Args:
        name: provider name to create in tb (e.g. the plugin name).
        plugin_url: the plugin's OpenAI base, e.g. ``http://127.0.0.1:8765/v1``.
        model_id: the model id the plugin advertises (e.g. ``plugin/my-rag``).
        gateway_url / admin_token: tb gateway + admin token; auto-discovered if
            omitted (same precedence as ``connect()``).
        token: optional API token tb should send to the plugin (matches the
            plugin's ``api_key`` if it enforces one).
    """
    resolved = _config.resolve(base_url=gateway_url, token=admin_token)
    headers = {"Authorization": f"Bearer {resolved.token or ''}"}
    url = resolved.base_url.rstrip("/") + "/api/v1/providers"
    payload = {
        "name": name,
        "api_base": plugin_url,
        "api_style": "openai",
        "token": token,
        "no_key_required": token == "",
        "enabled": True,
        "auth_type": "api_key",
    }

    try:
        resp = httpx.post(url, json=payload, headers=headers, timeout=timeout)
    except httpx.HTTPError as exc:
        raise GatewayUnreachableError(
            f"could not reach tingly-box at {resolved.base_url}: {exc}"
        ) from exc

    if resp.status_code == 401:
        raise AuthError("tingly-box rejected the admin token while creating the provider")

    note = (
        f"Provider created. Bind a rule mapping model {model_id!r} to this "
        f"provider (tb UI → Rules) so clients can select it."
    )
    created = resp.status_code in (200, 201)
    if not created:
        # A name clash (already registered) is fine and idempotent enough.
        note = (
            f"Provider not created (HTTP {resp.status_code}: {resp.text[:160]}). "
            f"It may already exist — verify in the tb UI."
        )

    data = _safe_json(resp) or {}
    provider_uuid = (
        (data.get("data") or {}).get("uuid")
        or data.get("uuid")
        or None
    )
    return RegisterResult(
        provider_uuid=provider_uuid,
        name=name,
        api_base=plugin_url,
        model_id=model_id,
        created=created,
        note=note,
    )


def _safe_json(resp: httpx.Response):
    try:
        return resp.json()
    except ValueError:
        return None
