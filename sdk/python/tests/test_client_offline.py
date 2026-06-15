"""Client transport-routing tests with a fake session (no real SDKs invoked)."""

import pytest

from tingly.client import Client
from tingly.discovery import Session
from tingly.errors import TinglyError


def _client(transport: str) -> Client:
    session = Session(
        base_url="http://tb.test:12580/tingly/experiment",
        token="model-tok",
        scenario="experiment",
        transport=transport,
        ready=True,
        services=1,
    )
    return Client(session, "http://tb.test:12580", "admin", "exp", 30.0)


def test_anthropic_only_rejects_openai():
    c = _client("anthropic")
    with pytest.raises(TinglyError):
        _ = c.openai


def test_openai_only_rejects_anthropic():
    c = _client("openai")
    with pytest.raises(TinglyError):
        _ = c.anthropic


def test_both_exposes_identity():
    c = _client("both")
    assert c.scenario == "experiment"
    assert c.transport == "both"
    assert c.ready is True
    assert c.base_url.endswith("/tingly/experiment")


def test_client_passes_scenario_root_to_transports(monkeypatch):
    """The client hands the scenario root + model token to each builder;
    per-transport URL shaping (e.g. the OpenAI /v1 suffix) happens inside the
    builder."""
    captured = {}

    def fake_openai(base_url, token, timeout):
        captured["openai_base"] = base_url
        captured["openai_token"] = token
        return object()

    def fake_anthropic(base_url, token, timeout):
        captured["anthropic_base"] = base_url
        return object()

    monkeypatch.setattr("tingly.transports.openai_compat.build_openai", fake_openai)
    monkeypatch.setattr(
        "tingly.transports.anthropic_compat.build_anthropic", fake_anthropic
    )
    c = _client("both")
    _ = c.openai
    _ = c.anthropic
    assert captured["openai_base"] == "http://tb.test:12580/tingly/experiment"
    assert captured["openai_token"] == "model-tok"
    assert captured["anthropic_base"] == "http://tb.test:12580/tingly/experiment"
