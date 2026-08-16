"""Guard-rail view — inspect what guard rails are active on the gateway.

Guard rails in tingly-box run *inline* on the request path, so they apply
automatically to every call an experiment makes — there is nothing the caller
must wire up. This view lets your code introspect the active policies (e.g. to
explain to *its* user what is enforced); a blocked request surfaces as a
:class:`~tingly.errors.GuardrailBlockedError` from the LLM call itself.

Reads ``GET /api/v1/guardrails/config`` into the gateway's own
``GuardrailsConfigResponse``. Notably ``Policy.enabled`` is a real field, so
"active" here means genuinely enabled — the previous hand-written version
counted every policy present in the file, enabled or not.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Optional

from .._api import ControlPlane
from .._generated.models import GuardrailsConfigResponse, Policy
from ..errors import TinglyError


@dataclass
class GuardrailStatus:
    """What the gateway is currently enforcing."""

    enabled: bool = False
    active_policies: int = 0
    policy_names: List[str] = field(default_factory=list)
    supported_scenarios: List[str] = field(default_factory=list)
    path: str = ""
    exists: bool = False

    @property
    def summary(self) -> str:
        """One line suitable for showing a user."""
        if not self.exists:
            return f"no guard-rail config at {self.path or '(unknown path)'}"
        if not self.enabled:
            return "guard-rail config present but no policies enabled"
        return f"{self.active_policies} policy(ies) enabled: {', '.join(self.policy_names)}"


class GuardrailsView:
    def __init__(self, gateway_url: str, admin_token: str, timeout: float):
        self._api = ControlPlane(gateway_url, admin_token, timeout=timeout)

    def status(self) -> GuardrailStatus:
        """Whether guard rails are configured and which policies are enabled."""
        cfg = self._config()
        if cfg is None:
            return GuardrailStatus()

        enabled = [p for p in (cfg.config.policies or []) if p.enabled]
        return GuardrailStatus(
            enabled=bool(enabled),
            active_policies=len(enabled),
            policy_names=[_label(p) for p in enabled],
            supported_scenarios=list(cfg.supported_scenarios),
            path=cfg.path,
            exists=cfg.exists,
        )

    def raw(self) -> Optional[GuardrailsConfigResponse]:
        """The full typed config — policies, groups, imports, raw YAML content."""
        return self._config()

    # -- internals -------------------------------------------------------

    def _config(self) -> Optional[GuardrailsConfigResponse]:
        try:
            return self._api.get("/api/v1/guardrails/config", GuardrailsConfigResponse)
        except TinglyError:
            # Guard rails being unconfigured is a normal state, not an error to
            # propagate into an experiment's own error handling.
            return None

    def close(self) -> None:
        self._api.close()


def _label(policy: Policy) -> str:
    """Prefer the human name, fall back to the id — never a bare '?'."""
    return policy.name or policy.id
