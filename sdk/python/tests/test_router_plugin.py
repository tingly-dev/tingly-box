"""Tests for the router showcase plugin (sdk/python/examples/router_plugin.py).

Unlike critic/fusion, a router doesn't generate anything — it resolves each
candidate model to its rule's single provider, picks the one with the most
quota headroom, and forwards with pin_provider= to guarantee that provider
is the one that actually serves the request. These tests pin that decision
logic with plugin.use()/rules/quota monkeypatched, no real tb.
"""

import importlib.util
import sys
from pathlib import Path

import pytest

from tingly.helpers.quota import ProviderQuota, UsageWindow
from tingly.helpers.rules import Rule
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


def _rule(model, *providers_active):
    """providers_active: e.g. [("p1", True)] for one active service, or
    [("p1", True), ("p2", True)] for a multi-service (non-routable) rule."""
    return Rule._from_json({
        "uuid": f"rule-{model}", "scenario": "experiment", "request_model": model,
        "services": [{"provider": p, "model": model, "active": active} for p, active in providers_active],
    })


def _quota(uuid, used_percent):
    return ProviderQuota(
        provider_uuid=uuid, provider_name=uuid, provider_type="anthropic",
        windows=[UsageWindow(key="session", type="session", used=used_percent, limit=100, used_percent=used_percent)],
    )


class _FakeRules:
    def __init__(self, rules_by_model):
        self._rules_by_model = rules_by_model

    def for_model(self, scenario, model):
        return self._rules_by_model.get(model)


class _FakeQuota:
    def __init__(self, quotas):
        self._quotas = quotas

    def batch(self, uuids):
        return {u: self._quotas[u] for u in uuids if u in self._quotas}

    def refresh(self, provider_uuid=None):
        raise AssertionError("refresh() should not be called by default routing")


class _FakeClient:
    def __init__(self, reply=None, rules=None, quota=None):
        self._reply = reply
        self.rules = rules
        self.quota = quota
        self.calls = []

    def ask(self, prompt, **kwargs):
        self.calls.append((prompt, kwargs))
        return self._reply


def test_resolve_candidates_skips_multi_service_rules(monkeypatch):
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATE_MODELS", ["sonnet1", "sonnet2", "sonnet3"])
    rules = _FakeRules({
        "sonnet1": _rule("sonnet1", ("p1", True)),                    # single active service — routable
        "sonnet2": _rule("sonnet2", ("p2", True), ("p3", True)),      # two active services — skip
        # "sonnet3" absent entirely (rule doesn't exist) — skip
    })
    monkeypatch.setattr(router.plugin, "use", lambda scenario: _FakeClient(rules=rules))

    resolved = router._resolve_candidates()

    assert [c.model for c in resolved] == ["sonnet1"]
    assert resolved[0].provider_uuid == "p1"


def test_resolve_candidates_skips_rule_with_no_active_services(monkeypatch):
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATE_MODELS", ["sonnet1"])
    rules = _FakeRules({"sonnet1": _rule("sonnet1", ("p1", False))})  # only inactive service
    monkeypatch.setattr(router.plugin, "use", lambda scenario: _FakeClient(rules=rules))

    assert router._resolve_candidates() == []


def test_pick_candidate_chooses_highest_headroom(monkeypatch):
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATE_MODELS", ["sonnet1", "sonnet2"])
    rules = _FakeRules({
        "sonnet1": _rule("sonnet1", ("p1", True)),
        "sonnet2": _rule("sonnet2", ("p2", True)),
    })
    quotas = _FakeQuota({"p1": _quota("p1", used_percent=90), "p2": _quota("p2", used_percent=10)})
    monkeypatch.setattr(router.plugin, "use", lambda scenario: _FakeClient(rules=rules, quota=quotas))

    chosen = router._pick_candidate()

    assert chosen.model == "sonnet2"  # 90% headroom beats 10%
    assert chosen.provider_uuid == "p2"


def test_pick_candidate_raises_when_no_routable_candidates(monkeypatch):
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATE_MODELS", ["sonnet1"])
    rules = _FakeRules({})  # no rule resolves
    monkeypatch.setattr(router.plugin, "use", lambda scenario: _FakeClient(rules=rules))

    with pytest.raises(RuntimeError, match="no router candidate"):
        router._pick_candidate()


def test_handle_forwards_with_pin_provider_for_the_chosen_candidate(monkeypatch):
    router = _load("router_plugin")
    monkeypatch.setattr(router, "CANDIDATE_MODELS", ["sonnet1", "sonnet2"])
    rules = _FakeRules({
        "sonnet1": _rule("sonnet1", ("p1", True)),
        "sonnet2": _rule("sonnet2", ("p2", True)),
    })
    quotas = _FakeQuota({"p1": _quota("p1", used_percent=90), "p2": _quota("p2", used_percent=10)})
    shared = _FakeClient(reply="the answer", rules=rules, quota=quotas)
    monkeypatch.setattr(router.plugin, "use", lambda scenario: shared)

    result = router.handle(_req("what's 2+2?"))

    assert result == "the answer"
    assert len(shared.calls) == 1
    prompt, kwargs = shared.calls[0]
    assert kwargs["model"] == "sonnet2"
    assert kwargs["pin_provider"] == "p2"
