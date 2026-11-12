# Translation through the Python SDK and ordinary tb

All translation examples live here. The model process is a Python SDK provider;
start the ordinary tb server and use its management API to configure routing.
The same endpoint accepts ordinary LLMs, native MT models and traditional
translation services through a backend-neutral Translator interface.
No special gateway executable or load-balancer schema is needed.
Run the commands below from the repository root.

## Load and serve Tencent 1.8B locally

```bash
python3 -m venv work/translation-demo/venv
work/translation-demo/venv/bin/pip install torch --index-url https://download.pytorch.org/whl/cpu
work/translation-demo/venv/bin/pip install -r sdk/python/examples/translation/requirements.txt
work/translation-demo/venv/bin/pip install -e sdk/python
HF_HOME="$PWD/work/translation-demo/hf" work/translation-demo/venv/bin/python \
  sdk/python/examples/translation/hunyuan_server.py \
  --revision 9a341cd1b679d3efd23b46e847b01745a71ed792
```

This loads `tencent/Hy-MT2-1.8B` through Transformers, then serves it through
`tingly.Server.openai_chat` on loopback port 18681. Several GB of disk and RAM
are needed. CPU inference works. `GET /v1/models` is available after loading.
Alternatively pass a downloaded snapshot directory with `--model`.

The served model ID defaults to `tencent/Hy-MT2-1.8B`, matching the capability
catalog. Keep that ID when registering the provider. Uncataloged model IDs use the generic LLM adapter, not native MT semantics.
Keep a canonical served ID for the MT example; with a local snapshot path,
pass `--served-model-name tencent/Hy-MT2-1.8B`. Public rule aliases remain free.

## Start normal tb and configure it

In a second terminal, use the normal CLI (an installed binary also works):

```bash
go run ./cli/tingly-box --config-dir "$PWD/work/translation-demo/tb" start \
  --no-daemon --host 127.0.0.1 --port 12680 --browser=false
```

Set `TINGLY_ADMIN_TOKEN` to that instance's UserToken and `TINGLY_TOKEN` to its
ModelToken using your normal secret mechanism. Do not commit either token.
The setup script creates one provider and one `custom` rule through the existing
management APIs, using only ordinary provider/model services:

```bash
export TINGLY_BASE_URL=http://127.0.0.1:12680
work/translation-demo/venv/bin/python sdk/python/examples/translation/setup.py
work/translation-demo/venv/bin/python sdk/python/examples/translation/client.py \
  --target zh-Hans 'Hello, world!' 'Save changes'
```

Run setup once per fresh configuration; repeated execution creates new resources
or may encounter existing rule conflicts. The returned UUIDs identify resources
for management through the normal API/UI. Rules persist in SQLite, not a `rules`
array in config.json.

The SDK calls `/tingly/custom/v1/translate` using the gateway token. The response
contains the logical model alias and ordered `translations` text objects.
`/tingly/custom/translate` also works. No demo token or special auth path exists.

## Ordinary LLM translation

No catalog entry is needed for a general instruction model. For example, load a
small local model through the same SDK serving code on a separate port:

```bash
HF_HOME="$PWD/work/translation-demo/hf" work/translation-demo/venv/bin/python \
  sdk/python/examples/translation/hunyuan_server.py \
  --model Qwen/Qwen2.5-0.5B-Instruct --revision 7ae557604adf67be50417f59c2c2f167def9a775 \
  --served-model-name local-llm --generic-chat --port 18683
work/translation-demo/venv/bin/python sdk/python/examples/translation/setup.py \
  --api-base http://127.0.0.1:18683/v1 --upstream-model local-llm --rule-model translate-llm
work/translation-demo/venv/bin/python sdk/python/examples/translation/client.py \
  --model translate-llm --target zh-Hans 'Hello, world!'
```

This is a local general Qwen LLM, not the hosted Qwen-MT API. The generic adapter
builds a translation instruction and keeps source text in a separate user
message. It reuses normal attempt planning, transforms, model limits, Chat /
Responses / Anthropic conversion and provider clients. It does not run an agent
tool loop; tool calls, refusals, truncation and empty answers fail validation.
An instruction is not a guarantee of translation accuracy or prompt-injection
resistance. Google-native, OAuth, multi-field credentials and vmodel providers
remain unsupported in this first backend slice.

## Traditional services

Create providers using the existing `api_style` field. The base URL is the
service root, not an OpenAI URL; the adapter appends the native path. Tokens are
read only from the configured provider, never forwarded from the caller.

```bash
# Existing local LibreTranslate instance (start it separately):
work/translation-demo/venv/bin/python sdk/python/examples/translation/setup.py \
  --api-style libretranslate --api-base http://127.0.0.1:18682 --rule-model translate-libre
# DeepL: set TINGLY_TRANSLATION_API_KEY through your secret mechanism first.
work/translation-demo/venv/bin/python sdk/python/examples/translation/setup.py \
  --api-style deepl --api-base https://api-free.deepl.com --rule-model translate-deepl
```

Traditional services have no upstream model: setup stores an empty service
`model`, and neither native payload contains a fabricated model field.
LibreTranslate uses JSON `q/source/target/format=text` and an optional body API
key. DeepL uses `/v2/translate`, `text[]` and `DeepL-Auth-Key` authentication.
Local language packs, DeepL endpoint/account tier and provider limits still
apply. No paid cloud call is required by these examples or automated tests.

You can configure the existing tier tactic with a traditional service at T0 and
an LLM at T1. Only listed services are eligible for failover; adding a provider
does not authorize new fallback destinations. Completed batch items are not
replayed. Incompatible language variants are filtered before selection. Upstream
request errors, quota and rate limits retain the existing failover policy;
there is no automatic cross-service fallback on arbitrary 4xx responses.

## Model adaptation and limits

- `Translator.Translate(ctx, TextInput) -> TextOutput` is backend-neutral.
  `ModelTranslator` handles ordinary LLM and native MT semantics; traditional
  clients implement the same interface using native HTTP, not Chat wrappers.
- Only native MT overrides need catalog data: `tencent/Hy-MT2-1.8B` and
  `qwen-mt-flash`. Unknown ordinary model IDs use the LLM path. Qwen-MT itself
  remains mock-only; no hosted Qwen account was used.
- Existing provider fields, LB services and rules are reused. No new LB schema
  or provider extension bag is introduced; `api_style` distinguishes native
  HTTP protocols. Dedicated UI selection for the new styles remains future work.
- JSON, synchronous plain text: 1–32 items, 32 KiB per item, 256 KiB body.
  Native language variants are mapped explicitly and unsupported distinctions
  rejected. Generic LLM language ability is best-effort, not catalog-certified.
- Optional `detected_source_language` and `billed_characters` are returned per
  item only when actually reported. Tokens are recorded only when reported.
  Character counts are never converted to tokens; the existing token-oriented
  usage database does not yet persist native character billing or compute its cost.
- Basic rule balancing/failover is supported. Smart-routing partitions, HTML,
  glossary, files, streaming and new translation guardrail coverage remain out
  of scope. Python model generation is serialized; cancellation does not
  forcibly stop already running inference (128-token generation budget).

## Verification

```bash
go test ./internal/catalog ./internal/protocol/translation ./internal/protocolserver ./internal/client
# Build normal tb, then run the SDK's existing E2E harness including translation:
go build -o "$PWD/work/translation-demo/tingly-box" ./cli/tingly-box
TINGLY_TB_BIN="$PWD/work/translation-demo/tingly-box" \
  work/translation-demo/venv/bin/python -m unittest discover -s sdk/python/tests
```

SDK quota tests require the existing `task gen:py:quota` generation step.
After starting the real model server, an additional handler-level smoke check is:

```bash
TINGLY_TRANSLATION_LIVE_BASE_URL=http://127.0.0.1:18681 \
  go test ./internal/protocolserver -run '^TestTranslationLiveHunyuan$' -v -count=1
```

The live check verifies connectivity and output completeness, not translation
quality. See [the test record](../../../../.research/translation-smoke-test.md).
