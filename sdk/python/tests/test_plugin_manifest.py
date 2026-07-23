"""Manifest round-trip + discovery tests."""

from pathlib import Path

from tingly import Plugin
from tingly.plugin import manifest as m


def test_manifest_roundtrip(tmp_path: Path):
    man = m.Manifest(
        name="my-rag",
        model_id="plugin/my-rag",
        entrypoint="rag_plugin:plugin",
        description='has "quotes" and \\ slash',
        port=9001,
    )
    man.write(tmp_path)
    loaded = m.load(tmp_path)
    assert loaded.name == "my-rag"
    assert loaded.model_id == "plugin/my-rag"
    assert loaded.entrypoint == "rag_plugin:plugin"
    assert loaded.port == 9001
    assert loaded.description == 'has "quotes" and \\ slash'


def test_manifest_find_walks_up(tmp_path: Path):
    m.Manifest(name="p", model_id="plugin/p", entrypoint="p:plugin").write(tmp_path)
    nested = tmp_path / "a" / "b"
    nested.mkdir(parents=True)
    found = m.find(nested)
    assert found is not None
    assert found.name == "p"


def test_plugin_builds_manifest():
    plugin = Plugin(name="my-rag", description="d")
    man = plugin.manifest(entrypoint="rag_plugin:plugin", port=8080)
    assert man.model_id == "plugin/my-rag"
    assert man.entrypoint == "rag_plugin:plugin"
    assert man.port == 8080
    assert man.transport == "anthropic"


def test_plugin_manifest_transport_follows_api_style_override():
    plugin = Plugin(name="my-rag", api_style="openai")
    man = plugin.manifest(entrypoint="rag_plugin:plugin")
    assert man.transport == "openai"

    # an explicit transport= still wins over api_style
    man2 = plugin.manifest(entrypoint="rag_plugin:plugin", transport="anthropic")
    assert man2.transport == "anthropic"


def test_plugin_use_caches_per_scenario(monkeypatch):
    import tingly.client as client_mod
    from tingly import Plugin

    calls = []

    def fake_connect(scenario, name):
        calls.append((scenario, name))
        return object()

    monkeypatch.setattr(client_mod, "connect", fake_connect)

    plugin = Plugin(name="p", scenario="experiment")
    a1 = plugin.llm
    a2 = plugin.llm                 # default scenario, cached
    b1 = plugin.use("claude_code")  # different rule-set
    assert a1 is a2
    assert b1 is not a1
    assert calls == [("experiment", "plugin:p"), ("claude_code", "plugin:p")]
