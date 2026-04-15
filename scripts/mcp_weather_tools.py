#!/usr/bin/env python3
"""
MCP stdio server exposing:
  - get_current_weather: simple weather lookup via Open-Meteo geocoding + forecast API
"""

from __future__ import annotations

import json
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, Optional


JSONRPC_VERSION = "2.0"

TOOL_WEATHER = {
    "name": "get_current_weather",
    "description": "Get current weather by city name.",
    "inputSchema": {
        "type": "object",
        "properties": {
            "location": {"type": "string", "description": "City name, e.g. Tokyo"},
            "unit": {
                "type": "string",
                "description": "Temperature unit",
                "enum": ["celsius", "fahrenheit"],
                "default": "celsius",
            },
        },
        "required": ["location"],
    },
}

WEATHER_CODES = {
    0: "clear",
    1: "mainly clear",
    2: "partly cloudy",
    3: "overcast",
    45: "fog",
    48: "depositing rime fog",
    51: "light drizzle",
    53: "moderate drizzle",
    55: "dense drizzle",
    61: "slight rain",
    63: "moderate rain",
    65: "heavy rain",
    71: "slight snow",
    73: "moderate snow",
    75: "heavy snow",
    80: "rain showers",
    81: "rain showers",
    82: "violent rain showers",
    95: "thunderstorm",
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


def _urlopen(req: urllib.request.Request, timeout: int):
    try:
        return urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.URLError as e:
        msg = str(getattr(e, "reason", e))
        if "CERTIFICATE_VERIFY_FAILED" not in msg:
            raise
        insecure_ctx = ssl._create_unverified_context()
        return urllib.request.urlopen(req, timeout=timeout, context=insecure_ctx)


def _http_get_json(url: str, timeout: int = 20) -> Dict[str, Any]:
    req = urllib.request.Request(url, method="GET", headers={"User-Agent": "tingly-box-mcp-weather/0.1"})
    with _urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode("utf-8", errors="replace"))


def tool_get_current_weather(args: Dict[str, Any]) -> Dict[str, Any]:
    location = str(args.get("location", "")).strip()
    if not location:
        raise ValueError("location is required")
    unit = str(args.get("unit", "celsius")).strip().lower()
    if unit not in ("celsius", "fahrenheit"):
        unit = "celsius"

    geo_url = "https://geocoding-api.open-meteo.com/v1/search?" + urllib.parse.urlencode({
        "name": location,
        "count": 1,
        "language": "en",
        "format": "json",
    })
    geo = _http_get_json(geo_url)
    results = geo.get("results") or []
    if not results:
        raise ValueError(f"location not found: {location}")
    first = results[0]
    lat = first.get("latitude")
    lon = first.get("longitude")
    resolved_name = first.get("name") or location
    country = first.get("country")
    tz = first.get("timezone")

    weather_url = "https://api.open-meteo.com/v1/forecast?" + urllib.parse.urlencode({
        "latitude": lat,
        "longitude": lon,
        "current": "temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m",
        "timezone": "auto",
        "temperature_unit": "fahrenheit" if unit == "fahrenheit" else "celsius",
        "wind_speed_unit": "kmh",
    })
    w = _http_get_json(weather_url)
    current = w.get("current") or {}
    code = int(current.get("weather_code", 0))
    condition = WEATHER_CODES.get(code, f"code-{code}")
    temp = current.get("temperature_2m")
    humidity = current.get("relative_humidity_2m")
    wind = current.get("wind_speed_10m")
    temp_unit_symbol = "°F" if unit == "fahrenheit" else "°C"

    display_location = resolved_name if not country else f"{resolved_name}, {country}"
    summary = (
        f"Current weather in {display_location}: {condition}, {temp}{temp_unit_symbol}, "
        f"wind {wind} km/h, humidity {humidity}%."
    )

    structured = {
        "tool": "get_current_weather",
        "location": display_location,
        "timezone": tz,
        "condition": condition,
        "temperature": temp,
        "temperature_unit": unit,
        "humidity": humidity,
        "wind_kmh": wind,
    }
    return {
        "content": [{"type": "text", "text": summary}],
        "structuredContent": structured,
    }


def handle_request(req: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    rid = req.get("id")
    method = req.get("method")
    params = req.get("params") or {}

    is_notification = rid is None

    if method == "ping":
        if is_notification:
            return None
        return ok(rid, {})

    if method == "initialize":
        if is_notification:
            return None
        return ok(
            rid,
            {
                "protocolVersion": "2024-11-05",
                "capabilities": {"tools": {}},
                "serverInfo": {"name": "tingly-weather-tools", "version": "0.1.0"},
            },
        )

    if method == "tools/list":
        if is_notification:
            return None
        return ok(rid, {"tools": [TOOL_WEATHER]})

    if method == "tools/call":
        if is_notification:
            return None
        name = str(params.get("name", "")).strip()
        arguments = params.get("arguments") or {}
        if not isinstance(arguments, dict):
            return err(rid, -32602, "arguments must be an object")
        try:
            if name != "get_current_weather":
                return err(rid, -32601, f"tool not found: {name}")
            result = tool_get_current_weather(arguments)
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
        return None
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
