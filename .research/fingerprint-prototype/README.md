# `.research/fingerprint-prototype/` — draft model-fingerprint collector

Status: **prototype, not wired into tingly-box.** Design and evidence live in
[`../model-fingerprinting.md`](../model-fingerprinting.md).

This directory answers one question at the API level: *is the model behind this
endpoint still the model I think it is?* It collects cheap behavioral evidence
directly from the upstream (bypassing tingly-box's protocol conversion) and
compares it with a baseline.

## What it does

- Direct HTTP, stdlib only. No SDK, no `~/.tingly-box`, no database.
- Captures things the Go SDK path loses:
  - upstream response headers (allowlisted / pattern-classified)
  - exact body hashes
  - SSE event names, chunk count, TTFT, per-request timings
  - usage / `count_tokens` / `logprobs`
- Tier 0: `/models`, a minimal non-stream request, a minimal stream request,
  an unsupported-parameter probe, and Anthropic `count_tokens`.
- Tier 1 subset: tokenizer count deltas (CJK / emoji / rare Unicode / code /
  repeats / spaces), deterministic outputs, tool calls, stop/truncation,
  knowledge probes.
- Tier 2 optional: OpenAI Chat `logprobs`.
- Writes the schema drafted in section 6 of the research doc.

## Usage

```bash
export TB_FP_TOKEN=...        # never put the key on the command line
python3 collect.py \
  --style openai \
  --base-url https://api.openai.com/v1 \
  --model gpt-5.1 \
  --out fingerprints/gpt-5.1.openai.$(date +%Y%m%dT%H%M%SZ).json

python3 collect.py \
  --style anthropic \
  --base-url https://api.anthropic.com \
  --model claude-opus-4-5 \
  --out fingerprints/claude-opus-4-5.anthropic.json
```

Compare one observation against a baseline, or a whole baseline directory:

```bash
python3 compare.py fingerprints/observed.json --baseline fingerprints/baseline.json
python3 compare.py fingerprints/observed.json --baseline-dir fingerprints/baselines/
```

The thresholds shipped in `probes.json` are **placeholders** (`status:
uncalibrated`), so the verdict is prefixed `PROVISIONAL_`. Calibrate on
at least three independent baseline collections before trusting `MATCH`
(see section 7.4 of the research doc).

## Cost / safety

- Sequential requests, 0.2 s apart, small `max_tokens`. Do not run this in a
  loop against a production key.
- No long-context, image, or max-output probes in this draft; those are the
  expensive Tier 2 items.
- CI must not call real providers. Use a local mock (see below) and only test
  schema/compare logic.
- The token is read from `TB_FP_TOKEN` (override with `--token-env`). The
  record stores only the API host, never the key or full URL.
- `--keep-raw` writes raw bodies/events under `<out>.blobs/`; leave it off for
  baseline commits.

## Adding probes

Edit `probes.json`; each probe has `id`, `category`, `kind`, `weight`, and
kind-specific fields:

| kind | fields | feature |
|---|---|---|
| `token_delta` | `base`, `add`, `max_tokens` | exact input-token delta |
| `text` | `prompt`, `max_tokens`, optional `expected` | modal output + exact-match rate |
| `tool` | `prompt`, `tool`, `max_tokens` | tool name + canonical arguments |
| `stop` | `prompt`, `stop`, `max_tokens` | `finish_reason` + output |
| `logprobs` | `prompt`, `max_tokens`, `top_logprobs` | top1 prob / margin / entropy / tokens |

Bump `probes.json` `version` when changing inputs or ground truth; old
fingerprints are then not directly comparable.

## Testing without a real provider

`collect.py` is plain HTTP, so a tiny local server that speaks the OpenAI or
Anthropic shape is enough. The repo also has `vmodel` (Go) as a deterministic
virtual provider; either works. The important checks are:

1. `python3 -m py_compile collect.py compare.py`
2. `python3 collect.py --help`
3. same mock collected twice -> `compare.py` says `MATCH`
4. two mocks with different tokenizer counts / tool names -> `DIFFERENT_MODEL`

## Known limitations

- Anthropic has no logprobs; Tier 2 logprobs is OpenAI Chat only.
- `count_tokens` is used for the Anthropic tokenizer; OpenAI tokenizer counts
  come from chat `usage.prompt_tokens`, so they include any hidden preamble.
  The delta design (`count(base+X)-count(base)`) cancels a constant preamble.
- No context-window / max-output probing yet.
- The baseline library itself is not included; it must be collected from
  trusted official endpoints and reviewed before use.
