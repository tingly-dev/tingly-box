"""Rules view — read a scenario's rules and the services each one has
configured, so a caller can discover which provider(s) back a given model.

Existing views (`usage`, `guardrails`, `quota`) all read *what happened* or
*what's available*; this one reads *how a model resolves* — the missing
piece for anything that wants to act on a specific one of a rule's services
(e.g. `Client.ask(..., pin_provider=...)`), rather than letting tb pick.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

import httpx


@dataclass
class RuleService:
    """One of a rule's configured services — a (provider, model) binding tb
    can route to, at the given tier."""

    provider: str  # provider UUID
    model: str
    active: bool = True
    weight: int = 1
    tier: int = 0


@dataclass
class Rule:
    uuid: str
    scenario: str
    request_model: str
    active: bool = True
    services: List[RuleService] = field(default_factory=list)

    @property
    def active_services(self) -> List[RuleService]:
        return [s for s in self.services if s.active]

    def service_for_provider(self, provider_uuid: str) -> Optional[RuleService]:
        """The rule's own active service bound to this provider, if any —
        the set of providers valid to pass as `pin_provider=` for this rule's
        model. `None` means this provider isn't one of this rule's services;
        tb will reject a pin to it (`X-Tingly-Pin-Provider` is scoped to the
        resolved rule's own services, not any provider on the box)."""
        for svc in self.active_services:
            if svc.provider == provider_uuid:
                return svc
        return None

    @classmethod
    def _from_json(cls, d: Dict[str, Any]) -> "Rule":
        return cls(
            uuid=d.get("uuid", ""),
            scenario=d.get("scenario", ""),
            request_model=d.get("request_model", ""),
            active=d.get("active", True),
            services=[
                RuleService(
                    provider=s.get("provider", ""),
                    model=s.get("model", ""),
                    active=s.get("active", True),
                    weight=s.get("weight", 1),
                    tier=s.get("tier", 0),
                )
                for s in d.get("services") or []
            ],
        )


class RulesView:
    def __init__(self, gateway_url: str, admin_token: str, timeout: float):
        self._gateway_url = gateway_url.rstrip("/")
        self._admin_token = admin_token
        self._timeout = timeout

    def list(self, scenario: str) -> List[Rule]:
        """All rules configured under a scenario (required — tb 400s without it)."""
        resp = httpx.get(
            f"{self._gateway_url}/api/v1/rules",
            params={"scenario": scenario},
            headers={"Authorization": f"Bearer {self._admin_token}"},
            timeout=self._timeout,
        )
        resp.raise_for_status()
        data = resp.json().get("data") or []
        return [Rule._from_json(d) for d in data]

    def for_model(self, scenario: str, model: str) -> Optional[Rule]:
        """The rule whose `request_model` matches, if any — the common case
        (a router deciding which provider backs *this* model)."""
        for rule in self.list(scenario):
            if rule.request_model == model:
                return rule
        return None
