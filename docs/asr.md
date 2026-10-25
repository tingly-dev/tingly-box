# Speech recognition

Open **Agent → Speech Recognition** to configure independent `asr` model rules,
upload an audio file or record from your microphone, and run transcription or
translation to English. Results can be downloaded as JSON, text or subtitles.
The playground and external SDK clients use the same model-authenticated API.

## Configure and call

Add an OpenAI-compatible API-key provider (for example, with API base
`https://api.openai.com/v1`). Add an active rule under Speech Recognition,
mapping a request model such as `listener` to a transcription model such as
`whisper-1` or `gpt-4o-transcribe`. If the upstream model list omits speech
models, enter the model ID manually.

```bash
curl http://localhost:12580/tingly/asr/v1/audio/transcriptions \
  -H "Authorization: Bearer $TINGLY_MODEL_TOKEN" \
  -F model=listener -F file=@speech.wav \
  -F response_format=verbose_json \
  -F 'timestamp_granularities[]=word' \
  -F 'timestamp_granularities[]=segment'
```

```python
from openai import OpenAI
import os

client = OpenAI(
    base_url="http://localhost:12580/tingly/asr/v1",
    api_key=os.environ["TINGLY_MODEL_TOKEN"],
)
with open("speech.wav", "rb") as audio:
    transcript = client.audio.transcriptions.create(
        model="listener", file=audio, response_format="json"
    )
print(transcript.text)
```

## API contract

- `POST /tingly/asr[/v1]/audio/transcriptions`: transcribe in the source language.
- `POST /tingly/asr[/v1]/audio/translations`: translate speech to English, when
  supported by the upstream model (for example, Whisper). This is not arbitrary
  target-language translation.
- Both accept multipart form data with exactly one `model` and one non-empty
  `file`. The gateway substitutes the routed model and preserves other fields,
  including repeated `timestamp_granularities[]`, `include[]`, prompts, language,
  temperature, chunking strategy and vendor extensions. The original filename,
  MIME type and audio bytes are retained.
- Supported response format values: `json`, `verbose_json`, `text`, `srt`, `vtt`,
  `diarized_json`. Each model decides which formats, language hints, word/segment
  timestamps, speaker diarization and streaming it supports. The playground
  offers JSON, verbose JSON, text, SRT and VTT; other options remain available
  through the API.
- `stream=true` forwards SSE transcription events progressively, including their
  optional usage. The playground displays partial text and downloads the final
  transcript as a text file. Streaming files is distinct from a realtime
  microphone/WebSocket session.
- Audio uploads are limited to **25 MiB**, with a **32 MiB** total multipart
  request limit. Uploads above 4 MiB spill to temporary disk storage. Files and
  writer resources are removed/closed after completion, upstream errors,
  malformed/oversized requests and cancellation.
- The `openai` and `team` scenarios expose both endpoints with their own rules.
  Team/model key permissions apply unchanged. The `asr` and `tts` scenarios have
  separate rules and transport gates: each accepts only its respective Audio API.
- The common audio transport retains provider auth/proxy/timeouts, rule headers,
  dual endpoint resolution, health/affinity/load balancing, tracing, request
  counts, latency, status and team/user attribution. Upstream JSON/SSE token
  usage is recorded when available; duration-only or text responses record zero
  tokens, without inventing token counts or monetary costs.

## Recording and compatibility

Browser recording requests microphone access only after pressing **Record**.
It requires HTTPS or localhost and MediaRecorder support. It stops after ten
minutes or on reaching the upload size limit, and releases microphone tracks
when stopped or when leaving the page. Choose the matching provider/model for
the browser's recording format (usually WebM/Opus, MP4 or Ogg). Upload remains
available when browser recording is unavailable.

This feature uses the OpenAI-compatible Audio API. Proprietary speech protocols,
agent OAuth backends, virtual chat models, realtime/WebSocket speech, long-file
batch jobs and local audio transcoding are outside this implementation. No
upstream vendor credentials are required by the local mock-upstream tests;
live provider compatibility still requires that provider's key and model.

Tracks [TEP #2056](https://github.com/tingly-dev/tingly-box/issues/2056). The
shared audio transport and TTS are described in [Text to speech](./tts.md).
