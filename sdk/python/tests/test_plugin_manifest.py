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
    assert man.transport == "openai"
