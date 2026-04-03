#!/usr/bin/env python3
"""
MCP stdio server that exposes two tools:
  - web_search: search via Serper API
  - web_fetch: fetch page content via Jina Reader

Environment variables:
  - SERPER_API_KEY   required for web_search
  - JINA_API_KEY     optional for web_fetch
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
    "name": "web_search",
    "description": "Search web pages with Serper and return top organic results.",
    "inputSchema": {
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "Search query."},
            "num_results": {
                "type": "integer",
                "minimum": 1,
                "maximum": 10,
                "default": 5,
                "description": "Max result count.",
            },
            "gl": {"type": "string", "description": "Country code, e.g. us."},
            "hl": {"type": "string", "description": "Language code, e.g. en."},
        },
        "required": ["query"],
    },
}

TOOL_WEB_FETCH = {
    "name": "web_fetch",
    "description": "Fetch and convert a URL to markdown-like text via Jina Reader.",
    "inputSchema": {
        "type": "object",
        "properties": {
            "url": {"type": "string", "description": "Target URL."},
            "query": {
                "type": "string",
                "description": "Optional focus question for local snippet extraction.",
            },
            "max_chars": {
                "type": "integer",
                "minimum": 256,
                "maximum": 100000,
                "default": 12000,
                "description": "Max output characters.",
            },
        },
        "required": ["url"],
    },
}


def _read_exact(n: int) -> bytes:
    out = bytearray()
    while len(out) < n:
        chunk = sys.stdin.buffer.read(n - len(out))
        if not chunk:
            raise EOFError("stdin closed")
        out.extend(chunk)
    return bytes(out)


def read_frame() -> Dict[str, Any]:
    content_length = None
    while True:
        line = sys.stdin.buffer.readline()
        if not line:
            raise EOFError("stdin closed")
        line = line.strip()
        if not line:
            break
        lower = line.lower()
        if lower.startswith(b"content-length:"):
            raw = line.split(b":", 1)[1].strip()
            content_length = int(raw.decode("utf-8"))
    if content_length is None or content_length <= 0:
        raise ValueError("missing content-length")
    payload = _read_exact(content_length)
    return json.loads(payload.decode("utf-8"))


def write_frame(obj: Dict[str, Any]) -> None:
    payload = json.dumps(obj, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    header = f"Content-Length: {len(payload)}\r\n\r\n".encode("utf-8")
    sys.stdout.buffer.write(header)
    sys.stdout.buffer.write(payload)
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
        insecure_ctx = ssl._create_unverified_context()
        return urllib.request.urlopen(req, timeout=timeout, context=insecure_ctx)


def _safe_int(v: Any, default: int) -> int:
    try:
        return int(v)
    except (TypeError, ValueError):
        return default


def tool_web_search(args: Dict[str, Any]) -> Dict[str, Any]:
    query = str(args.get("query", "")).strip()
    if not query:
        raise ValueError("query is required")

    api_key = os.getenv("SERPER_API_KEY", "").strip()
    if not api_key:
        raise ValueError("SERPER_API_KEY is not set")

    num_results = _safe_int(args.get("num_results"), 5)
    if num_results < 1:
        num_results = 1
    if num_results > 10:
        num_results = 10

    payload: Dict[str, Any] = {"q": query, "num": num_results}
    gl = str(args.get("gl", "")).strip()
    hl = str(args.get("hl", "")).strip()
    if gl:
        payload["gl"] = gl
    if hl:
        payload["hl"] = hl

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
        "tool": "web_search",
        "query": query,
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
    url = str(args.get("url", "")).strip()
    if not url:
        raise ValueError("url is required")
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme not in ("http", "https"):
        raise ValueError("url must be http/https")

    max_chars = _safe_int(args.get("max_chars"), 12000)
    if max_chars < 256:
        max_chars = 256
    if max_chars > 100000:
        max_chars = 100000

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

    query = str(args.get("query", "")).strip()
    snippets = _extract_snippets(content, query) if query else []

    structured = {
        "tool": "web_fetch",
        "url": url,
        "source": source,
        "query": query,
        "truncated": truncated,
        "content": content,
    }
    if snippets:
        structured["snippets"] = snippets

    return {
        "content": [{"type": "text", "text": json.dumps(structured, ensure_ascii=False)}],
        "structuredContent": structured,
    }


def handle_request(req: Dict[str, Any]) -> Dict[str, Any]:
    rid = req.get("id")
    method = req.get("method")
    params = req.get("params") or {}

    if method == "initialize":
        return ok(
            rid,
            {
                "protocolVersion": "2024-11-05",
                "capabilities": {"tools": {}},
                "serverInfo": {"name": "tingly-web-tools", "version": "0.1.0"},
            },
        )

    if method == "tools/list":
        return ok(rid, {"tools": [TOOL_WEB_SEARCH, TOOL_WEB_FETCH]})

    if method == "tools/call":
        name = str(params.get("name", "")).strip()
        arguments = params.get("arguments") or {}
        if not isinstance(arguments, dict):
            return err(rid, -32602, "arguments must be an object")
        try:
            if name == "web_search":
                result = tool_web_search(arguments)
            elif name == "web_fetch":
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

    return err(rid, -32601, f"method not found: {method}")


def main() -> int:
    while True:
        try:
            req = read_frame()
        except EOFError:
            return 0
        except Exception as e:
            write_frame(err(None, -32700, f"parse error: {e}"))
            continue
        write_frame(handle_request(req))


if __name__ == "__main__":
    sys.exit(main())
