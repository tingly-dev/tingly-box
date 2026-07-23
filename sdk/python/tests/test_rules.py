"""RulesView tests (gateway mocked with respx)."""

import httpx
import respx

from tingly.helpers.rules import Rule, RulesView

BASE = "http://tb.test:12580"


def _view() -> RulesView:
    return RulesView(BASE, "admin", 5.0)


@respx.mock
def test_list_requires_scenario_query_param():
    route = respx.get(f"{BASE}/api/v1/rules", params={"scenario": "experiment"}).mock(
        return_value=httpx.Response(200, json={"success": True, "data": []})
    )
    _view().list("experiment")
    assert route.called


@respx.mock
def test_list_parses_services():
    respx.get(f"{BASE}/api/v1/rules").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "data": [{
                "uuid": "r1", "scenario": "experiment", "request_model": "sonnet1", "active": True,
                "services": [
                    {"provider": "p1", "model": "claude-sonnet-4-6", "active": True, "weight": 1, "tier": 0},
                    {"provider": "p2", "model": "claude-sonnet-4-6", "active": False, "weight": 1, "tier": 1},
                ],
            }],
        })
    )
    rules = _view().list("experiment")
    assert len(rules) == 1
    assert rules[0].request_model == "sonnet1"
    assert len(rules[0].services) == 2
    assert [s.provider for s in rules[0].active_services] == ["p1"]


@respx.mock
def test_for_model_finds_matching_rule():
    respx.get(f"{BASE}/api/v1/rules").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "data": [
                {"uuid": "r1", "scenario": "experiment", "request_model": "sonnet1", "services": []},
                {"uuid": "r2", "scenario": "experiment", "request_model": "sonnet2", "services": []},
            ],
        })
    )
    rule = _view().for_model("experiment", "sonnet2")
    assert rule is not None
    assert rule.uuid == "r2"


@respx.mock
def test_for_model_returns_none_when_no_match():
    respx.get(f"{BASE}/api/v1/rules").mock(
        return_value=httpx.Response(200, json={"success": True, "data": []})
    )
    assert _view().for_model("experiment", "nope") is None


def test_service_for_provider_ignores_inactive():
    rule = Rule._from_json({
        "uuid": "r1", "scenario": "experiment", "request_model": "sonnet1",
        "services": [
            {"provider": "p1", "model": "m", "active": False},
            {"provider": "p2", "model": "m", "active": True},
        ],
    })
    assert rule.service_for_provider("p1") is None
    assert rule.service_for_provider("p2") is not None
    assert rule.service_for_provider("unknown") is None
