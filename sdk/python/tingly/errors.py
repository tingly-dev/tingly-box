"""Error hierarchy for the tingly SDK.

Every failure a user can hit maps to one of these so plugin authors can react
precisely — and, in the guardrail case, explain to *their* user why a request
was refused rather than surfacing a bare "it failed".
"""

from __future__ import annotations

from typing import Optional


class TinglyError(Exception):
    """Base class for all tingly SDK errors."""


class GatewayUnreachableError(TinglyError):
    """The tingly-box gateway could not be discovered or reached.

    Raised during ``connect()`` discovery or when a request cannot establish a
    connection to the gateway.
    """


class AuthError(TinglyError):
    """The gateway rejected the supplied token (HTTP 401)."""


class ScenarioNotFoundError(TinglyError):
    """The requested scenario is unknown or not bindable (HTTP 404).

    ``valid_scenarios`` carries the gateway's list of bindable scenarios so the
    caller can suggest a correct value.
    """

    def __init__(self, scenario: str, valid_scenarios: Optional[list] = None):
        self.scenario = scenario
        self.valid_scenarios = valid_scenarios or []
        valid = ", ".join(self.valid_scenarios) if self.valid_scenarios else "(none reported)"
        super().__init__(
            f"scenario {scenario!r} is unknown or not bindable. "
            f"Valid scenarios: {valid}"
        )


class GuardrailBlockedError(TinglyError):
    """tingly-box refused the request due to a guard-rail policy.

    ``policy_id`` and ``reason`` are surfaced so the caller can show *why*.
    """

    def __init__(self, reason: str, policy_id: Optional[str] = None):
        self.policy_id = policy_id
        self.reason = reason
        prefix = f"[{policy_id}] " if policy_id else ""
        super().__init__(f"{prefix}request blocked by guard rail: {reason}")


class UpstreamError(TinglyError):
    """An upstream LLM provider returned a server error (HTTP 5xx)."""

    def __init__(self, message: str, status_code: Optional[int] = None):
        self.status_code = status_code
        super().__init__(message)
