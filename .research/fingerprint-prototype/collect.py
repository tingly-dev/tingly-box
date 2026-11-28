#!/usr/bin/env python3
"""Draft model-fingerprint collector (stdlib only, sequential, direct-to-upstream).

This is a research prototype, not wired into tingly-box. It deliberately uses
raw HTTP instead of the Go SDKs so it can capture:
  - upstream response headers (allowlisted / pattern-classified)
  - exact response bytes and body hashes
  - per-SSE-chunk arrival times, TTFT, chunk count
  - token counts / usage / logprobs without SDK re-serialization

It never reads ~/.tingly-box, never writes a database, and never sends a key
anywhere except the --base-url you give it. The token comes from an env var.

Usage:
  TB_FP_TOKEN=... python3 collect.py --style openai \
      --base-url https://api.openai.com/v1 --model gpt-5.1 --out fp.json
  TB_FP_TOKEN=... python3 collect.py --style anthropic \
      --base-url https://api.anthropic.com --model claude-opus-4-5 --out fp.json
"""

import argparse
import hashlib
import http.client
import json
import math
import os
import re
import statistics
import sys
import time
import unicodedata
from pathlib import Path
from urllib.parse import urlsplit

SCHEMA_VERSION = "model-fingerprint/v0.1"
TOOL_VERSION = "0.1.0"

# Header names we keep. Values are either kept verbatim ("value") or reduced to
# a coarse pattern class ("pattern"). Everything else keeps only its name.
HEADER_ALLOWLIST = {
    "content-type": "value",
    "server": "value",
    "via": "value",
    "transfer-encoding": "value",
    "connection": "value",
    "cf-ray": "pattern",
    "date": "pattern",
    "x-request-id": "pattern",
    "request-id": "pattern",
    "openai-version": "value",
    "openai-processing-ms": "value",
    "openai-organization": "pattern",
    "anthropic-ratelimit-requests-limit": "value",
    "anthropic-ratelimit-tokens-limit": "value",
    "x-ratelimit-limit-requests": "value",
    "x-ratelimit-remaining-requests": "value",
    "x-envoy-upstream-service-time": "value",
    "retry-after": "value",
    "set-cookie": "ignore",
    "authorization": "ignore",
    "x-api-key": "ignore",
    "cookie": "ignore",
}

UUID_RE = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
HEX_RE = re.compile(r"^[0-9a-fA-F]+$")


def canon(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def sha256_hex(data):
    if isinstance(data, str):
        data = data.encode("utf-8")
    return "sha256:" + hashlib.sha256(data).hexdigest()


def norm_text(s):
    if s is None:
        return ""
    return unicodedata.normalize("NFC", s).strip()


def pattern_class(value):
    v = value.strip()
    if UUID_RE.match(v):
        return "uuid"
    if len(v) > 8 and HEX_RE.match(v):
        return "hex:%d" % ((len(v) // 8) * 8)
    if v.startswith("req_"):
        return "req_<hex>"
    if re.match(r"^\d{4}-\d{2}-\d{2}", v):
        return "date"
    return "present"


def summarize_headers(items):
    values, names = {}, []
    for name, value in items:
        lname = name.lower()
        if lname not in names:
            names.append(lname)
        mode = HEADER_ALLOWLIST.get(lname)
        if mode == "ignore" or mode is None:
            continue
        if mode == "value":
            values[lname] = {"value": value[:200]}
        else:
            values[lname] = {"pattern": pattern_class(value)}
    return {"names": sorted(names), "allowlist": values}


def join_url(base, path):
    b = base.rstrip("/")
    if b.endswith("/v1") and path.startswith("/v1/"):
        return b + path[3:]
    if path.startswith("/"):
        return b + path
    return b + "/" + path


def _first_content(style, obj):
    """Return (text, tool_name, tool_args_raw, finish_reason) from a decoded body."""
    text, tool_name, tool_args, finish = "", None, None, None
    if style == "openai":
        choices = obj.get("choices") or []
        if choices:
            ch = choices[0]
            finish = ch.get("finish_reason")
            msg = ch.get("message") or {}
            content = msg.get("content")
            if isinstance(content, str):
                text = content
            for tc in msg.get("tool_calls") or []:
                fn = tc.get("function") or {}
                tool_name = fn.get("name")
                tool_args = fn.get("arguments")
                break
    else:
        finish = obj.get("stop_reason")
        for block in obj.get("content") or []:
            if block.get("type") == "text":
                text += block.get("text") or ""
            elif block.get("type") == "tool_use":
                tool_name = block.get("name")
                tool_args = json.dumps(block.get("input"), ensure_ascii=False, sort_keys=True)
    return text, tool_name, tool_args, finish


def normalize_usage(style, u):
    if not isinstance(u, dict):
        return None
    if style == "openai":
        return {
            "input_tokens": u.get("prompt_tokens"),
            "output_tokens": u.get("completion_tokens"),
            "total_tokens": u.get("total_tokens"),
            "prompt_details": u.get("prompt_tokens_details"),
            "completion_details": u.get("completion_tokens_details"),
        }
    return {
        "input_tokens": u.get("input_tokens"),
        "output_tokens": u.get("output_tokens"),
        "cache_read_input_tokens": u.get("cache_read_input_tokens"),
        "cache_creation_input_tokens": u.get("cache_creation_input_tokens"),
    }


def http_call(url, method, headers, body, timeout, stream=False):
    """One direct HTTP call. Returns a dict with status/headers/body/timing/events."""
    u = urlsplit(url)
    conn_cls = http.client.HTTPSConnection if u.scheme == "https" else http.client.HTTPConnection
    conn = conn_cls(u.hostname, u.port, timeout=timeout)
    path = u.path or "/"
    if u.query:
        path += "?" + u.query
    t0 = time.monotonic()
    try:
        conn.request(method, path, body=body, headers=headers)
        resp = conn.getresponse()
        status = resp.status
        hdrs = summarize_headers(resp.getheaders())
        if not stream:
            raw = resp.read()
            elapsed = (time.monotonic() - t0) * 1000
            return {"status": status, "headers": hdrs, "raw": raw, "total_ms": round(elapsed, 1),
                    "events": [], "chunk_count": 0, "ttft_ms": None}
        events, ttft = [], None
        current_event = None
        while True:
            line = resp.readline()
            if not line:
                break
            now = (time.monotonic() - t0) * 1000
            s = line.decode("utf-8", "replace").rstrip("\r\n")
            if s.startswith("event:"):
                current_event = s[6:].strip()
            elif s.startswith("data:"):
                payload = s[5:].strip()
                if payload == "[DONE]":
                    events.append({"t_ms": round(now, 1), "event": "done", "data": None})
                    current_event = None
                    continue
                try:
                    data = json.loads(payload)
                except ValueError:
                    data = None
                events.append({"t_ms": round(now, 1), "event": current_event or "data",
                               "data": data, **({"text": payload[:200]} if data is None else {})})
                current_event = None
        elapsed = (time.monotonic() - t0) * 1000
        # TTFT: first delta that carries content or a tool call.
        for ev in events:
            d = ev.get("data")
            if not isinstance(d, dict):
                continue
            got = False
            if d.get("choices"):
                delta = (d["choices"][0] or {}).get("delta") or {}
                got = bool(delta.get("content")) or bool(delta.get("tool_calls"))
            elif d.get("type") == "content_block_delta":
                delta = d.get("delta") or {}
                got = bool(delta.get("text")) or delta.get("type") == "input_json_delta"
            if got:
                ttft = ev["t_ms"]
                break
        return {"status": status, "headers": hdrs, "raw": None, "total_ms": round(elapsed, 1),
                "events": events, "chunk_count": len(events), "ttft_ms": ttft}
    finally:
        conn.close()


def headers_for(style, token):
    if style == "openai":
        return {"Authorization": "Bearer " + token, "Content-Type": "application/json"}
    return {"x-api-key": token, "anthropic-version": "2023-06-01", "Content-Type": "application/json"}


def chat_body(style, model, prompt, max_tokens, extra=None):
    if style == "openai":
        body = {"model": model, "messages": [{"role": "user", "content": prompt}],
                "temperature": 0, "max_tokens": max_tokens}
    else:
        body = {"model": model, "max_tokens": max_tokens, "temperature": 0,
                "messages": [{"role": "user", "content": prompt}]}
    if extra:
        body.update(extra)
    return body


def tool_body(style, model, probe):
    t = probe["tool"]
    body = chat_body(style, model, probe["prompt"], probe.get("max_tokens", 150))
    if style == "openai":
        body["tools"] = [{"type": "function", "function": {
            "name": t["name"], "description": t["description"], "parameters": t["parameters"]}}]
        body["tool_choice"] = "auto"
    else:
        body["tools"] = [{"name": t["name"], "description": t["description"],
                          "input_schema": t["parameters"]}]
        body["tool_choice"] = {"type": "auto"}
    return body


def stop_body(style, model, probe):
    key = "stop" if style == "openai" else "stop_sequences"
    return chat_body(style, model, probe["prompt"], probe.get("max_tokens", 40),
                     {key: probe.get("stop") or []})


def logprobs_body(style, model, probe):
    if style != "openai":
        raise RuntimeError("logprobs probes are OpenAI Chat only in this prototype")
    return chat_body(style, model, probe["prompt"], probe.get("max_tokens", 4), {
        "logprobs": True, "top_logprobs": probe.get("top_logprobs", 5)})


def stream_extract(style, events):
    text, tool_name, tool_args, finish, usage = "", None, "", None, None
    arg_parts = {}
    for ev in events:
        d = ev.get("data")
        if not isinstance(d, dict):
            continue
        if style == "openai":
            if isinstance(d.get("usage"), dict):
                usage = normalize_usage(style, d["usage"])
            for ch in d.get("choices") or []:
                delta = ch.get("delta") or {}
                if delta.get("content"):
                    text += delta["content"]
                for tc in delta.get("tool_calls") or []:
                    idx = tc.get("index", 0)
                    fn = tc.get("function") or {}
                    if fn.get("name"):
                        tool_name = fn["name"]
                    if fn.get("arguments"):
                        arg_parts[idx] = arg_parts.get(idx, "") + fn["arguments"]
                if ch.get("finish_reason"):
                    finish = ch["finish_reason"]
        else:
            typ = d.get("type")
            if typ == "message_start":
                usage = normalize_usage(style, ((d.get("message") or {}).get("usage") or {}))
            elif typ == "content_block_start":
                block = d.get("content_block") or {}
                if block.get("type") == "tool_use":
                    tool_name = block.get("name")
                    if block.get("input"):
                        tool_args = json.dumps(block["input"], ensure_ascii=False, sort_keys=True)
            elif typ == "content_block_delta":
                delta = d.get("delta") or {}
                if delta.get("type") == "text_delta" and delta.get("text"):
                    text += delta["text"]
                if delta.get("type") == "input_json_delta" and delta.get("partial_json"):
                    tool_args += delta["partial_json"]
            elif typ == "message_delta":
                delta = d.get("delta") or {}
                if delta.get("stop_reason"):
                    finish = delta["stop_reason"]
                if isinstance(d.get("usage"), dict):
                    merged = dict(usage or {})
                    merged.update(normalize_usage(style, d["usage"]) or {})
                    usage = merged
    if arg_parts:
        tool_args = "".join(arg_parts[k] for k in sorted(arg_parts))
    return text, tool_name, tool_args, finish, usage


def decode_response(style, res):
    """Return (text, tool_name, tool_args, finish, usage, obj_or_none)."""
    if res.get("events"):
        text, name, args, finish, usage = stream_extract(style, res["events"])
        return text, name, args, finish, usage, None
    try:
        obj = json.loads(res["raw"].decode("utf-8", "replace"))
    except (ValueError, AttributeError):
        return "", None, None, None, None, None
    text, name, args, finish = _first_content(style, obj)
    usage = normalize_usage(style, obj.get("usage"))
    return text, name, args, finish, usage, obj


def classify_id(value):
    if not value:
        return None
    if value.startswith("chatcmpl-"):
        return "chatcmpl-<token>"
    if value.startswith("msg_"):
        return "msg_<token>"
    if value.startswith("resp_"):
        return "resp_<token>"
    if UUID_RE.match(value):
        return "uuid"
    return "other:len%d" % ((len(value) // 8) * 8)


def make_observation(seq, res, style, keep_raw, blob_dir, variant=None):
    text, name, args, finish, usage, obj = decode_response(style, res)
    obs = {
        "seq": seq,
        "status": res["status"],
        "output_text": norm_text(text),
        "output_sha256": sha256_hex(norm_text(text)),
        "tool_name": name,
        "tool_args": args,
        "finish_reason": finish,
        "usage": usage,
        "ttft_ms": res.get("ttft_ms"),
        "total_ms": res.get("total_ms"),
        "chunk_count": res.get("chunk_count"),
        "logprobs_digest": None,
        "raw_ref": None,
    }
    if variant:
        obs["variant"] = variant
    if res["status"] >= 400:
        body = res.get("raw") or b""
        obs["error_preview"] = body.decode("utf-8", "replace")[:400]
    if keep_raw:
        blob_dir.mkdir(parents=True, exist_ok=True)
        payload = {"headers": res.get("headers"), "body": None, "events": None}
        if res.get("raw"):
            try:
                payload["body"] = json.loads(res["raw"].decode("utf-8", "replace"))
            except ValueError:
                payload["body"] = res["raw"].decode("utf-8", "replace")
        if res.get("events"):
            payload["events"] = res["events"]
        digest = sha256_hex(canon(payload)).split(":", 1)[1]
        path = blob_dir / (digest + ".json")
        path.write_text(json.dumps(payload, ensure_ascii=False, indent=1), encoding="utf-8")
        obs["raw_ref"] = str(path)
    if obj:
        obs["reported_model"] = obj.get("model")
        obs["system_fingerprint"] = obj.get("system_fingerprint")
        obs["service_tier"] = obj.get("service_tier")
        obs["id_pattern"] = classify_id(obj.get("id"))
    return obs


def summarize_logprobs(obj):
    try:
        content = obj["choices"][0]["logprobs"]["content"]
    except (KeyError, IndexError, TypeError):
        return None
    toks, top1, margins, entropies = [], [], [], []
    for item in content or []:
        toks.append(item.get("token"))
        lp = item.get("logprob")
        tops = item.get("top_logprobs") or []
        if isinstance(lp, (int, float)):
            top1.append(math.exp(lp))
        if len(tops) >= 2:
            margins.append(tops[0]["logprob"] - tops[1]["logprob"])
        if tops:
            entropies.append(sum(-math.exp(t["logprob"]) * t["logprob"] for t in tops))
    digest = sha256_hex(canon(content))
    return {
        "tokens": toks,
        "top1_prob_mean": round(statistics.fmean(top1), 6) if top1 else None,
        "margin_mean": round(statistics.fmean(margins), 6) if margins else None,
        "entropy_mean": round(statistics.fmean(entropies), 6) if entropies else None,
        "digest": digest,
    }


def invoke(style, base, token, model, body, timeout, stream=False):
    if stream:
        body = dict(body)
        body["stream"] = True
        if style == "openai":
            opts = dict(body.get("stream_options") or {})
            opts["include_usage"] = True
            body["stream_options"] = opts
    url = join_url(base, "/v1/chat/completions" if style == "openai" else "/v1/messages")
    raw = json.dumps(body, ensure_ascii=False).encode("utf-8")
    return http_call(url, "POST", headers_for(style, token), raw, timeout, stream=stream)


def models_summary(raw):
    try:
        obj = json.loads(raw.decode("utf-8", "replace"))
    except (ValueError, AttributeError):
        return {"parse_error": True}
    data = obj.get("data") or obj.get("models") or []
    ids = [m.get("id") for m in data if isinstance(m, dict)]
    created = [m.get("created") for m in data if isinstance(m, dict) and isinstance(m.get("created"), int)]
    owned = sorted({m.get("owned_by") for m in data if isinstance(m, dict) and m.get("owned_by")})
    return {
        "count": len(ids),
        "id_patterns": sorted({re.sub(r"[0-9]+", "<n>", i or "") for i in ids})[:20],
        "created_min": min(created) if created else None,
        "created_max": max(created) if created else None,
        "owned_by": owned,
    }


def json_keys(raw):
    try:
        return sorted(json.loads(raw).keys())
    except (ValueError, AttributeError, TypeError):
        return []


def run_t0(style, base, token, model, probes, timeout, keep_raw, blob_dir):
    spec = probes["tier0"]
    reqs = []
    models_res = http_call(join_url(base, spec["models_path"]), "GET", headers_for(style, token),
                           None, timeout)
    reqs.append({
        "request_id": "t0.models", "method": "GET", "path": spec["models_path"],
        "status": models_res["status"], "elapsed_ms": models_res["total_ms"],
        "headers": models_res["headers"],
        "body_sha256": sha256_hex(models_res["raw"] or b""),
        "body_summary": models_summary(models_res["raw"] or b"") if models_res["status"] < 400 else None,
        "raw_ref": None,
    })

    # Minimal non-stream response: schema, ids, usage keys, finish_reason.
    mini = chat_body(style, model, "hi", 1)
    ns = invoke(style, base, token, model, mini, timeout, stream=False)
    ns_obs = make_observation(1, ns, style, keep_raw, blob_dir)
    reqs.append({
        "request_id": "t0.min_nonstream", "method": "POST",
        "path": spec["chat_path"][style], "status": ns["status"],
        "elapsed_ms": ns["total_ms"], "headers": ns["headers"],
        "body_sha256": sha256_hex(ns["raw"] or b""),
        "body_summary": {
            "top_keys": json_keys(ns["raw"]) if ns["status"] < 400 else [],
            "usage_keys": sorted((ns_obs["usage"] or {}).keys()),
            "finish_reason": ns_obs["finish_reason"],
            "id_pattern": ns_obs.get("id_pattern"),
            "reported_model": ns_obs.get("reported_model"),
            "system_fingerprint_present": bool(ns_obs.get("system_fingerprint")),
            "service_tier": ns_obs.get("service_tier"),
        },
        "raw_ref": ns_obs.get("raw_ref"),
    })

    # Same request, streamed: SSE shape and timing.
    st = invoke(style, base, token, model, mini, timeout, stream=True)
    st_obs = make_observation(1, st, style, keep_raw, blob_dir)
    reqs.append({
        "request_id": "t0.min_stream", "method": "POST",
        "path": spec["chat_path"][style], "status": st["status"],
        "elapsed_ms": st["total_ms"], "headers": st["headers"],
        "body_sha256": sha256_hex(canon([e.get("data") for e in st.get("events") or []])),
        "body_summary": {
            "chunk_count": st.get("chunk_count"),
            "event_names": sorted({e.get("event") for e in st.get("events") or [] if e.get("event")}),
            "ttft_ms": st.get("ttft_ms"),
            "content": st_obs["output_text"],
            "finish_reason": st_obs["finish_reason"],
        },
        "raw_ref": st_obs.get("raw_ref"),
    })

    # Unsupported / extra parameter: error envelope or silent acceptance.
    extra = spec["unsupported_param"][style]
    un = invoke(style, base, token, model, chat_body(style, model, "hi", 1, extra), timeout)
    reqs.append({
        "request_id": "t0.unsupported_param", "method": "POST",
        "path": spec["chat_path"][style], "status": un["status"],
        "elapsed_ms": un["total_ms"], "headers": un["headers"],
        "body_sha256": sha256_hex(un["raw"] or b""),
        "body_summary": {
            "accepted": un["status"] < 400,
            "error_preview": (un["raw"] or b"").decode("utf-8", "replace")[:400] if un["status"] >= 400 else None,
        },
        "raw_ref": None,
    })

    # Anthropic-shaped tokenizer endpoint, when available.
    if style == "anthropic":
        body = {"model": model, "messages": [{"role": "user", "content": "hello world"}]}
        raw = json.dumps(body, ensure_ascii=False).encode("utf-8")
        ct = http_call(join_url(base, spec["count_tokens_path"]), "POST",
                       headers_for(style, token), raw, timeout)
        out = None
        if ct["status"] < 400 and ct["raw"]:
            try:
                out = json.loads(ct["raw"]).get("input_tokens")
            except ValueError:
                pass
        reqs.append({
            "request_id": "t0.count_tokens", "method": "POST", "path": spec["count_tokens_path"],
            "status": ct["status"], "elapsed_ms": ct["total_ms"], "headers": ct["headers"],
            "body_sha256": sha256_hex(ct["raw"] or b""),
            "body_summary": {"input_tokens": out},
            "raw_ref": None,
        })

    nonstream = next(r for r in reqs if r["request_id"] == "t0.min_nonstream")
    stream = next(r for r in reqs if r["request_id"] == "t0.min_stream")
    signature = {
        "serving_path_sig": sha256_hex(canon({
            "headers": nonstream["headers"],
            "status": nonstream["status"],
            "id_pattern": nonstream["body_summary"]["id_pattern"],
            "top_keys": nonstream["body_summary"]["top_keys"],
            "usage_keys": nonstream["body_summary"]["usage_keys"],
            "finish_reason": nonstream["body_summary"]["finish_reason"],
            "stream_event_names": stream["body_summary"]["event_names"],
        })),
        "catalog_sig": sha256_hex(canon(
            next(r for r in reqs if r["request_id"] == "t0.models")["body_summary"])),
        "schema_sig": sha256_hex(canon(nonstream["body_summary"])),
        "finish_reason_vocab": [nonstream["body_summary"]["finish_reason"]] if nonstream["body_summary"]["finish_reason"] else [],
        "usage_keys": nonstream["body_summary"]["usage_keys"],
        "id_patterns": {"primary": nonstream["body_summary"]["id_pattern"]},
        "stream": stream["body_summary"],
    }
    return {"requests": reqs, "signature": signature}


def run_token_delta(style, base, token, model, probe, timeout, repeats):
    entries, counts = [], {"base": [], "plus": []}
    for variant, text in (("base", probe["base"]), ("plus", probe["base"] + probe["add"])):
        for i in range(repeats):
            if style == "anthropic":
                body = {"model": model, "messages": [{"role": "user", "content": text}]}
                raw = json.dumps(body, ensure_ascii=False).encode("utf-8")
                res = http_call(join_url(base, "/v1/messages/count_tokens"), "POST",
                                headers_for(style, token), raw, timeout)
                count = None
                if res["status"] < 400 and res["raw"]:
                    try:
                        count = json.loads(res["raw"]).get("input_tokens")
                    except ValueError:
                        pass
            else:
                res = invoke(style, base, token, model,
                             chat_body(style, model, text, probe.get("max_tokens", 1)), timeout)
                _, _, _, _, usage, _ = decode_response(style, res)
                count = (usage or {}).get("input_tokens")
            entries.append({"seq": i + 1, "variant": variant, "status": res["status"],
                            "total_ms": res["total_ms"], "token_count": count,
                            "error_preview": None if res["status"] < 400 else
                            (res["raw"] or b"").decode("utf-8", "replace")[:300]})
            if isinstance(count, int):
                counts[variant].append(count)
    base_count = statistics.median(counts["base"]) if counts["base"] else None
    plus_count = statistics.median(counts["plus"]) if counts["plus"] else None
    delta = (plus_count - base_count) if base_count is not None and plus_count is not None else None
    return entries, {"base_count": base_count, "plus_count": plus_count, "delta": delta}


def run_prompt_probe(style, base, token, model, probe, timeout, repeats, keep_raw, blob_dir):
    kind = probe["kind"]
    if kind == "tool":
        body = tool_body(style, model, probe)
    elif kind == "stop":
        body = stop_body(style, model, probe)
    elif kind == "logprobs":
        body = logprobs_body(style, model, probe)
    else:
        body = chat_body(style, model, probe["prompt"], probe.get("max_tokens", 32))
    obs, logprobs = [], []
    for i in range(repeats):
        res = invoke(style, base, token, model, body, timeout)
        o = make_observation(i + 1, res, style, keep_raw, blob_dir)
        obs.append(o)
        if kind == "logprobs" and res["raw"] and res["status"] < 400:
            try:
                lp = summarize_logprobs(json.loads(res["raw"]))
            except ValueError:
                lp = None
            if lp:
                logprobs.append(lp)
                obs[-1]["logprobs_digest"] = lp["digest"]
    features = {}
    outs = [o["output_text"] for o in obs if o["output_text"]]
    features["finish_reasons"] = [o["finish_reason"] for o in obs]
    if kind == "tool":
        tools = [o for o in obs if o.get("tool_name")]
        raw_args = tools[0]["tool_args"] if tools else None
        canonical = raw_args
        if raw_args:
            try:
                canonical = json.dumps(json.loads(raw_args), ensure_ascii=False, sort_keys=True)
            except ValueError:
                pass
        features["tool_name"] = tools[0]["tool_name"] if tools else None
        features["tool_args"] = canonical
        features["tool_args_raw"] = raw_args
        features["tool_args_sha256"] = sha256_hex(canonical or "") if tools else None
    elif kind == "stop":
        features["finish_reasons"] = [o["finish_reason"] for o in obs]
        features["outputs"] = outs
    elif kind == "logprobs":
        if logprobs:
            features["tokens"] = logprobs[0]["tokens"]
            features["top1_prob_mean"] = statistics.median([x["top1_prob_mean"] for x in logprobs if x["top1_prob_mean"] is not None])
            features["margin_mean"] = statistics.median([x["margin_mean"] for x in logprobs if x["margin_mean"] is not None])
            features["entropy_mean"] = statistics.median([x["entropy_mean"] for x in logprobs if x["entropy_mean"] is not None])
        else:
            features["available"] = False
    else:
        modal = ""
        if outs:
            modal = max(set(outs), key=lambda v: (outs.count(v), v))
        features["outputs"] = outs
        features["modal_output"] = modal
        if "expected" in probe:
            features["expected"] = probe["expected"]
            features["exact_match_rate"] = sum(1 for o in outs if o == probe["expected"]) / len(outs) if outs else 0.0
    return obs, features


def build_record(args, probes, t0, t1, t2, quality):
    target = {
        "requested_model": args.model,
        "reported_model": None,
        "provider": {
            "uuid": None,
            "name": args.provider_name or "",
            "api_style": args.style,
            "api_base_host": urlsplit(args.base_url).hostname,
            "channel": args.channel,
            "region": args.region or None,
        },
        "protocol": "openai_chat" if args.style == "openai" else "anthropic_v1",
        "gateway": {"version": None, "rule_uuid": None, "scenario": None, "flags": {}},
    }
    if t1:
        for p in t1["probes"]:
            for o in p["observations"]:
                if o.get("reported_model"):
                    target["reported_model"] = o["reported_model"]
                    break
            if target["reported_model"]:
                break
    record = {
        "schema_version": SCHEMA_VERSION,
        "fingerprint_id": None,
        "kind": args.kind,
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "tool": {"name": "tb-fingerprint", "version": TOOL_VERSION, "git_sha": args.git_sha or None},
        "target": target,
        "probe_set": {"id": probes["id"], "version": probes["version"],
                      "sha256": sha256_hex(canon(probes)), "tiers": args.tiers,
                      "calibration": probes.get("calibration")},
        "feature_extractor_version": "fx/1",
        "params": {"temperature": 0, "top_p": 1, "seed": None, "max_tokens": None,
                   "system_prompt": "none", "stop": []},
        "environment": {"collected_from": "script", "network": args.network or "unknown",
                        "rtt_estimate_ms": None, "host": sys.platform},
        "tiers": {"t0": t0, "t1": t1, "t2": t2},
        "quality": quality,
        "integrity": None,
    }
    without = {k: v for k, v in record.items() if k not in ("fingerprint_id", "integrity")}
    record["fingerprint_id"] = sha256_hex(canon(without))
    with_id = {k: v for k, v in record.items() if k != "integrity"}
    record["integrity"] = {"record_sha256": sha256_hex(canon(with_id)), "signature": None}
    return record


def main():
    ap = argparse.ArgumentParser(description="Collect a model fingerprint directly from an upstream API.")
    ap.add_argument("--style", choices=["openai", "anthropic"], required=True)
    ap.add_argument("--base-url", required=True)
    ap.add_argument("--model", required=True)
    ap.add_argument("--provider-name", default="")
    ap.add_argument("--channel", choices=["direct", "through_tb", "through_aggregator"], default="direct")
    ap.add_argument("--region", default="")
    ap.add_argument("--network", default="")
    ap.add_argument("--token-env", default="TB_FP_TOKEN")
    ap.add_argument("--probes", default=str(Path(__file__).with_name("probes.json")))
    ap.add_argument("--out", default="-")
    ap.add_argument("--kind", choices=["baseline", "observed"], default="observed")
    ap.add_argument("--tiers", default="t0,t1")
    ap.add_argument("--tier2", action="store_true", help="also run logprobs probes (OpenAI Chat only)")
    ap.add_argument("--repeats", type=int, default=None)
    ap.add_argument("--timeout", type=float, default=None)
    ap.add_argument("--sleep", type=float, default=0.2)
    ap.add_argument("--keep-raw", action="store_true")
    ap.add_argument("--git-sha", default=os.environ.get("GIT_SHA", ""))
    args = ap.parse_args()

    token = os.environ.get(args.token_env, "")
    if not token:
        sys.exit("missing token: set %s" % args.token_env)
    probes = json.loads(Path(args.probes).read_text(encoding="utf-8"))
    defaults = probes.get("defaults", {})
    repeats = args.repeats or defaults.get("repeats", 3)
    timeout = args.timeout or defaults.get("timeout_seconds", 60)
    tiers = [t.strip() for t in args.tiers.split(",") if t.strip()]
    out_path = Path(args.out) if args.out != "-" else None
    blob_dir = (out_path.parent / (out_path.stem + ".blobs")) if out_path else Path("fingerprint.blobs")

    quality = {"requests": 0, "ok": 0, "failed": 0, "rate_limited": 0, "truncated": 0,
               "failure_rate": 0.0, "confidence": "low", "warnings": []}
    t0, t1, t2 = None, None, None

    if "t0" in tiers:
        t0 = run_t0(args.style, args.base_url, token, args.model, probes, timeout,
                    args.keep_raw, blob_dir)
        for r in t0["requests"]:
            quality["requests"] += 1
            quality["ok" if r["status"] < 400 else "failed"] += 1
            if r["status"] == 429:
                quality["rate_limited"] += 1
        time.sleep(args.sleep)

    if "t1" in tiers:
        entries = []
        for probe in probes["probes"]:
            if probe["kind"] == "logprobs":
                continue
            if probe["kind"] == "token_delta":
                obs, feats = run_token_delta(args.style, args.base_url, token, args.model,
                                             probe, timeout, min(repeats, 2))
            else:
                obs, feats = run_prompt_probe(args.style, args.base_url, token, args.model,
                                              probe, timeout, repeats, args.keep_raw, blob_dir)
            entries.append({
                "probe_id": probe["id"], "category": probe["category"], "tier": "t1",
                "weight": probe.get("weight", 1.0),
                "input_sha256": sha256_hex(canon({k: probe.get(k) for k in
                                                  ("base", "add", "prompt", "stop", "tool")})),
                "input_preview": (probe.get("prompt") or probe.get("base") or "")[:160],
                "request": {k: probe[k] for k in ("max_tokens", "stop", "tool") if k in probe},
                "repeats": len(obs), "observations": obs, "features": feats,
            })
            for o in obs:
                quality["requests"] += 1
                quality["ok" if 0 < o["status"] < 400 else "failed"] += 1
                if o["status"] == 429:
                    quality["rate_limited"] += 1
            time.sleep(args.sleep)
        t1 = {"probes": entries, "signature": None}

    if args.tier2:
        if args.style != "openai":
            quality["warnings"].append("tier2 logprobs skipped: Anthropic-shaped endpoint")
        else:
            lp_entries = []
            for probe in probes["probes"]:
                if probe["kind"] != "logprobs":
                    continue
                obs, feats = run_prompt_probe(args.style, args.base_url, token, args.model,
                                              probe, timeout, repeats, args.keep_raw, blob_dir)
                lp_entries.append({"probe_id": probe["id"], "observations": obs, "features": feats})
                for o in obs:
                    quality["requests"] += 1
                    quality["ok" if 0 < o["status"] < 400 else "failed"] += 1
                if not feats or feats.get("available") is False:
                    quality["warnings"].append("logprobs unsupported or missing for %s" % probe["id"])
                time.sleep(args.sleep)
            t2 = {"available": any(bool((e.get("features") or {}).get("tokens")) for e in lp_entries),
                  "logprobs": {"probes": lp_entries}}

    if quality["requests"]:
        quality["failure_rate"] = round(quality["failed"] / quality["requests"], 4)
        quality["confidence"] = "high" if quality["failure_rate"] < 0.05 else (
            "medium" if quality["failure_rate"] < 0.2 else "low")

    record = build_record(args, probes, t0, t1, t2, quality)
    text = json.dumps(record, ensure_ascii=False, indent=2)
    if out_path:
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(text + "\n", encoding="utf-8")
        print("wrote %s (%s, %d requests)" % (out_path, record["fingerprint_id"][:19],
                                              quality["requests"]), file=sys.stderr)
    else:
        print(text)


if __name__ == "__main__":
    main()


