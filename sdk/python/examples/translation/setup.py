"""Register this SDK provider with an ordinary running tb through its admin API.

Requires TINGLY_ADMIN_TOKEN (UserToken). The client uses TINGLY_TOKEN (ModelToken).
"""
import argparse
import json
import os
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-base")
    parser.add_argument("--api-style", choices=["openai", "anthropic", "libretranslate", "deepl"], default="openai")
    parser.add_argument("--rule-model", default="translate")
    parser.add_argument("--upstream-model", default=None)
    args = parser.parse_args()
    if args.api_base is None:
        if args.api_style != "openai":
            parser.error("--api-base is required for this native service protocol")
        args.api_base = "http://127.0.0.1:18681/v1"
    base = os.environ.get("TINGLY_BASE_URL", "http://127.0.0.1:12680").rstrip("/")
    token = os.environ["TINGLY_ADMIN_TOKEN"]
    upstream_token = os.environ.get("TINGLY_TRANSLATION_API_KEY", "")
    upstream_model = args.upstream_model if args.upstream_model is not None else ("" if args.api_style in ("libretranslate", "deepl") else "tencent/Hy-MT2-1.8B")

    def post(path, body):
        request = urllib.request.Request(base + path, json.dumps(body).encode(), headers={
            "Authorization": f"Bearer {token}", "Content-Type": "application/json",
        }, method="POST")
        with urllib.request.urlopen(request, timeout=30) as response:
            return json.load(response)

    provider = post("/api/v2/providers", {
        "name": "sdk-" + args.rule_model, "api_base": args.api_base, "api_style": args.api_style,
        "no_key_required": not bool(upstream_token), "token": upstream_token,
    })["data"]
    rule = post("/api/v1/rule", {
        "scenario": "custom", "request_model": args.rule_model, "response_model": args.rule_model,
        "active": True, "services": [{
            "provider": provider["uuid"], "model": upstream_model,
            "active": True, "weight": 1,
        }],
    })
    print(json.dumps({"provider_uuid": provider["uuid"], "rule": rule}, indent=2))


if __name__ == "__main__":
    main()
