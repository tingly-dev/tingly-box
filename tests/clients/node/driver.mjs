#!/usr/bin/env node
// Subprocess client driver for the tingly-box protocol matrix harness.
//
// Speaks the JSON-over-stdin/stdout contract defined in
// internal/protocoltest/client_subprocess.go: reads one request object from
// stdin, sends it through the gateway using the REAL Node SDKs
// (@anthropic-ai/sdk, openai), and writes one normalized response object to
// stdout.
//
// Gateway/API errors are reported in-band via the "error" field; a non-zero
// exit code means the driver itself is broken.

import Anthropic from "@anthropic-ai/sdk";
import OpenAI from "openai";

function toolCall(id, name, args) {
  return { id, name, arguments: args };
}

// ── Anthropic (v1 + beta) ────────────────────────────────────────────────────

function normalizeAnthropicMessage(msg) {
  let content = "";
  let thinking = "";
  const toolCalls = [];
  for (const block of msg.content ?? []) {
    if (block.type === "text") content += block.text;
    else if (block.type === "thinking") thinking += block.thinking;
    else if (block.type === "tool_use")
      toolCalls.push(toolCall(block.id, block.name, JSON.stringify(block.input ?? {})));
  }
  return {
    http_status: 200,
    role: msg.role,
    content,
    model: msg.model ?? "",
    finish_reason: msg.stop_reason ?? "",
    thinking,
    tool_calls: toolCalls,
    usage: msg.usage
      ? { input_tokens: msg.usage.input_tokens ?? 0, output_tokens: msg.usage.output_tokens ?? 0 }
      : null,
    raw_body: JSON.stringify(msg),
  };
}

async function runAnthropic(req, beta) {
  const client = new Anthropic({
    baseURL: req.base_url + "/tingly/anthropic",
    apiKey: req.api_key,
    maxRetries: 0,
    timeout: req.timeout_ms ?? 30000,
  });
  const svc = beta ? client.beta.messages : client.messages;
  const params = {
    model: req.model,
    max_tokens: 1024,
    messages: [{ role: "user", content: req.prompt }],
  };
  // The gateway selects the Beta protocol via the ?beta=true query param.
  const opts = beta ? { query: { beta: "true" } } : {};

  if (!req.stream) {
    return normalizeAnthropicMessage(await svc.create(params, opts));
  }

  const stream = svc.stream(params, opts);
  let count = 0;
  try {
    for await (const _event of stream) count++;
    const msg = await stream.finalMessage();
    const resp = normalizeAnthropicMessage(msg);
    resp.stream_event_count = count;
    return resp;
  } catch (e) {
    if (isAPIError(e)) return apiErrorResponse(e);
    // Mid-stream error event (e.g. a truncated upstream): the SDK raised; the
    // turn was not completed. Report it in-band rather than crashing.
    return { http_status: 200, stream_event_count: count, raw_body: String(e?.message ?? e),
      error: { status: 0, type: e?.name ?? "StreamError", message: String(e?.message ?? e) } };
  }
}

// ── OpenAI Chat Completions ──────────────────────────────────────────────────

function normalizeChatCompletion(resp) {
  const choice = resp.choices[0];
  const msg = choice.message;
  const toolCalls = (msg.tool_calls ?? []).map((tc) =>
    toolCall(tc.id, tc.function.name, tc.function.arguments),
  );
  return {
    http_status: 200,
    role: msg.role,
    content: msg.content ?? "",
    model: resp.model ?? "",
    finish_reason: choice.finish_reason ?? "",
    thinking: "",
    tool_calls: toolCalls,
    usage: resp.usage
      ? { input_tokens: resp.usage.prompt_tokens ?? 0, output_tokens: resp.usage.completion_tokens ?? 0 }
      : null,
    raw_body: JSON.stringify(resp),
  };
}

async function runOpenAIChat(req, client) {
  const params = {
    model: req.model,
    messages: [{ role: "user", content: req.prompt }],
  };
  if (!req.stream) {
    return normalizeChatCompletion(await client.chat.completions.create(params));
  }

  const stream = await client.chat.completions.create({ ...params, stream: true });
  let count = 0;
  let role = "";
  let content = "";
  let finish = "";
  let model = "";
  const tools = new Map();
  for await (const chunk of stream) {
    count++;
    model = chunk.model || model;
    const choice = chunk.choices?.[0];
    if (!choice) continue;
    const delta = choice.delta ?? {};
    role = delta.role || role;
    content += delta.content ?? "";
    for (const tc of delta.tool_calls ?? []) {
      const slot = tools.get(tc.index) ?? { id: "", name: "", arguments: "" };
      if (tc.id) slot.id = tc.id;
      if (tc.function?.name) slot.name = tc.function.name;
      slot.arguments += tc.function?.arguments ?? "";
      tools.set(tc.index, slot);
    }
    finish = choice.finish_reason || finish;
  }
  const toolCalls = [...tools.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([, t]) => toolCall(t.id, t.name, t.arguments));
  return {
    http_status: 200,
    role,
    content,
    model,
    finish_reason: finish,
    thinking: "",
    tool_calls: toolCalls,
    usage: null,
    stream_event_count: count,
    raw_body: content,
  };
}

// ── OpenAI Responses ─────────────────────────────────────────────────────────

function normalizeResponse(resp) {
  let content = "";
  const toolCalls = [];
  for (const item of resp.output ?? []) {
    if (item.type === "message") {
      for (const part of item.content ?? []) {
        if (part.type === "output_text") content += part.text;
      }
    } else if (item.type === "function_call") {
      toolCalls.push(toolCall(item.call_id ?? item.id ?? "", item.name, item.arguments ?? ""));
    }
  }
  return {
    http_status: 200,
    role: "assistant",
    content,
    model: resp.model ?? "",
    finish_reason: resp.status ?? "",
    thinking: "",
    tool_calls: toolCalls,
    usage: resp.usage
      ? { input_tokens: resp.usage.input_tokens ?? 0, output_tokens: resp.usage.output_tokens ?? 0 }
      : null,
    raw_body: JSON.stringify(resp),
  };
}

async function runOpenAIResponses(req, client) {
  const params = { model: req.model, input: req.prompt };
  if (!req.stream) {
    return normalizeResponse(await client.responses.create(params));
  }

  const stream = await client.responses.create({ ...params, stream: true });
  let count = 0;
  let final = null;
  let deltaText = "";
  for await (const event of stream) {
    count++;
    if (event.type === "response.completed" || event.type === "response.incomplete") {
      final = event.response;
    } else if (event.type === "response.output_text.delta") {
      deltaText += event.delta ?? "";
    }
  }
  let resp;
  if (final) {
    resp = normalizeResponse(final);
  } else {
    // Stream cut before a terminal event (midstream-close).
    resp = {
      http_status: 200,
      role: "assistant",
      content: deltaText,
      model: "",
      finish_reason: "",
      thinking: "",
      tool_calls: [],
      usage: null,
      raw_body: deltaText,
    };
  }
  resp.stream_event_count = count;
  return resp;
}

// ── shared ───────────────────────────────────────────────────────────────────

function apiErrorResponse(e) {
  const raw = e.error !== undefined ? JSON.stringify(e.error) : String(e.message ?? e);
  return {
    http_status: e.status ?? 0,
    raw_body: raw,
    error: { status: e.status ?? 0, type: e.name ?? "APIError", message: String(e.message ?? e) },
  };
}

function isAPIError(e) {
  return typeof e?.status === "number" && e.status >= 400;
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

  let resp;
  try {
    if (req.source === "anthropic_v1") {
      resp = await runAnthropic(req, false);
    } else if (req.source === "anthropic_beta") {
      resp = await runAnthropic(req, true);
    } else if (req.source === "openai_chat" || req.source === "openai_responses") {
      const client = new OpenAI({
        baseURL: req.base_url + "/tingly/openai/v1",
        apiKey: req.api_key,
        maxRetries: 0,
        timeout: req.timeout_ms ?? 30000,
      });
      resp =
        req.source === "openai_chat"
          ? await runOpenAIChat(req, client)
          : await runOpenAIResponses(req, client);
    } else {
      throw new Error(`unsupported source protocol: ${req.source}`);
    }
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
