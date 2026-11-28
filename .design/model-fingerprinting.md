# Model fingerprinting (proposal)

Status: **proposal — no production code.** Authoritative research, evidence and
open questions live in [`../.research/model-fingerprinting.md`](../.research/model-fingerprinting.md).

## Decision summary

- Verify a remote model by collecting evidence **directly from the upstream**,
  not through TB's protocol conversion. TB rewrites `model`, synthesizes SSE
  headers, and drops/estimates fields on cross-protocol paths, so a through-TB
  fingerprint measures the gateway as much as the model.
- Split the result into **serving-path signature** (headers, schema, ids,
  error envelope, SSE shape, TTFT) and **model-core signature** (tokenizer
  counts, tool/format behavior, deterministic outputs, logprobs).
- Start with Tier 0 + Tier 1 tokenizer/tool/format probes; add Tier 2
  logprobs where available. Timing and headers are serving-path evidence only.
- A supplier that proxies the genuine model is indistinguishable from the
  genuine model by API. The scheme raises the cost of substitution; it does
  not prove weights.

## Where the draft lives

`.research/fingerprint-prototype/` — stdlib-only collector and comparator, not
wired into the CLI, DB, Bench or the probe subsystem. See its README for usage
and the known limitations.

## Next decision points

1. Freeze probe set v0.1 and collect a small baseline library from trusted
   official endpoints.
2. Calibrate `tau_match` / `tau_mismatch` per probe set + serving path.
3. Decide whether to promote the prototype to `internal/fingerprint` +
   `tingly-box fingerprint` once the probe set is stable.
4. Do not add a Taskfile target or UI until the user asks.
