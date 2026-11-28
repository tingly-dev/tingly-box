#!/usr/bin/env python3
"""Compare two model-fingerprint records (stdlib only).

This is the draft decision layer described in .research/model-fingerprinting.md
section 7. The thresholds are placeholders until a baseline library is
calibrated; the output says PROVISIONAL in that case.

Usage:
  python3 compare.py observed.json --baseline baseline.json
  python3 compare.py observed.json --baseline-dir baselines/
"""

import argparse
import json
import statistics
import sys
from difflib import SequenceMatcher
from pathlib import Path


def modal(values):
    vals = [v for v in values if v]
    if not vals:
        return ""
    return max(set(vals), key=lambda v: (vals.count(v), v))


def modal_output(features):
    if not features:
        return ""
    return features.get("modal_output") or modal(features.get("outputs") or [])


def weighted_median(pairs, default=None):
    usable = [(s, w) for s, w in pairs if s is not None]
    if not usable:
        return default
    usable.sort(key=lambda x: x[0])
    total = sum(w for _, w in usable)
    acc = 0.0
    for s, w in usable:
        acc += w
        if acc >= total / 2.0:
            return s
    return usable[-1][0]


def text_sim(a, b):
    if not a and not b:
        return 1.0
    if not a or not b:
        return 0.0
    return SequenceMatcher(None, a, b).ratio()


def rel_sim(pairs):
    """1 - mean relative difference, clipped to [0,1]."""
    usable = [(a, b) for a, b in pairs if isinstance(a, (int, float)) and isinstance(b, (int, float))]
    if not usable:
        return None
    diffs = [abs(a - b) / max(1e-9, abs(b)) for a, b in usable]
    return max(0.0, 1.0 - statistics.fmean(diffs))


def probe_similarity(probe, base_probe):
    cat = probe.get("category")
    pf, bf = probe.get("features") or {}, base_probe.get("features") or {}
    if cat == "tokenizer":
        a, b = pf.get("delta"), bf.get("delta")
        if not isinstance(a, int) or not isinstance(b, int):
            return None
        return 1.0 if a == b else max(0.0, 1.0 - abs(a - b) / max(1, abs(b)))
    if cat == "tool":
        if not pf.get("tool_name") or not bf.get("tool_name"):
            return 0.0
        if pf["tool_name"] != bf["tool_name"]:
            return 0.0
        return 1.0 if pf.get("tool_args") == bf.get("tool_args") else 0.4
    if cat == "stop":
        a = modal(pf.get("finish_reasons") or [])
        b = modal(bf.get("finish_reasons") or [])
        if not a or not b:
            return None
        return 1.0 if a == b else 0.0
    if cat == "logprobs":
        if not pf.get("tokens") or not bf.get("tokens"):
            return None
        same_tokens = 1.0 if pf["tokens"] == bf["tokens"] else 0.0
        dist = rel_sim([
            (pf.get("top1_prob_mean"), bf.get("top1_prob_mean")),
            (pf.get("margin_mean"), bf.get("margin_mean")),
            (pf.get("entropy_mean"), bf.get("entropy_mean")),
        ])
        if dist is None:
            return same_tokens
        return 0.7 * dist + 0.3 * same_tokens
    # canonical / knowledge / anything text-shaped
    return text_sim(modal_output(pf), modal_output(bf))


def compare_records(observed, baseline):
    warnings = []
    obs_ps = observed.get("probe_set") or {}
    base_ps = baseline.get("probe_set") or {}
    compatible = (obs_ps.get("id") == base_ps.get("id") and
                  obs_ps.get("version") == base_ps.get("version"))
    if not compatible:
        warnings.append("probe_set differs (%s/%s vs %s/%s); comparing shared probe ids only" % (
            obs_ps.get("id"), obs_ps.get("version"), base_ps.get("id"), base_ps.get("version")))

    obs_t0 = ((observed.get("tiers") or {}).get("t0") or {}).get("signature") or {}
    base_t0 = ((baseline.get("tiers") or {}).get("t0") or {}).get("signature") or {}
    serving_path = None
    if obs_t0.get("serving_path_sig") and base_t0.get("serving_path_sig"):
        serving_path = 1.0 if obs_t0["serving_path_sig"] == base_t0["serving_path_sig"] else 0.0
        if serving_path == 0.0:
            warnings.append("serving path signature differs; model comparison continues but headers/route may have changed")

    base_probes = {p["probe_id"]: p for p in (((baseline.get("tiers") or {}).get("t1") or {}).get("probes") or [])}
    cat_pairs = {}
    matched = missing = 0
    for p in (((observed.get("tiers") or {}).get("t1") or {}).get("probes") or []):
        bp = base_probes.get(p["probe_id"])
        if not bp:
            missing += 1
            continue
        s = probe_similarity(p, bp)
        if s is None:
            missing += 1
            continue
        matched += 1
        cat_pairs.setdefault(p.get("category") or "other", []).append((s, float(p.get("weight", 1.0))))

    cat_scores = {c: weighted_median(v) for c, v in cat_pairs.items()}
    cat_weights = {c: sum(w for _, w in v) for c, v in cat_pairs.items()}
    overall = weighted_median([(s, cat_weights[c]) for c, s in cat_scores.items() if s is not None])

    cal = (observed.get("probe_set") or {}).get("calibration") or {}
    if not cal:
        cal = {"status": "uncalibrated", "tau_match": 0.90, "tau_mismatch": 0.60,
               "critical_category_threshold": 0.99,
               "critical_categories": ["tokenizer", "tool", "stop"]}
    if cal.get("status") != "calibrated":
        warnings.append("thresholds are uncalibrated placeholders")
    critical = [c for c in cal.get("critical_categories", []) if c in cat_scores]
    critical_fail = [c for c in critical
                     if cat_scores[c] is not None and cat_scores[c] < cal.get("critical_category_threshold", 0.99)]

    if overall is None:
        verdict = "INCONCLUSIVE"
    elif critical_fail:
        verdict = "DIFFERENT_MODEL"
    elif overall >= cal.get("tau_match", 0.90):
        verdict = "MATCH"
    elif overall <= cal.get("tau_mismatch", 0.60):
        verdict = "DIFFERENT_MODEL"
    elif cat_scores.get("canonical") is not None or cat_scores.get("knowledge") is not None:
        verdict = "SAME_FAMILY_DIFFERENT_VERSION"
    else:
        verdict = "INCONCLUSIVE"
    if cal.get("status") != "calibrated":
        verdict = "PROVISIONAL_" + verdict

    return {
        "verdict": verdict,
        "confidence": ("high" if matched >= 10 and not warnings else
                       "medium" if matched >= 5 else "low"),
        "overall": overall,
        "category_scores": cat_scores,
        "critical_failures": critical_fail,
        "serving_path_match": serving_path,
        "matched_probes": matched,
        "missing_probes": missing,
        "baseline_fingerprint_id": baseline.get("fingerprint_id"),
        "baseline_target": (baseline.get("target") or {}).get("requested_model"),
        "observed_target": (observed.get("target") or {}).get("requested_model"),
        "warnings": warnings,
    }


def load_records(path):
    p = Path(path)
    if p.is_dir():
        return [(str(f), json.loads(f.read_text(encoding="utf-8")))
                for f in sorted(p.rglob("*.json")) if f.name != "manifest.json"]
    return [(str(p), json.loads(p.read_text(encoding="utf-8")))]


def main():
    ap = argparse.ArgumentParser(description="Compare fingerprint records.")
    ap.add_argument("observed")
    ap.add_argument("--baseline", help="single baseline JSON")
    ap.add_argument("--baseline-dir", help="directory of baseline JSON files")
    ap.add_argument("--json", action="store_true", help="print full JSON result")
    ap.add_argument("--top", type=int, default=3)
    args = ap.parse_args()
    if not args.baseline and not args.baseline_dir:
        sys.exit("pass --baseline or --baseline-dir")
    observed = json.loads(Path(args.observed).read_text(encoding="utf-8"))
    baselines = load_records(args.baseline or args.baseline_dir)
    results = []
    for path, rec in baselines:
        if rec.get("kind") == "comparison":
            continue
        r = compare_records(observed, rec)
        r["baseline_path"] = path
        results.append(r)
    results.sort(key=lambda r: (-(r["overall"] if r["overall"] is not None else -1), r["verdict"]))
    best = results[0] if results else {"verdict": "INCONCLUSIVE", "warnings": ["no baselines"]}
    if args.json:
        print(json.dumps({"best": best, "top": results[:args.top]}, ensure_ascii=False, indent=2))
        return
    print("observed: %s   baselines: %d" % (observed.get("fingerprint_id", "?"), len(results)))
    for r in results[:args.top]:
        score = "n/a" if r["overall"] is None else "%.3f" % r["overall"]
        print("  %-34s overall=%s  %s" % (r.get("baseline_target") or Path(r["baseline_path"]).name,
                                          score, r["verdict"]))
        cats = ", ".join("%s=%s" % (c, "n/a" if s is None else "%.2f" % s)
                         for c, s in sorted((r.get("category_scores") or {}).items()))
        print("      %s" % cats)
        if r.get("warnings"):
            print("      warnings: %s" % "; ".join(r["warnings"]))
    print("\nbest: %s" % best["verdict"])
    if best.get("critical_failures"):
        print("critical failures: %s" % ", ".join(best["critical_failures"]))


if __name__ == "__main__":
    main()
