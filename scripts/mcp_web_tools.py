#!/usr/bin/env python3
"""
MCP stdio server that exposes two tools:
  - mcp_web_search: search via Serper API
  - mcp_web_fetch: fetch page content via Jina Reader

Environment variables:
  - SERPER_API_KEY   required for mcp_web_search
  - JINA_API_KEY     optional for mcp_web_fetch
"""

from __future__ import annotations

import json
import os
import re
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional, Tuple


JSONRPC_VERSION = "2.0"

TOOL_WEB_SEARCH = {
    "name": "mcp_web_search",
    "description": "Search web pages with Serper and return top organic results.",
    "inputSchema": {
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "Search query."},
            "allowed_domains": {
                "type": "array",
                "items": {"type": "string"},
                "description": "Optional domain allow list.",
            },
            "blocked_domains": {
                "type": "array",
                "items": {"type": "string"},
                "description": "Optional domain block list.",
            },
        },
        "required": ["query"],
    },
}

TOOL_WEB_FETCH = {
    "name": "mcp_web_fetch",
    "description": "Fetch and convert a URL to markdown-like text via Jina Reader.",
    "inputSchema": {
        "type": "object",
        "properties": {
            "url": {"type": "string", "description": "Target URL."},
            "prompt": {
                "type": "string",
                "description": "Extraction instruction for content focus.",
            },
        },
        "required": ["url", "prompt"],
    },
}


def read_frame() -> Dict[str, Any]:
    """Read one NDJSON frame (go-sdk CommandTransport compatible)."""
    line = sys.stdin.buffer.readline()
    if not line:
        raise EOFError("stdin closed")
    line = line.strip()
    if not line:
        raise ValueError("empty frame")
    return json.loads(line.decode("utf-8"))


def write_frame(obj: Dict[str, Any]) -> None:
    """Write one NDJSON frame (go-sdk CommandTransport compatible)."""
    payload = json.dumps(obj, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    sys.stdout.buffer.write(payload)
    sys.stdout.buffer.write(b"\n")
    sys.stdout.buffer.flush()


def ok(rid: Any, result: Dict[str, Any]) -> Dict[str, Any]:
    return {"jsonrpc": JSONRPC_VERSION, "id": rid, "result": result}


def err(rid: Any, code: int, message: str) -> Dict[str, Any]:
    return {"jsonrpc": JSONRPC_VERSION, "id": rid, "error": {"code": code, "message": message}}


def _http_json_post(url: str, headers: Dict[str, str], body: Dict[str, Any]) -> Dict[str, Any]:
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, method="POST")
    for k, v in headers.items():
        req.add_header(k, v)
    with _urlopen(req, timeout=30) as resp:
        raw = resp.read().decode("utf-8", errors="replace")
    return json.loads(raw)


def _http_text_post(url: str, headers: Dict[str, str], body: Dict[str, Any]) -> str:
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, method="POST")
    for k, v in headers.items():
        req.add_header(k, v)
    with _urlopen(req, timeout=60) as resp:
        return resp.read().decode("utf-8", errors="replace")


def _urlopen(req: urllib.request.Request, timeout: int):
    try:
        return urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.URLError as e:
        msg = str(getattr(e, "reason", e))
        if "CERTIFICATE_VERIFY_FAILED" not in msg:
            raise
        # Fallback for hosts where local CA bundle is unavailable.
        # NOTE: SSL verification is disabled. This is insecure in production.
        import sys
        print("WARNING: SSL verification disabled due to CERTIFICATE_VERIFY_FAILED. "
              "Set the system CA bundle or install certificates to restore secure HTTPS.", file=sys.stderr)
        insecure_ctx = ssl._create_unverified_context()
        return urllib.request.urlopen(req, timeout=timeout, context=insecure_ctx)


def _safe_int(v: Any, default: int) -> int:
    try:
        return int(v)
    except (TypeError, ValueError):
        return default


def _require_allowed_keys(args: Dict[str, Any], allowed: set[str]) -> None:
    unknown = [k for k in args.keys() if k not in allowed]
    if unknown:
        raise ValueError(f"unsupported argument(s): {', '.join(sorted(unknown))}")


def _as_string_list(v: Any, key: str) -> List[str]:
    if v is None:
        return []
    if not isinstance(v, list):
        raise ValueError(f"{key} must be an array of strings")
    out: List[str] = []
    for item in v:
        if not isinstance(item, str):
            raise ValueError(f"{key} must be an array of strings")
        s = item.strip()
        if s:
            out.append(s)
    return out


def tool_web_search(args: Dict[str, Any]) -> Dict[str, Any]:
    _require_allowed_keys(args, {"query", "allowed_domains", "blocked_domains"})

    query = str(args.get("query", "")).strip()
    if not query:
        raise ValueError("query is required")

    api_key = os.getenv("SERPER_API_KEY", "").strip()
    if not api_key:
        raise ValueError("SERPER_API_KEY is not set")

    allowed_domains = _as_string_list(args.get("allowed_domains"), "allowed_domains")
    blocked_domains = _as_string_list(args.get("blocked_domains"), "blocked_domains")

    final_query = query
    if allowed_domains:
        allow_expr = " OR ".join(f"site:{d}" for d in allowed_domains)
        final_query = f"{final_query} ({allow_expr})"
    for d in blocked_domains:
        final_query = f"{final_query} -site:{d}"

    payload: Dict[str, Any] = {"q": final_query, "num": 5}

    resp = _http_json_post(
        "https://google.serper.dev/search",
        {"X-API-KEY": api_key, "Content-Type": "application/json"},
        payload,
    )
    organic = resp.get("organic", []) or []
    results: List[Dict[str, Any]] = []
    for item in organic:
        results.append(
            {
                "title": item.get("title", ""),
                "url": item.get("link") or item.get("url", ""),
                "snippet": item.get("snippet", ""),
            }
        )

    structured = {
        "tool": "mcp_web_search",
        "query": query,
        "effective_query": final_query,
        "allowed_domains": allowed_domains,
        "blocked_domains": blocked_domains,
        "result_count": len(results),
        "results": results,
    }
    return {
        "content": [{"type": "text", "text": json.dumps(structured, ensure_ascii=False)}],
        "structuredContent": structured,
    }


def _extract_snippets(content: str, question: str, max_lines: int = 8) -> List[str]:
    q = question.strip().lower()
    if not q:
        return []
    words = [w for w in re.split(r"[\s,.;:!?()]+", q) if len(w) >= 2]
    if not words:
        return []
    hits: List[str] = []
    for line in content.splitlines():
        line_s = line.strip()
        if not line_s:
            continue
        low = line_s.lower()
        if any(w in low for w in words):
            hits.append(line_s)
            if len(hits) >= max_lines:
                break
    return hits


def _strip_html(html: str) -> str:
    text = re.sub(r"(?is)<script[^>]*>.*?</script>", " ", html)
    text = re.sub(r"(?is)<style[^>]*>.*?</style>", " ", text)
    text = re.sub(r"(?s)<[^>]+>", " ", text)
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def _fetch_direct(url: str) -> str:
    req = urllib.request.Request(
        url,
        method="GET",
        headers={
            "User-Agent": "tingly-box-mcp-web-tools/0.1",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        },
    )
    with _urlopen(req, timeout=30) as resp:
        raw = resp.read().decode("utf-8", errors="replace")
        ctype = (resp.headers.get("Content-Type") or "").lower()
    if "text/html" in ctype or "<html" in raw.lower():
        return _strip_html(raw)
    return raw.strip()


def tool_web_fetch(args: Dict[str, Any]) -> Dict[str, Any]:
    _require_allowed_keys(args, {"url", "prompt"})

    url = str(args.get("url", "")).strip()
    if not url:
        raise ValueError("url is required")
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme not in ("http", "https"):
        raise ValueError("url must be http/https")

    prompt = str(args.get("prompt", "")).strip()
    if not prompt:
        raise ValueError("prompt is required")

    max_chars = 12000

    headers = {
        "Content-Type": "application/json",
        "X-Engine": "direct",
        "X-Retain-Images": "none",
        "X-Return-Format": "markdown",
        "X-Timeout": "60",
    }
    jina_key = os.getenv("JINA_API_KEY", "").strip()
    if jina_key:
        headers["Authorization"] = f"Bearer {jina_key}"

    source = "jina"
    try:
        content = _http_text_post("https://r.jina.ai/", headers, {"url": url})
    except Exception:
        content = _fetch_direct(url)
        source = "direct"
    truncated = False
    if len(content) > max_chars:
        content = content[:max_chars]
        truncated = True

    snippets = _extract_snippets(content, prompt)

    structured = {
        "tool": "mcp_web_fetch",
        "url": url,
        "source": source,
        "prompt": prompt,
        "truncated": truncated,
        "content": content,
    }
    if snippets:
        structured["snippets"] = snippets

    return {
        "content": [{"type": "text", "text": json.dumps(structured, ensure_ascii=False)}],
        "structuredContent": structured,
    }


def handle_request(req: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    rid = req.get("id")
    method = req.get("method")
    params = req.get("params") or {}

    # Notifications (no id) are one-way; don't send a response.
    is_notification = rid is None

    if method == "ping":
        if is_notification:
            return None
        return ok(rid, {})

    if method == "initialize":
        result = {
            "protocolVersion": "2024-11-05",
            "capabilities": {"tools": {}},
            "serverInfo": {"name": "tingly-web-tools", "version": "0.1.0"},
        }
        if is_notification:
            return None
        return ok(rid, result)

    if method == "tools/list":
        if is_notification:
            return None
        return ok(rid, {"tools": [TOOL_WEB_SEARCH, TOOL_WEB_FETCH]})

    if method == "tools/call":
        if is_notification:
            return None
        name = str(params.get("name", "")).strip()
        arguments = params.get("arguments") or {}
        if not isinstance(arguments, dict):
            return err(rid, -32602, "arguments must be an object")
        try:
            if name == "mcp_web_search":
                result = tool_web_search(arguments)
            elif name == "mcp_web_fetch":
                result = tool_web_fetch(arguments)
            else:
                return err(rid, -32601, f"tool not found: {name}")
            return ok(rid, result)
        except urllib.error.HTTPError as e:
            return err(rid, -32001, f"http error {e.code}: {e.reason}")
        except urllib.error.URLError as e:
            return err(rid, -32002, f"network error: {e.reason}")
        except ValueError as e:
            return err(rid, -32602, str(e))
        except Exception as e:
            return err(rid, -32000, f"tool execution failed: {e}")

    if method == "resources/list":
        if is_notification:
            return None
        return ok(rid, {"resources": []})

    if method == "prompts/list":
        if is_notification:
            return None
        return ok(rid, {"prompts": []})

    if method == "logging/setLevel":
        if is_notification:
            return None
        return ok(rid, {})

    if is_notification:
        return None  # Unknown notification; silently ignore.

    # Be permissive for unknown calls to keep compatibility with evolving MCP clients.
    return ok(rid, {})


def main() -> int:
    while True:
        try:
            req = read_frame()
        except EOFError:
            return 0
        except Exception as e:
            write_frame(err(None, -32700, f"parse error: {e}"))
            continue
        resp = handle_request(req)
        if resp is not None:
            write_frame(resp)


if __name__ == "__main__":
    sys.exit(main())
