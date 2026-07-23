"""Tests for the router showcase plugin (sdk/python/examples/router_plugin.py).

Unlike critic/fusion, a router doesn't generate anything — it picks ONE
candidate by quota headroom and forwards only to that one. These tests pin
that decision logic with plugin.use()/quota monkeypatched, no real tb.
"""

import importlib.util
import sys
from pathlib import Path

from tingly.helpers.quota import ProviderQuota, UsageWindow
from tingly.plugin.types import ChatRequest

EXAMPLES = Path(__file__).parent.parent / "examples"


def _load(name):
    path = EXAMPLES / f"{name}.py"
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


def _req(content):
    return ChatRequest.from_openai_body({"model": "x", "messages": [{"role": "user", "content": content}]})


def _quota(uuid, used_percent):
    return ProviderQuota(
        provider_uuid=uuid, provider_name=uuid, provider_type="anthropic",
        windows=[UsageWindow(key="session", type="session", used=used_percent, limit=100, used_percent=used_percent)],
    )


class _FakeQuota:
    def __init__(self, quotas):
        self._quotas = quotas

    def batch(self, uuids):
        return {u: self._quotas[u] for u in uuids if u in self._quotas}


class _FakeClient:
    def __init__(self, reply=None, quota=None):
        self._reply = reply
        self.quota = quota
        self.calls = []

    def ask(self, prompt, **kwargs):
        self.calls.append((prompt, kwargs))
        return self._reply


def test_pick_candidate_chooses_highest_headroom(monkeypatch):
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATES", [
        router.Candidate(scenario="experiment", model="m1", provider_uuid="u1"),
        router.Candidate(scenario="experiment", model="m2", provider_uuid="u2"),
    ])
    quotas = _FakeQuota({"u1": _quota("u1", used_percent=90), "u2": _quota("u2", used_percent=10)})
    monkeypatch.setattr(router.plugin, "use", lambda scenario: _FakeClient(quota=quotas))

    chosen = router._pick_candidate()

    assert chosen.provider_uuid == "u2"  # 90% headroom beats 10%


def test_pick_candidate_defaults_missing_quota_to_full_headroom(monkeypatch):
    """A candidate tb has no quota data for yet must not be starved out by
    one that does — treat "unknown" the same as "unconstrained", not zero."""
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATES", [
        router.Candidate(scenario="experiment", model="m1", provider_uuid="u1"),
        router.Candidate(scenario="experiment", model="m2", provider_uuid="unknown"),
    ])
    quotas = _FakeQuota({"u1": _quota("u1", used_percent=90)})  # "unknown" absent
    monkeypatch.setattr(router.plugin, "use", lambda scenario: _FakeClient(quota=quotas))

    chosen = router._pick_candidate()

    assert chosen.provider_uuid == "unknown"


def test_handle_forwards_only_to_the_chosen_candidate(monkeypatch):
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATES", [
        router.Candidate(scenario="s1", model="m1", provider_uuid="u1"),
        router.Candidate(scenario="s2", model="m2", provider_uuid="u2"),
    ])
    quotas = _FakeQuota({"u1": _quota("u1", used_percent=90), "u2": _quota("u2", used_percent=10)})
    llm_client = _FakeClient(quota=quotas)
    s2_client = _FakeClient(reply="answer from s2")

    def fake_use(scenario):
        if scenario == router.plugin.scenario:
            return llm_client
        if scenario == "s2":
            return s2_client
        raise AssertionError(f"must not forward to the un-chosen candidate's scenario: {scenario!r}")

    monkeypatch.setattr(router.plugin, "use", fake_use)

    result = router.handle(_req("what's 2+2?"))

    assert result == "answer from s2"
    assert len(s2_client.calls) == 1
    assert s2_client.calls[0][1]["model"] == "m2"
