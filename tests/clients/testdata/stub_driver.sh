#!/bin/sh
# Stub driver for the subprocess client contract test
# (internal/protocoltest/client_subprocess_test.go). Consumes the request from
# stdin and replies with a fixed, fully-populated response so the Go-side JSON
# plumbing is verified on every PR without Python/Node installed.
cat > /dev/null
printf '%s\n' '{
  "http_status": 200,
  "role": "assistant",
  "content": "The capital of France is Paris.",
  "model": "stub-model",
  "finish_reason": "stop",
  "thinking": "pondering",
  "tool_calls": [{"id": "call_1", "name": "get_weather", "arguments": "{\"location\":\"Paris\"}"}],
  "usage": {"input_tokens": 10, "output_tokens": 8},
  "stream_event_count": 4,
  "stream_completed": false,
  "stream_error": {"type": "StreamError", "message": "upstream truncated"},
  "raw_body": "stub raw body"
}'
