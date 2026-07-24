"""Transport URL-shaping tests, using a stubbed LLM SDK module.

These pin the contract with the gateway routes:
  - OpenAI SDK base_url = scenario_root + "/v1"  -> /tingly/<scn>/v1/chat/completions
  - Anthropic SDK base_url = scenario_root        -> /tingly/<scn>/v1/messages
"""

import sys
import types

from tingly.transports import anthropic_compat, openai_compat

ROOT = "http://tb.test:12580/tingly/experiment"


def test_openai_appends_v1(monkeypatch):
    captured = {}

    fake = types.ModuleType("openai")

    class FakeOpenAI:
        def __init__(self, base_url, api_key, timeout):
            captured["base_url"] = base_url
            captured["api_key"] = api_key

    fake.OpenAI = FakeOpenAI
    monkeypatch.setitem(sys.modules, "openai", fake)

    openai_compat.build_openai(ROOT, "tok", 10.0)
    assert captured["base_url"] == ROOT + "/v1"
    assert captured["api_key"] == "tok"


def test_anthropic_no_v1(monkeypatch):
    captured = {}

    fake = types.ModuleType("anthropic")

    class FakeAnthropic:
        def __init__(self, base_url, api_key, timeout):
            captured["base_url"] = base_url

    fake.Anthropic = FakeAnthropic
    monkeypatch.setitem(sys.modules, "anthropic", fake)

    anthropic_compat.build_anthropic(ROOT, "tok", 10.0)
    assert captured["base_url"] == ROOT
