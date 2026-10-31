# Text to speech

Tingly Box exposes the OpenAI Audio Speech API through a dedicated `tts`
scenario. Open **Agent → Text to Speech** to configure model rules, generate
speech, listen to it, and download the audio. The playground supports voice
names, speed (0.25–4), output format and optional voice instructions. It uses
these same gateway rules, rather than calling a provider directly. Cancel
stops the request; downloaded speech is generated audio.

## Configure a provider and model

1. Add an OpenAI-compatible **API-key** provider with its versioned API base,
   for example `https://api.openai.com/v1`, and its provider key.
2. Add an active rule under the **Text to Speech** scenario. Choose a request
   model alias (for example `speaker`) and map it to the upstream speech model
   (for example `gpt-4o-mini-tts`). You can enter a model manually when the
   provider's model list does not include audio models.
3. Use the model API token shown in the connection card to call the gateway.
   This is distinct from both the management token and the provider key.

```bash
curl http://localhost:12580/tingly/tts/v1/audio/speech \
  -H "Authorization: Bearer $TINGLY_MODEL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"model":"speaker","input":"Hello from Tingly Box","voice":"alloy","response_format":"mp3"}' \
  --output speech.mp3
```

```python
from openai import OpenAI
import os

client = OpenAI(
    base_url="http://localhost:12580/tingly/tts/v1",
    api_key=os.environ["TINGLY_MODEL_TOKEN"],
)
with client.audio.speech.with_streaming_response.create(
    model="speaker", input="Hello from Tingly Box", voice="alloy"
) as response:
    response.stream_to_file("speech.mp3")
```

## API behavior

- `POST /tingly/tts[/v1]/audio/speech` accepts the standard JSON contract:
  `model`, `input`, `voice` (name or `{ "id": "voice_..." }`), `speed`,
  `response_format`, `instructions`, and `stream_format`.
- Formats: `mp3`, `opus`, `aac`, `flac`, `wav`, `pcm`. PCM is downloadable but
  cannot be played directly by the browser. Codec playback varies by browser.
- Binary audio is streamed immediately. `stream_format: "sse"` forwards audio
  events as they arrive. Model-specific capabilities are validated upstream;
  the gateway preserves additional vendor parameters and rewrites only `model`.
- Speech request bodies are limited to 2 MiB. Text length, available voices,
  instructions and codec support depend on the selected provider/model.
- The `openai` and `team` scenarios also expose this endpoint using their own
  rules. Team keys remain restricted to their team and model scope. The `tts`
  scenario does not accept chat requests or unrelated protocol surfaces.
- Provider proxy settings, timeout, rule headers, health/affinity/load balancing,
  model authentication, tracing and usage logs use the existing gateway stack.
  There are no automatic retries after audio has been sent. Upstream HTTP errors
  retain their status; interrupted streams terminate instead of appending JSON.
- Request counts, latency, success/failure and team/user attribution are recorded.
  Token usage is recorded when the upstream reports it in JSON/SSE. Binary audio
  responses normally have no usage object and record zero tokens; audio duration,
  characters and monetary cost are not inferred from bytes or text.

This implementation supports providers exposing the OpenAI-compatible Audio API,
including self-hosted compatible services. Agent OAuth backends (Codex/Kimi),
virtual chat models, and proprietary vendor speech APIs are not audio providers
through this endpoint. Realtime/WebSocket speech and voice cloning/voice
management are separate APIs and are not implemented here.

Tracks [TEP #2056](https://github.com/tingly-dev/tingly-box/issues/2056).
