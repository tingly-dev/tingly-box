"""Register this SDK provider with an ordinary running tb through its admin API.

Requires TINGLY_ADMIN_TOKEN (UserToken). The client uses TINGLY_TOKEN (ModelToken).
"""
import argparse
import json
import os
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-base", default="http://127.0.0.1:18081/v1")
    parser.add_argument("--upstream-model", default="tencent/Hy-MT2-1.8B")
    args = parser.parse_args()
    base = os.environ.get("TINGLY_BASE_URL", "http://127.0.0.1:12580").rstrip("/")
    token = os.environ["TINGLY_ADMIN_TOKEN"]

    def post(path, body):
        request = urllib.request.Request(base + path, json.dumps(body).encode(), headers={
            "Authorization": f"Bearer {token}", "Content-Type": "application/json",
        }, method="POST")
        with urllib.request.urlopen(request, timeout=30) as response:
            return json.load(response)

    provider = post("/api/v2/providers", {
        "name": "sdk-translation", "api_base": args.api_base, "api_style": "openai",
        "no_key_required": True, "token": "",
    })["data"]
    rule = post("/api/v1/rule", {
        "scenario": "custom", "request_model": "translate", "response_model": "translate",
        "active": True, "services": [{
            "provider": provider["uuid"], "model": args.upstream_model,
            "active": True, "weight": 1,
        }],
    })
    print(json.dumps({"provider_uuid": provider["uuid"], "rule": rule}, indent=2))


if __name__ == "__main__":
    main()
