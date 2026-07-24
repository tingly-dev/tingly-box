"""Guard-rail view — inspect what guard rails are active on the gateway.

Guard rails in tingly-box run *inline* on the request path, so they are applied
automatically to every call an experiment makes — there is nothing the plugin
author must wire up. This view lets a plugin introspect the active policies
(e.g. to explain to its user what is enforced); a blocked request surfaces as a
:class:`~tingly.errors.GuardrailBlockedError` from the LLM call itself.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import List

import httpx


@dataclass
class GuardrailStatus:
    enabled: bool = False
    active_policies: int = 0
    policy_names: List[str] = None  # type: ignore[assignment]

    def __post_init__(self):
        if self.policy_names is None:
            self.policy_names = []


class GuardrailsView:
    def __init__(self, gateway_url: str, admin_token: str, timeout: float):
        self._gateway_url = gateway_url.rstrip("/")
        self._admin_token = admin_token
        self._timeout = timeout

    def status(self) -> GuardrailStatus:
        """Return whether guard rails are enabled and how many policies are active."""
        url = f"{self._gateway_url}/api/v1/guardrails/config"
        headers = {"Authorization": f"Bearer {self._admin_token}"}
        try:
            resp = httpx.get(url, headers=headers, timeout=self._timeout)
            resp.raise_for_status()
            payload = resp.json()
        except (httpx.HTTPError, ValueError):
            return GuardrailStatus()

        data = payload.get("data") or payload
        policies = data.get("policies") or []
        names = [p.get("name", p.get("id", "?")) for p in policies if isinstance(p, dict)]
        return GuardrailStatus(
            enabled=bool(data.get("enabled", bool(policies))),
            active_policies=len(policies),
            policy_names=names,
        )
