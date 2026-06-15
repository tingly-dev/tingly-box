"""Scenario + transport constants mirrored from the tingly-box backend.

These are the bindable scenarios most relevant to SDK users. The authoritative
list lives in ``internal/typ/type.go``; the gateway validates the scenario at
session time and returns the accepted transport, so this module is for
convenience and is not load-bearing.
"""

from __future__ import annotations

# Default scenario for experiments / plugins.
EXPERIMENT = "experiment"

# Other commonly useful bindable scenarios.
OPENAI = "openai"
ANTHROPIC = "anthropic"
AGENT = "agent"

# Transport labels returned by the gateway session response.
TRANSPORT_OPENAI = "openai"
TRANSPORT_ANTHROPIC = "anthropic"
TRANSPORT_BOTH = "both"


def supports_openai(transport: str) -> bool:
    return transport in (TRANSPORT_OPENAI, TRANSPORT_BOTH)


def supports_anthropic(transport: str) -> bool:
    return transport in (TRANSPORT_ANTHROPIC, TRANSPORT_BOTH)
