# Translation through the Python SDK and ordinary tb

All translation examples live here. The model process is a Python SDK provider;
start the ordinary tb server and use its management API to configure routing.
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
`tingly.Server.openai_chat` on loopback port 18081. Several GB of disk and RAM
are needed. CPU inference works. `GET /v1/models` is available after loading.
Alternatively pass a downloaded snapshot directory with `--model`.

The served model ID defaults to `tencent/Hy-MT2-1.8B`, matching the capability
catalog. Keep that ID when registering the provider. Arbitrary upstream aliases
are currently unsupported: the provider/model binding mechanism is not yet
implemented, and no family is guessed from substrings. The public rule alias
can still be any name, independent of the upstream ID.

## Start normal tb and configure it

In a second terminal, use the normal CLI (an installed binary also works):

```bash
go run ./cli/tingly-box --config-dir "$PWD/work/translation-demo/tb" start \
  --no-daemon --host 127.0.0.1 --port 12580 --browser=false
```

Set `TINGLY_ADMIN_TOKEN` to that instance's UserToken and `TINGLY_TOKEN` to its
ModelToken using your normal secret mechanism. Do not commit either token.
The setup script creates one provider and one `custom` rule through the existing
management APIs, using only ordinary provider/model services:

```bash
export TINGLY_BASE_URL=http://127.0.0.1:12580
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

## Model adaptation and limits

- `internal/catalog/{hunyuan,qwen}.models.json` describes model translation
  semantics. The protocol adapter consumes the selected model's capability;
  load balancing only chooses ordinary provider/model services. No
  `translation_backend` routing field is used.
- Only the exact catalog IDs `tencent/Hy-MT2-1.8B` and `qwen-mt-flash` are
  currently supported. Qwen is a hosted native API adapter with mock coverage;
  no Qwen cloud live test is claimed or required by this local example.
- JSON, synchronous plain text: 1–32 items, 32 KiB per item, 256 KiB body.
  Unsupported fields and language variants are rejected. The language map
  lives in `internal/protocol/translation`; Qwen Traditional Chinese is omitted.
- Existing auth/team scope, provider clients and base-rule load balancing /
  failover are reused. Completed batch items are not replayed. Failure returns
  an indexed error; earlier items may already have consumed quota.
- Native API-key/no-key OpenAI serving only. Smart-routing partitions, OAuth,
  multi-field credentials, generic-chat fallback, HTML, glossary, files and
  streaming remain outside this version. No new guardrails coverage is claimed.
- Usage is recorded only when actually reported. No source detection result or
  confidence is fabricated. Generation is serialized; HTTP cancellation does
  not forcibly stop Python inference already in progress (128-token budget).

## Verification

```bash
go test ./internal/catalog ./internal/protocol/translation ./internal/protocolserver
# Build normal tb, then run the SDK's existing E2E harness including translation:
go build -o "$PWD/work/translation-demo/tingly-box" ./cli/tingly-box
TINGLY_TB_BIN="$PWD/work/translation-demo/tingly-box" \
  work/translation-demo/venv/bin/python -m unittest discover -s sdk/python/tests
```

SDK quota tests require the existing `task gen:py:quota` generation step.
After starting the real model server, an additional handler-level smoke check is:

```bash
TINGLY_TRANSLATION_LIVE_BASE_URL=http://127.0.0.1:18081 \
  go test ./internal/protocolserver -run '^TestTranslationLiveHunyuan$' -v -count=1
```

The live check verifies connectivity and output completeness, not translation
quality. See [the test record](../../../../.research/translation-smoke-test.md).
