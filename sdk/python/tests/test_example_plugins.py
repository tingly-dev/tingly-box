"""Tests for the critic/fusion showcase plugins (sdk/python/examples/).

These exercise handler logic only — plugin.use() is monkeypatched to a fake
client, so no real tb and no real model calls. The examples aren't part of
the installed `tingly` package, so they're loaded by file path, the same way
`tingly plugin run` loads a user's plugin script (see tingly/cli.py).
"""

import importlib.util
import sys
from pathlib import Path

from tingly.plugin.types import ChatRequest

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
    critic = _load("critic_plugin")
    fake = _FakeClient('{"verdict": "approve", "issues": [], "suggestion": "looks good"}')
    monkeypatch.setattr(critic.plugin, "use", lambda scenario: fake)

    result = critic.handle(_req("def f(): return 1/0", system="a python snippet"))

    assert result == "verdict: approve\nsuggestion: looks good"
    prompt, kwargs = fake.calls[0]
    assert "a python snippet" in prompt
    assert "def f(): return 1/0" in prompt
    assert kwargs["model"] == critic.CRITIC_MODEL


def test_critic_lists_issues(monkeypatch):
    critic = _load("critic_plugin")
    fake = _FakeClient('{"verdict": "revise", "issues": ["divides by zero"], "suggestion": "guard the denominator"}')
    monkeypatch.setattr(critic.plugin, "use", lambda scenario: fake)

    result = critic.handle(_req("def f(): return 1/0"))

    assert "verdict: revise" in result
    assert "- divides by zero" in result
    assert "suggestion: guard the denominator" in result


def test_critic_degrades_gracefully_on_non_json(monkeypatch):
    """A critic model that ignores the JSON contract must not crash the
    request — it should surface as a 'revise' verdict carrying the raw text."""
    critic = _load("critic_plugin")
    fake = _FakeClient("looks fine to me")
    monkeypatch.setattr(critic.plugin, "use", lambda scenario: fake)

    result = critic.handle(_req("some code"))

    assert "verdict: revise" in result
    assert "looks fine to me" in result


def test_critic_strips_markdown_code_fence(monkeypatch):
    critic = _load("critic_plugin")
    fake = _FakeClient('```json\n{"verdict": "approve", "issues": [], "suggestion": ""}\n```')
    monkeypatch.setattr(critic.plugin, "use", lambda scenario: fake)

    result = critic.handle(_req("some code"))

    assert result == "verdict: approve"


# -- fusion -------------------------------------------------------------

def test_poll_panel_gathers_one_result_per_panel_entry(monkeypatch):
    fusion = _load("fusion_plugin")
    fake = _FakeClient("same-answer")
    monkeypatch.setattr(fusion.plugin, "use", lambda scenario: fake)

    results = fusion._poll_panel("q")

    assert results == ["same-answer"] * len(fusion.PANEL)
    assert len(fake.calls) == len(fusion.PANEL)


def test_fusion_skips_judge_when_panel_agrees(monkeypatch):
    fusion = _load("fusion_plugin")
    monkeypatch.setattr(fusion, "_poll_panel", lambda question: ["42", "42"])

    def judge_should_not_be_called(scenario):
        raise AssertionError("judge must not be called when the panel agrees")

    monkeypatch.setattr(fusion.plugin, "use", judge_should_not_be_called)

    assert fusion.handle(_req("what is 6*7?")) == "42"


def test_fusion_calls_judge_when_panel_disagrees(monkeypatch):
    fusion = _load("fusion_plugin")
    monkeypatch.setattr(fusion, "_poll_panel", lambda question: ["A", "B"])
    judge = _FakeClient("SYNTHESIZED")
    monkeypatch.setattr(fusion.plugin, "use", lambda scenario: judge)

    result = fusion.handle(_req("question"))

    assert result == "SYNTHESIZED"
    assert len(judge.calls) == 1
    judge_prompt = judge.calls[0][0]
    assert "A" in judge_prompt and "B" in judge_prompt
    assert "question" in judge_prompt
