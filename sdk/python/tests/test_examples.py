"""Tests for the example servers (sdk/python/examples/).

Every example is a provider that dispatches to tb rules, so what these pin is
the *dispatch decision* — which rule each request is routed to — rather than
any model output. srv.use() / srv.tb are monkeypatched, so no real tb and no
real model calls. The examples aren't part of the installed `tingly` package,
so they're loaded by file path.
"""

import importlib.util
import sys
from pathlib import Path

from tingly.server.types import ChatRequest

EXAMPLES = Path(__file__).parent.parent / "examples"


def _load(name):
    path = EXAMPLES / f"{name}.py"
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


def _req(content, system=None):
    messages = []
    if system:
        messages.append({"role": "system", "content": system})
    messages.append({"role": "user", "content": content})
    return ChatRequest.from_openai_body({"model": "x", "messages": messages})


class _FakeClient:
    """Stands in for a tingly.Client — records calls, replies with a fixed
    string or (if given a callable) the result of calling it with the prompt."""

    def __init__(self, reply):
        self._reply = reply
        self.calls = []

    def ask(self, prompt, **kwargs):
        self.calls.append((prompt, kwargs))
        return self._reply(prompt) if callable(self._reply) else self._reply


# -- critic -----------------------------------------------------------------

def test_critic_formats_valid_json_verdict(monkeypatch):
    critic = _load("critic_server")
    fake = _FakeClient('{"verdict": "approve", "issues": [], "suggestion": "looks good"}')
    monkeypatch.setattr(critic.srv, "use", lambda scenario: fake)

    result = critic.handle(_req("def f(): return 1/0", system="a python snippet"))

    assert result == "verdict: approve\nsuggestion: looks good"
    prompt, kwargs = fake.calls[0]
    assert "a python snippet" in prompt
    assert "def f(): return 1/0" in prompt
    assert kwargs["model"] == critic.CRITIC_MODEL


def test_critic_lists_issues(monkeypatch):
    critic = _load("critic_server")
    fake = _FakeClient('{"verdict": "revise", "issues": ["divides by zero"], "suggestion": "guard the denominator"}')
    monkeypatch.setattr(critic.srv, "use", lambda scenario: fake)

    result = critic.handle(_req("def f(): return 1/0"))

    assert "verdict: revise" in result
    assert "- divides by zero" in result
    assert "suggestion: guard the denominator" in result


def test_critic_degrades_gracefully_on_non_json(monkeypatch):
    """A critic model that ignores the JSON contract must not crash the
    request — it should surface as a 'revise' verdict carrying the raw text."""
    critic = _load("critic_server")
    fake = _FakeClient("looks fine to me")
    monkeypatch.setattr(critic.srv, "use", lambda scenario: fake)

    result = critic.handle(_req("some code"))

    assert "verdict: revise" in result
    assert "looks fine to me" in result


def test_critic_strips_markdown_code_fence(monkeypatch):
    critic = _load("critic_server")
    fake = _FakeClient('```json\n{"verdict": "approve", "issues": [], "suggestion": ""}\n```')
    monkeypatch.setattr(critic.srv, "use", lambda scenario: fake)

    result = critic.handle(_req("some code"))

    assert result == "verdict: approve"


# -- shared fakes for rule discovery ----------------------------------------

class _FakeRule:
    """Stands in for a generated models.Rule — only the fields examples read."""

    def __init__(self, request_model, active=True, provider=None):
        self.request_model = request_model
        self.active = active
        self.services = [_FakeService(provider)] if provider else []


class _FakeService:
    def __init__(self, provider, active=True):
        self.provider = provider
        self.active = active


class _FakeTB:
    def __init__(self, rules=(), quota=None):
        self._rules = list(rules)
        self.quota = quota

    def rules(self, scenario=None):
        return self._rules


def _bind_tb(monkeypatch, module, tb):
    """Point module.srv.tb at a fake without touching the real property."""
    monkeypatch.setattr(type(module.srv), "tb", property(lambda self: tb), raising=False)


# -- fusion -------------------------------------------------------------

def test_panel_is_discovered_and_deduplicated(monkeypatch):
    """The panel used to be two hard-coded copies of the same rule, which is
    latency without a second opinion. Members must be distinct."""
    fusion = _load("fusion_server")
    _bind_tb(monkeypatch, fusion, _FakeTB([
        _FakeRule("gpt-5"),
        _FakeRule("gpt-5"),                 # duplicate rule id
        _FakeRule("claude-sonnet-4-6"),
        _FakeRule("stale", active=False),   # inactive
        _FakeRule("gemini-3-pro"),
        _FakeRule("extra"),                 # beyond PANEL_SIZE
    ]))
    panel = fusion._panel()
    assert panel == ["gpt-5", "claude-sonnet-4-6", "gemini-3-pro"]
    assert len(panel) == len(set(panel)) == fusion.PANEL_SIZE


def test_judge_prefers_a_rule_the_panel_did_not_use(monkeypatch):
    fusion = _load("fusion_server")
    _bind_tb(monkeypatch, fusion, _FakeTB([
        _FakeRule("gpt-5"), _FakeRule("claude-opus-4-6")
    ]))
    assert fusion._judge(["gpt-5"]) == "claude-opus-4-6"


def test_judge_falls_back_to_a_panel_member_when_nothing_else_exists(monkeypatch):
    fusion = _load("fusion_server")
    _bind_tb(monkeypatch, fusion, _FakeTB([_FakeRule("gpt-5")]))
    assert fusion._judge(["gpt-5"]) == "gpt-5"


def test_single_rule_box_answers_directly_instead_of_paying_for_a_judge(monkeypatch):
    fusion = _load("fusion_server")
    _bind_tb(monkeypatch, fusion, _FakeTB([_FakeRule("only-model")]))
    fake = _FakeClient("direct")
    monkeypatch.setattr(fusion.srv, "use", lambda scenario: fake)

    assert fusion.handle(_req("q")) == "direct"
    assert len(fake.calls) == 1
    assert fake.calls[0][1]["model"] == "only-model"


def test_poll_panel_gathers_one_result_per_panel_entry(monkeypatch):
    fusion = _load("fusion_server")
    fake = _FakeClient("same-answer")
    monkeypatch.setattr(fusion.srv, "use", lambda scenario: fake)

    panel = ["a", "b"]
    results = fusion._poll_panel("q", panel)

    assert results == ["same-answer"] * len(panel)
    assert [c[1]["model"] for c in fake.calls] == panel


def test_fusion_skips_judge_when_panel_agrees(monkeypatch):
    fusion = _load("fusion_server")
    _bind_tb(monkeypatch, fusion, _FakeTB([_FakeRule("a"), _FakeRule("b")]))
    monkeypatch.setattr(fusion, "_poll_panel", lambda question, panel: ["42", "42"])

    def judge_should_not_be_called(scenario):
        raise AssertionError("judge must not be called when the panel agrees")

    monkeypatch.setattr(fusion.srv, "use", judge_should_not_be_called)

    assert fusion.handle(_req("what is 6*7?")) == "42"


def test_fusion_calls_judge_when_panel_disagrees(monkeypatch):
    fusion = _load("fusion_server")
    _bind_tb(monkeypatch, fusion, _FakeTB([_FakeRule("a"), _FakeRule("b")]))
    monkeypatch.setattr(fusion, "_poll_panel", lambda question, panel: ["A", "B"])
    judge = _FakeClient("SYNTHESIZED")
    monkeypatch.setattr(fusion.srv, "use", lambda scenario: judge)

    result = fusion.handle(_req("question"))

    assert result == "SYNTHESIZED"
    assert len(judge.calls) == 1
    judge_prompt = judge.calls[0][0]
    assert "A" in judge_prompt and "B" in judge_prompt
    assert "question" in judge_prompt


# -- router -----------------------------------------------------------------

def _router(monkeypatch, available, reply="answer"):
    router = _load("router_server")
    _bind_tb(monkeypatch, router, _FakeTB(available))
    fake = _FakeClient(reply)
    monkeypatch.setattr(router.srv, "use", lambda scenario: fake)
    return router, fake


def test_router_sends_short_prompts_to_a_cheap_rule(monkeypatch):
    router, fake = _router(
        monkeypatch, [_FakeRule("gpt-5"), _FakeRule("claude-haiku-4-5")]
    )
    router.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "claude-haiku-4-5"


def test_router_sends_long_prompts_to_a_strong_rule(monkeypatch):
    router, fake = _router(
        monkeypatch, [_FakeRule("claude-haiku-4-5"), _FakeRule("claude-opus-4-6")]
    )
    router.handle(_req("x" * (router.LONG_REQUEST_CHARS + 1)))
    assert fake.calls[0][1]["model"] == "claude-opus-4-6"


def test_router_recognises_code(monkeypatch):
    router, fake = _router(
        monkeypatch, [_FakeRule("claude-haiku-4-5"), _FakeRule("claude-sonnet-4-6")]
    )
    router.handle(_req("def f():\n    return 1"))
    assert fake.calls[0][1]["model"] == "claude-sonnet-4-6"


def test_router_skips_rules_this_box_does_not_have(monkeypatch):
    """Preferences are hints matched against real rules, not requirements.

    A box with only one model must still work rather than dispatching to a
    model id that does not exist.
    """
    router, fake = _router(monkeypatch, [_FakeRule("only-model")])
    router.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "only-model"


def test_router_ignores_inactive_rules(monkeypatch):
    router, fake = _router(
        monkeypatch, [_FakeRule("claude-haiku-4-5", active=False), _FakeRule("gpt-5")]
    )
    router.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "gpt-5"


def test_router_falls_back_to_auto_on_an_empty_box(monkeypatch):
    """No rules configured -> let tb's own smart routing decide."""
    router, fake = _router(monkeypatch, [])
    router.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "auto"


def test_router_reports_its_decision(monkeypatch):
    router, _ = _router(monkeypatch, [_FakeRule("gpt-5")], reply="the answer")
    out = router.handle(_req("hi"))
    assert "router:" in out and "gpt-5" in out and "the answer" in out


# -- quota router -----------------------------------------------------------

class _FakeUsage:
    """Stands in for a generated models.ProviderUsage."""

    def __init__(self, name, uuid, provider_type="codex", windows=()):
        self.provider_name = name
        self.provider_uuid = uuid
        self.provider_type = provider_type
        self.windows = list(windows)


class _FakeQuota:
    """Stands in for QuotaView, using the real headroom maths."""

    def __init__(self, usages):
        self._usages = list(usages)

    def of_type(self, provider_type):
        return [u for u in self._usages if u.provider_type == provider_type]

    def usable(self, provider_type, min_headroom=5.0):
        from tingly.helpers.quota import headroom_percent

        scored = []
        for u in self.of_type(provider_type):
            left = headroom_percent(u)
            if left is not None and left > min_headroom:
                scored.append((u, left))
        scored.sort(key=lambda pair: pair[1], reverse=True)
        return scored


def _window(used_percent, limit=100.0, **kw):
    from tingly._generated.models import UsageWindow

    return UsageWindow(
        description="", label="", limit=limit, type="", unit="",
        used=used_percent, used_percent=used_percent, **kw
    )


def _quota_router(monkeypatch, usages, rules):
    mod = _load("quota_router_server")
    _bind_tb(monkeypatch, mod, _FakeTB(rules, quota=_FakeQuota(usages)))
    fake = _FakeClient("answer")
    monkeypatch.setattr(mod.srv, "use", lambda scenario: fake)
    return mod, fake


def test_quota_router_picks_the_account_with_the_most_headroom(monkeypatch):
    mod, fake = _quota_router(
        monkeypatch,
        usages=[
            _FakeUsage("codex-personal", "p1", windows=[_window(88.0)]),  # 12% left
            _FakeUsage("codex-team", "p2", windows=[_window(39.0)]),      # 61% left
        ],
        rules=[
            _FakeRule("codex-a", provider="p1"),
            _FakeRule("codex-b", provider="p2"),
        ],
    )
    out = mod.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "codex-b"
    assert "codex-team" in out and "61% left" in out


def test_quota_router_skips_accounts_below_the_threshold(monkeypatch):
    mod, fake = _quota_router(
        monkeypatch,
        usages=[
            _FakeUsage("codex-work", "p1", windows=[_window(98.0)]),   # 2% left
            _FakeUsage("codex-team", "p2", windows=[_window(90.0)]),   # 10% left
        ],
        rules=[
            _FakeRule("codex-a", provider="p1"),
            _FakeRule("codex-b", provider="p2"),
        ],
    )
    mod.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "codex-b"


def test_quota_router_still_routes_when_everything_is_low(monkeypatch):
    """Running low is not being out — keep serving via the least exhausted."""
    mod, fake = _quota_router(
        monkeypatch,
        usages=[
            _FakeUsage("codex-work", "p1", windows=[_window(99.0)]),
            _FakeUsage("codex-team", "p2", windows=[_window(97.0)]),
        ],
        rules=[
            _FakeRule("codex-a", provider="p1"),
            _FakeRule("codex-b", provider="p2"),
        ],
    )
    out = mod.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "codex-b"
    assert "all accounts low" in out


def test_quota_router_ignores_other_provider_types(monkeypatch):
    mod, fake = _quota_router(
        monkeypatch,
        usages=[
            _FakeUsage("claude", "p1", provider_type="anthropic", windows=[_window(1.0)]),
            _FakeUsage("codex-team", "p2", windows=[_window(50.0)]),
        ],
        rules=[
            _FakeRule("claude-rule", provider="p1"),
            _FakeRule("codex-b", provider="p2"),
        ],
    )
    mod.handle(_req("hi"))
    assert fake.calls[0][1]["model"] == "codex-b"


def test_quota_router_explains_itself_when_no_rule_points_at_the_account(monkeypatch):
    mod, fake = _quota_router(
        monkeypatch,
        usages=[_FakeUsage("codex-team", "p2", windows=[_window(10.0)])],
        rules=[_FakeRule("something-else", provider="other-uuid")],
    )
    out = mod.handle(_req("hi"))
    assert fake.calls == []
    assert "no rule under openai" in out


def test_quota_router_explains_itself_when_nothing_reports_quota(monkeypatch):
    mod, fake = _quota_router(monkeypatch, usages=[], rules=[])
    out = mod.handle(_req("hi"))
    assert fake.calls == []
    assert "no usable codex rule" in out
