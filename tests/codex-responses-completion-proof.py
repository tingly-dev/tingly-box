#!/usr/bin/env python3
"""Run the same real Codex/vmodel matrix with only the gateway handler changed."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
from datetime import datetime, timezone


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex", required=True, type=Path)
    parser.add_argument("--baseline", required=True, help="Git revision before the fix")
    parser.add_argument("--output", required=True, type=Path, help="New evidence directory")
    parser.add_argument("--case", default="", choices=["", "valid-usage", "absent-usage", "null-usage", "null-counters", "truncated"], help="Optional single case")
    parser.add_argument("--retries", type=int, default=1)
    args = parser.parse_args()
    if args.retries < 0:
        parser.error("--retries must be nonnegative")
    repo = Path(__file__).resolve().parents[1]
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    codex = args.codex.resolve()

    def git(*arguments):
        return subprocess.check_output(["git", *arguments], cwd=repo)

    handler = repo / "internal/protocol/stream/openai_passthrough.go"
    baseline = git("rev-parse", args.baseline).decode().strip()
    original = git("show", baseline + ":internal/protocol/stream/openai_passthrough.go")
    (output / "handler-before.go.txt").write_bytes(original)
    (output / "handler-after.go.txt").write_bytes(handler.read_bytes())
    overlay = output / "before-overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(handler): str(output / "handler-before.go.txt")}}, indent=2) + "\n")
    manifest = {
        "started_utc": datetime.now(timezone.utc).isoformat(),
        "baseline_revision": baseline,
        "working_revision": git("rev-parse", "HEAD").decode().strip(),
        "working_status": git("status", "--short").decode(),
        "submodules": git("submodule", "status").decode(),
        "go_version": subprocess.check_output(["go", "version"]).decode().strip(),
        "codex_binary_sha256": hashlib.sha256(codex.read_bytes()).hexdigest(),
        "handler_before_sha256": hashlib.sha256(original).hexdigest(),
        "handler_after_sha256": hashlib.sha256(handler.read_bytes()).hexdigest(),
        "test_sha256": hashlib.sha256((repo / "internal/protocoltest/codex_completion_e2e_test.go").read_bytes()).hexdigest(),
        "comparison": "same SDKs, gateway and test; before stage overlays only openai_passthrough.go from baseline",
        "retry_limit": args.retries,
        "runs": [],
    }
    for stage in ("before", "after"):
        command = ["go", "test"]
        if stage == "before":
            command += ["-overlay", str(overlay)]
        command += ["./internal/protocoltest", "-run", "^TestCodexResponsesCompletionE2E$", "-count=1", "-v"]
        env = dict(os.environ, TINGLY_CODEX_BIN=str(codex), TINGLY_CODEX_EVIDENCE_DIR=str(output), TINGLY_CODEX_EXPECT=stage, TINGLY_CODEX_RETRIES=str(args.retries), TINGLY_CODEX_CASE=args.case)
        print("Running", stage, flush=True)
        with (output / f"{stage}.test.log").open("w") as log:
            result = subprocess.run(command, cwd=repo, env=env, stdout=log, stderr=subprocess.STDOUT, timeout=600)
        manifest["runs"].append({"stage": stage, "command": command, "exit_code": result.returncode})
        (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        if result.returncode:
            print((output / f"{stage}.test.log").read_text()[-12000:], file=sys.stderr)
            return result.returncode

    results = [json.loads(p.read_text()) for p in sorted(output.glob("*/*/result.json"))]
    expected = 2 if args.case else 10
    if len(results) != expected:
        raise RuntimeError(f"Expected {expected} case results, got {len(results)}")
    for before in (output / "before").glob("*/upstream.sse"):
        after = output / "after" / before.parent.name / "upstream.sse"
        if before.read_bytes() != after.read_bytes():
            raise RuntimeError(f"Upstream fixtures differ for {before.parent.name}")
    manifest["fixture_pairs_byte_identical"] = True
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    (output / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    checksums = []
    for path in sorted(output.rglob("*")):
        if path.is_file():
            checksums.append(hashlib.sha256(path.read_bytes()).hexdigest() + "  " + str(path.relative_to(output)))
    (output / "SHA256SUMS").write_text("\n".join(checksums) + "\n")
    for result in results:
        print(f"{result['stage']:6} {result['case']:14} exit={result['exit_code']} requests={result['gateway_requests']} retries={result['reconnects']} completed={result['turn_completed']} failed={result['turn_failed']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
