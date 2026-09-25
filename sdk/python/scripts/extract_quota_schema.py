#!/usr/bin/env python3
"""Extract the provider-quota paths and their schema closure out of
openapi.json into a minimal spec, for datamodel-code-generator to consume.

Why: openapi.json now describes ~280 schemas across the whole backend, but
the Python SDK's quota client only needs the half-dozen types the
provider-quota endpoints actually return. Generating from the full spec
would reintroduce exactly the "_generated/ models off the full spec" scope
this SDK deliberately walked back from (see .design/python-sdk.md) — so
this script keeps the generated surface proportional to what is used
instead.

Usage: python3 extract_quota_schema.py <openapi.json> <output.json>
"""

import json
import sys

QUOTA_PATH_PREFIX = "/api/v1/provider-quota"


def collect_refs(node, refs):
    if isinstance(node, dict):
        ref = node.get("$ref")
        if isinstance(ref, str) and ref.startswith("#/components/schemas/"):
            refs.add(ref.rsplit("/", 1)[-1])
        for value in node.values():
            collect_refs(value, refs)
    elif isinstance(node, list):
        for item in node:
            collect_refs(item, refs)


def main():
    if len(sys.argv) != 3:
        print(f"usage: {sys.argv[0]} <openapi.json> <output.json>", file=sys.stderr)
        return 1

    spec_path, out_path = sys.argv[1], sys.argv[2]
    spec = json.load(open(spec_path))

    paths = {p: methods for p, methods in spec["paths"].items() if p.startswith(QUOTA_PATH_PREFIX)}
    if not paths:
        print(f"no paths under {QUOTA_PATH_PREFIX} found in {spec_path}", file=sys.stderr)
        return 1

    all_schemas = spec["components"]["schemas"]
    closure = set()
    frontier = set()
    collect_refs(paths, frontier)
    while frontier:
        name = frontier.pop()
        if name in closure or name not in all_schemas:
            continue
        closure.add(name)
        collect_refs(all_schemas[name], frontier)

    reduced = {
        "openapi": spec.get("openapi", "3.0.0"),
        "info": {"title": "tingly-box quota API (extracted)", "version": "0"},
        "paths": paths,
        "components": {"schemas": {name: all_schemas[name] for name in closure}},
    }
    json.dump(reduced, open(out_path, "w"), indent=2)
    print(f"extracted {len(closure)} schema(s), {len(paths)} path(s) -> {out_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
