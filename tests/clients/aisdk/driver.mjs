#!/usr/bin/env node
// Subprocess client driver for the tingly-box protocol matrix harness, built
// on the AI SDK by Vercel (`ai` + @ai-sdk/anthropic + @ai-sdk/openai).
//
// Speaks the JSON-over-stdin/stdout contract defined in
// internal/protocoltest/client_subprocess.go. Unlike the plain `node` driver
// (official Anthropic/OpenAI SDKs), this exercises the gateway through the
// AI SDK's own abstraction layer: its providers parse responses with zod
// schemas and normalize streams through a unified fullStream pipeline, so a
// response that official SDKs tolerate can still fail here.
//
// Gateway/API errors are reported in-band via the "error" field; a non-zero
// exit code means the driver itself is broken.

import { generateText, streamText, jsonSchema, tool, APICallError } from "ai";
import { createAnthropic } from "@ai-sdk/anthropic";
import { createOpenAI } from "@ai-sdk/openai";

// The AI SDK rejects tool calls for tools that were not declared in the
// request (NoSuchToolError), so the scenario tool is always declared.
// It has no execute function: calls are returned to the "client", which is
// exactly what the tool_use scenarios assert on.
const tools = {
  get_weather: tool({
    description: "Get the current weather for a location",
    inputSchema: jsonSchema({
      type: "object",
      properties: {
        location: { type: "string" },
        unit: { type: "string" },
      },
    }),
  }),
};

function buildModel(req) {
  const common = { apiKey: req.api_key };
  switch (req.source) {
    case "anthropic_v1":
    case "anthropic_beta": {
      // The gateway selects the Beta protocol via the ?beta=true query param;
      // the AI SDK has no per-request query option, so wrap fetch.
      const fetchImpl =
        req.source === "anthropic_beta"
          ? (input, init) => {
              const url = new URL(typeof input === "string" ? input : input.url);
              url.searchParams.set("beta", "true");
              return fetch(url, init);
            }
          : fetch;
      const anthropic = createAnthropic({
        ...common,
        baseURL: req.base_url + "/tingly/anthropic/v1",
        fetch: fetchImpl,
      });
      return anthropic(req.model);
    }
    case "openai_chat": {
      const openai = createOpenAI({
        ...common,
        baseURL: req.base_url + "/tingly/openai/v1",
      });
      return openai.chat(req.model);
    }
    case "openai_responses": {
      const openai = createOpenAI({
        ...common,
        baseURL: req.base_url + "/tingly/openai/v1",
      });
      return openai.responses(req.model);
    }
    default:
      throw new Error(`unsupported source protocol: ${req.source}`);
  }
}

function mapUsage(usage) {
  if (!usage) return null;
  return {
    input_tokens: usage.inputTokens ?? 0,
    output_tokens: usage.outputTokens ?? 0,
  };
}

function mapToolCalls(toolCalls) {
  return (toolCalls ?? []).map((tc) => ({
    id: tc.toolCallId ?? "",
    name: tc.toolName ?? "",
    arguments: JSON.stringify(tc.input ?? {}),
  }));
}

async function runNonStream(req, model) {
  const result = await generateText({
    model,
    prompt: req.prompt,
    tools,
    maxRetries: 0,
  });
  return {
    http_status: 200,
    role: "assistant",
    content: result.text ?? "",
    model: result.response?.modelId ?? "",
    finish_reason: result.finishReason ?? "",
    thinking: result.reasoningText ?? "",
    tool_calls: mapToolCalls(result.toolCalls),
    usage: mapUsage(result.usage),
    raw_body: JSON.stringify(result.response?.body ?? { text: result.text }),
  };
}

async function runStream(req, model) {
  const result = streamText({
    model,
    prompt: req.prompt,
    tools,
    maxRetries: 0,
    // Errors are surfaced as fullStream 'error' parts below; the default
    // onError would also print them, failing the driver on expected errors.
    onError: () => {},
  });

  let count = 0;
  let content = "";
  let thinking = "";
  let finishReason = "";
  let usage = null;
  let modelId = "";
  const toolCalls = [];
  let streamError = null;

  // Accumulate from fullStream directly instead of awaiting result promises,
  // which reject on mid-stream cuts — the partial message is the point.
  for await (const part of result.fullStream) {
    count++;
    switch (part.type) {
      case "text-delta":
        content += part.text ?? "";
        break;
      case "reasoning-delta":
        thinking += part.text ?? "";
        break;
      case "tool-call":
        toolCalls.push({
          id: part.toolCallId ?? "",
          name: part.toolName ?? "",
          arguments: JSON.stringify(part.input ?? {}),
        });
        break;
      case "finish":
        finishReason = part.finishReason ?? "";
        usage = mapUsage(part.totalUsage);
        break;
      case "error":
        streamError = part.error;
        break;
    }
  }

  if (streamError && content === "" && thinking === "" && toolCalls.length === 0) {
    // Failed before any content: report as an API error (pre-content 4xx/5xx).
    // Errors after content started (mid-stream cut) keep the partial message.
    return apiErrorResponse(streamError);
  }

  try {
    modelId = (await result.response)?.modelId ?? "";
  } catch {
    // Mid-stream cut: response metadata may be unavailable.
  }

  return {
    http_status: 200,
    role: "assistant",
    content,
    model: modelId,
    finish_reason: finishReason,
    thinking,
    tool_calls: toolCalls,
    usage,
    stream_event_count: count,
    raw_body: content,
  };
}

// unwrapError digs through AI SDK wrapper errors (RetryError holds the
// underlying APICallError in lastError / errors).
function unwrapError(e) {
  if (e?.lastError) return unwrapError(e.lastError);
  if (Array.isArray(e?.errors) && e.errors.length > 0) return unwrapError(e.errors[e.errors.length - 1]);
  return e;
}

function apiErrorResponse(err) {
  const e = unwrapError(err);
  const status = APICallError.isInstance(e) ? e.statusCode ?? 0 : (e?.statusCode ?? 0);
  const raw = APICallError.isInstance(e)
    ? e.responseBody ?? String(e.message ?? e)
    : String(e?.message ?? e);
  return {
    http_status: status,
    raw_body: raw,
    error: { status, type: e?.name ?? "APICallError", message: String(e?.message ?? e) },
  };
}

function isAPIError(err) {
  const e = unwrapError(err);
  return APICallError.isInstance(e) || (typeof e?.statusCode === "number" && e.statusCode >= 400);
}

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  return Buffer.concat(chunks).toString("utf8");
}

async function main() {
  const req = JSON.parse(await readStdin());
  if (req.version !== 1) {
    throw new Error(`unsupported driver protocol version: ${req.version}`);
  }

  const model = buildModel(req);
  let resp;
  try {
    resp = req.stream ? await runStream(req, model) : await runNonStream(req, model);
  } catch (e) {
    if (!isAPIError(e)) throw e;
    resp = apiErrorResponse(e);
  }
  process.stdout.write(JSON.stringify(resp) + "\n");
}

main().catch((e) => {
  console.error(e?.stack ?? String(e));
  process.exit(1);
});
