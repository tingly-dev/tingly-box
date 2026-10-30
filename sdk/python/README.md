# tingly (Python SDK)

Turn a Python function into a tb provider, and call tb from Python. Zero
dependencies. Design notes: [`.design/python-sdk.md`](../../.design/python-sdk.md).

## Write a provider

```python
import tingly

@tingly.image("qwen-image-2.1")
def generate(prompt):
    return pipe(prompt).images[0]      # bytes or a PIL image

tingly.serve()                          # http://0.0.0.0:8765/v1
```

| decorator | serves | your function gets | returns |
|---|---|---|---|
| `openai_chat(model)` / `chat` | `/v1/chat/completions` | `messages` | `str` or a ChatCompletion dict |
| `openai_responses(model)` / `responses` | `/v1/responses` | `input` | `str` or a Response dict |
| `anthropic_message(model)` / `message` | `/v1/messages` | `messages` | `str` or a Message dict |
| `image(model)` | `/v1/images/generations` | `prompt` | `bytes`, a PIL image, or a list |
| `image_edit(model)` | `/v1/images/edits` | `prompt`, `images` (`list[bytes]`) | same as `image` |

Values arrive exactly as the caller sent them. Other body fields are passed
only if your function declares them: `def generate(prompt, size=None)` also
gets `size`, `model` works the same way, and `**kw` gets everything. Several
models can share one process; they're listed on `/v1/models` and routed by
`model`. Streaming clients work: the whole reply arrives as one chunk.

[`examples/image.py`](examples/image.py) is a runnable image provider with a
fake model.

## Script a model, run it against a real tb

Need a *specific interaction* — call a tool, get the result, call another,
hit a 529, then answer? Write it as a script and let `tingly.vmodel` start a
tb to serve it. No provider to register, no rule to create:

```python
from tingly import vmodel

flow = (vmodel.Script("read-edit")
        .tool("Read", {"file_path": "/tmp/a.go"}, say="Let me look.")
        .tool("Edit", {"file_path": "/tmp/a.go", "old_string": "foo", "new_string": "bar"})
        .error(529)                       # the upstream hiccups once
        .say("Done."))

with vmodel.Testbed(flow) as tb:          # starts a throwaway tb, writes the script
    reply = tb.messages(flow.model, "fix foo")      # Anthropic wire; tb.chat() is OpenAI
    # ...or point a real SDK/agent at it:
    #   Anthropic(base_url=tb.anthropic_base, api_key=tb.token)
    #   OpenAI(base_url=tb.openai_base,       api_key=tb.token)
```

`python examples/vmodel_flow.py` (or `task demo:vmodel` from the repo root)
runs the whole thing with a small agent loop. Step kinds: `.say()`, `.tool()`,
`.error(status)`, `.cut(mode)` (a stream that dies part-way), each taking
`usage=`, `stop_reason=`, `repeat=`; `Script(..., on_exhaust="clamp")` decides
what happens after the last step. `stop_reason=` takes `end_turn`, `tool_use`,
`max_tokens` or `stop_sequence` (mapped for OpenAI). The same script also
answers the OpenAI protocol (`tb.chat()`), but as its own independent run:
each protocol has its own place in the program, so a call on one never
consumes a step of the other. Within one protocol every caller shares the
place; re-adding the script (`tb.add(flow)`) restarts it.

- **Sessions isolate runs.** Callers of one model share its place in the
  program, so concurrent tests would consume each other's steps. `s = tb.session()`
  gives your test its own run: `s.messages(flow, ...)`, or hand any SDK/agent
  `s.model(flow)` (`"read-edit@<id>"`) as the model name. Re-adding the script
  restarts every session.
- **Needs a tb binary**: `TINGLY_TB_BIN`, `tb_bin=...`, or `tingly-box` on
  `PATH` (`go build -o tb ./cli/tingly-box` builds one).
- **Already running tb?** `Testbed.attach(flow)` writes into its config dir
  (`~/.tingly-box`, or `config_dir=` / `$TINGLY_CONFIG_DIR`) and removes the
  script on exit; `base_url` defaults to `http://localhost:12580`.
- **A script is just a file** `<config-dir>/vmodels/<id>.yaml`; `Testbed.add()`
  also takes the path of a hand-written one, and raises `ScriptError` with tb's
  own message if tb rejects it. Schema: [`.design/vmodel-script.md`](../../.design/vmodel-script.md).

### Register it in tb

- **Connect AI → Self-hosted → Custom endpoint**, OpenAI,
  `http://localhost:8765/v1`, no key. For images, add an `imagegen` rule
  pointing at it. For text, `openai_chat` is usually all you need: tb
  translates Anthropic and Responses clients into Chat.
- To receive Anthropic requests natively (`anthropic_message`), use **Dual
  endpoint** instead, Anthropic URL `http://localhost:8765`, and give it any
  placeholder key (e.g. `not-required`) — tb's Anthropic client refuses an
  empty one.

## Your own `Server`

`tingly.*` and `tingly.serve()` use a default `tingly.Server`. Create your
own for `.tb` (a client back into tb), several servers in one process, or
tests — same methods:

```python
from tingly import Server

srv = Server(tb_base_url="http://localhost:12580", tb_token="...")  # or TINGLY_BASE_URL / TINGLY_TOKEN

@srv.openai_chat("relay")
def relay(messages):
    return srv.tb.chat(model="claude-opus-4-8", messages=messages)

srv.run(port=8765)
```

More examples:

```bash
cd sdk/python
TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=<gateway token> python examples/relay.py   # all three text protocols
TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=<gateway token> python examples/fanout.py  # ask several models, merge
python examples/image.py
```

## Call tb

```python
from tingly import Client, text_of

tb = Client(base_url="http://localhost:12580", token="...")   # gateway token
print(text_of(tb.chat(model="gpt-4o", messages=[{"role": "user", "content": "hi"}])))
```

### Quota

Typed from tb's `openapi.json`. Generate the models once (needs `pydantic`,
e.g. `pip install -e '.[quota]'`):

```bash
task gen:py:quota
```

```python
tb = Client(base_url="http://localhost:12580", token="...", admin_token="...")  # admin_token: tb's UserToken, defaults to token
tb.quota_summary()
tb.list_quota()
tb.get_quota(provider_uuid)
```

## Tests

```bash
task gen:py:quota                      # from repo root, once: the quota tests need it
cd sdk/python && python -m unittest discover tests
task test:py:e2e                       # from repo root: builds tb, runs the end-to-end test against it
```
