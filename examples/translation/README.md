# Try the unified translation endpoint

This first implementation supports native `hunyuan_mt` and `qwen_mt` adapters
over OpenAI-compatible serving endpoints. It uses a configured adapter per
service, so an arbitrary deployment alias such as `local-mt` works.

## Run Tencent's smallest parameter-size model through Python

The example loads `tencent/Hy-MT2-1.8B` with Transformers and serves it using
this repository's Python SDK (`tingly.Server.openai_chat`). Model
weights are downloaded separately from the code; expect several GB of storage
and RAM. CPU inference works; a GPU is not required for this small smoke check.

```bash
python3 -m venv work/translation-demo/venv
work/translation-demo/venv/bin/pip install torch --index-url https://download.pytorch.org/whl/cpu
work/translation-demo/venv/bin/pip install -r examples/translation/requirements.txt
work/translation-demo/venv/bin/pip install -e sdk/python
HF_HOME="$PWD/work/translation-demo/hf" work/translation-demo/venv/bin/python \
  examples/translation/hunyuan_server.py --served-model-name local-mt
```

For reproducibility, supply `--revision <model-commit>` or pass a downloaded
snapshot directory with `--model`. The development server binds to loopback and
serializes generation. `GET http://127.0.0.1:18081/v1/models` becomes available
after the model has loaded. It is intentionally a small serving example, not a
production inference scheduler. A canceled HTTP call does not forcibly stop an
already executing Python generation; its token budget bounds that work.

In a second terminal, start the production gateway handlers with an isolated
temporary demo configuration:

```bash
go run ./examples/translation/gateway
```

Then call the gateway:

```bash
curl http://127.0.0.1:18080/tingly/custom/v1/translate \
  -H 'Authorization: Bearer local-translation-demo' \
  -H 'Content-Type: application/json' \
  -d '{"text":["Hello, world!","Save changes"],"source_lang":"auto","target_lang":"zh-Hans"}'
```

The response always has `model` and an ordered `translations` array of text
objects. Both `/tingly/custom/translate` and `/tingly/custom/v1/translate` work.

The retained SDK client example calls the same gateway:

```bash
work/translation-demo/venv/bin/python sdk/python/examples/translate.py \
  --target zh-Hans 'Hello, world!' 'Save changes'
```

Set `TINGLY_BASE_URL` and `TINGLY_TOKEN` to use another gateway. The example's
defaults match this loopback demo. Both the SDK client and model-serving
implementation remain in the repository; neither downloads weights on import.

## Connect a hosted Qwen-MT endpoint

Set `TINGLY_TRANSLATION_API_KEY` through your usual environment/secret manager,
then run the demo with `--backend qwen_mt --model qwen-mt-flash --upstream
<your-regional-compatible-mode-base-url>`. An explicit `--proxy` is available.
The gateway converts language tags to native translation options and sends the
original text as a user message. No system message is injected.

In a regular Tingly configuration, create a provider and rule through the
management API. The rule's service JSON includes:

```json
{
  "provider": "your-provider-uuid",
  "model": "your-upstream-model-alias",
  "active": true,
  "translation_backend": "hunyuan_mt"
}
```

Use `qwen_mt` for Qwen. The rule request-model alias defaults to `translate` in
the public request. Rules and providers persist in the database; do not add a
`rules` array to `config.json`. A dedicated frontend adapter selector is not
included in this version; use the demo or management API to configure it.

## Limits and verification

- JSON, synchronous plain text; 1–32 independent items, 32 KiB per item and
  256 KiB total request body. Unsupported fields are rejected.
- Language support is the explicit map in `internal/translation`; unverified
  region variants and Qwen Traditional Chinese are rejected rather than
  silently converted. Hunyuan supports the configured Traditional Chinese mapping.
- Native API-key/no-key OpenAI endpoints only; generic chat, OAuth, multi-field
  credentials, smart-routing partitions, HTML, glossary, streaming and files
  are outside this first version. There is no new guardrails coverage claim.
- Configured base-rule load balancing/failover applies per item. Completed items
  are not replayed. A failed batch returns an error and zero-based input index;
  earlier work can already have consumed quota.
- Tokens are recorded when actually reported by the model. Missing usage does
  not produce a successful token measurement. No detected source language or
  confidence is fabricated.
- DeepL/LibreTranslate native backends and client compatibility routes remain
  follow-ups under TEP-2254.

Run deterministic protocol checks:

```bash
go test ./internal/translation ./internal/protocolserver -run '^TestTranslation' -count=1
```

After starting the Python backend, opt into the real-model gateway check:

```bash
TINGLY_TRANSLATION_LIVE_BASE_URL=http://127.0.0.1:18081 \
  go test ./internal/protocolserver -run '^TestTranslationLiveHunyuan$' -v -count=1
```

That check reports actual translations through the authenticated gateway. It is
a connectivity/behavior smoke check, not a translation-quality benchmark.
