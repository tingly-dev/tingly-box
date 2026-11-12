"""Call the unified gateway. Start examples/translation's model + gateway first.

    python sdk/python/examples/translate.py --target zh-Hans 'Hello, world!'

Override TINGLY_BASE_URL / TINGLY_TOKEN for your own Tingly instance.
"""
import argparse
import json
import os

from tingly import Client


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("text", nargs="+")
    parser.add_argument("--target", required=True)
    parser.add_argument("--source", default="auto")
    parser.add_argument("--model", default="translate")
    parser.add_argument("--scenario", default="custom")
    args = parser.parse_args()
    tb = Client(
        os.environ.get("TINGLY_BASE_URL", "http://127.0.0.1:18080"),
        os.environ.get("TINGLY_TOKEN", "local-translation-demo"),
        scenario=args.scenario,
    )
    result = tb.translate(args.text, args.target, source_lang=args.source, model=args.model)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
