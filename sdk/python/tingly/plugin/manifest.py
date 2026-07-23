"""``tingly.toml`` plugin manifest — read/write.

The manifest is what tingly-box (Layer 2 supervisor) reads to install and run a
plugin, and what `tingly plugin register` uses to wire the plugin in as an
upstream provider. It is deliberately tiny:

    [plugin]
    name = "my-rag"
    model_id = "plugin/my-rag"
    version = "0.1.0"
    entrypoint = "rag_plugin:plugin"   # module:attr that yields a Plugin
    transport = "anthropic"            # anthropic (primary) | openai (secondary) — the
                                        # wire protocol tb should use to call this plugin;
                                        # the server always answers both regardless
    port = 8765
    description = "Answers from my private corpus"
"""

from __future__ import annotations

import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Optional

if sys.version_info >= (3, 11):
    import tomllib  # type: ignore
else:  # pragma: no cover - 3.9/3.10 fallback
    try:
        import tomli as tomllib  # type: ignore
    except ImportError:
        tomllib = None  # type: ignore

MANIFEST_NAME = "tingly.toml"


@dataclass
class Manifest:
    name: str
    model_id: str
    entrypoint: str
    version: str = "0.1.0"
    transport: str = "anthropic"
    port: int = 8765
    description: str = ""

    def to_toml(self) -> str:
        d = asdict(self)
        lines = ["[plugin]"]
        for key in ("name", "model_id", "version", "entrypoint", "transport", "description"):
            lines.append(f'{key} = "{_escape(str(d[key]))}"')
        lines.append(f"port = {int(d['port'])}")
        return "\n".join(lines) + "\n"

    def write(self, directory: Path) -> Path:
        path = Path(directory) / MANIFEST_NAME
        path.write_text(self.to_toml(), encoding="utf-8")
        return path


def load(path: Path) -> Manifest:
    """Load a manifest from a ``tingly.toml`` file or its containing directory."""
    p = Path(path)
    if p.is_dir():
        p = p / MANIFEST_NAME
    if tomllib is None:  # pragma: no cover
        raise RuntimeError(
            "reading tingly.toml needs Python 3.11+ or the `tomli` package"
        )
    with p.open("rb") as fh:
        data = tomllib.load(fh)
    plugin = data.get("plugin") or {}
    return Manifest(
        name=plugin["name"],
        model_id=plugin.get("model_id", f"plugin/{plugin['name']}"),
        entrypoint=plugin["entrypoint"],
        version=plugin.get("version", "0.1.0"),
        transport=plugin.get("transport", "anthropic"),
        port=int(plugin.get("port", 8765)),
        description=plugin.get("description", ""),
    )


def find(start: Optional[Path] = None) -> Optional[Manifest]:
    """Search upward from ``start`` (cwd by default) for a ``tingly.toml``."""
    cur = Path(start or Path.cwd()).resolve()
    for directory in [cur, *cur.parents]:
        candidate = directory / MANIFEST_NAME
        if candidate.exists():
            return load(candidate)
    return None


def _escape(s: str) -> str:
    return s.replace("\\", "\\\\").replace('"', '\\"')
