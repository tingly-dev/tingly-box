#!/usr/bin/env node
// Subprocess client driver for the tingly-box protocol matrix harness — a
// faithful port of the OpenAI Codex CLI's Responses-API client.
//
// Speaks the JSON-over-stdin/stdout contract defined in
// internal/protocoltest/client_subprocess.go: reads one request object from
// stdin, sends it through the gateway shaped EXACTLY like the real Codex client
// (openai/codex, codex-rs core/src/client.rs + client_common.rs), and writes
// one normalized response object to stdout.
//
// Unlike the node/python/aisdk drivers — which send the SDK's minimal
// `{model, input}` Responses call — this driver reconstructs Codex's full
// request (instructions, reasoning, include=reasoning.encrypted_content,
// store=false, prompt_cache_key, tool_choice=auto, parallel_tool_calls,
// text.verbosity, the shell function tool + the apply_patch custom/freeform
// tool, and Codex's identity headers) and accumulates the SSE with a
// Codex-style ResponseEvent state machine. Codex hand-rolls its own HTTP (no
// SDK), so this driver uses Node's built-in fetch — no npm dependencies.
//
// It only speaks openai_responses (Codex's built-in openai provider uses
// wire_api=responses); the Go-side driver restricts Supports() accordingly.
//
// Gateway/API errors are reported in-band via the "error" field; a non-zero
// exit code means the driver itself is broken.
//
// Wire reference & rationale: .design/harness-codex-client.md.

// A fixed conversation UUID: Codex uses it for prompt_cache_key and the
// conversation_id/session_id headers, and all three must agree. Fixed (not
// random) keeps requests deterministic.
const CONVERSATION_ID = "00000000-0000-4000-8000-000000000001";
const CODEX_VERSION = "0.46.0";

// A compact stand-in for Codex's multi-KB BASE_INSTRUCTIONS. The exact prose is
// irrelevant to the gateway — what matters is that `instructions` carries the
// system prompt (not the user turn).
const BASE_INSTRUCTIONS =
  "You are Codex, based on GPT-5. You are running as a coding agent in the " +
  "Codex CLI on a user's computer. Use the tools available to you to assist " +
  "the user with their coding tasks.";

// The synthetic environment-context message Prompt::get_formatted_input
// prepends ahead of the real turn.
const ENVIRONMENT_CONTEXT =
  "<environment_context>\n" +
  "  <cwd>/repo</cwd>\n" +
  "  <approval_policy>on-request</approval_policy>\n" +
  "  <sandbox_mode>workspace-write</sandbox_mode>\n" +
  "</environment_context>";

// A compact lark grammar standing in for Codex's APPLY_PATCH_LARK_GRAMMAR.
const APPLY_PATCH_GRAMMAR =
  'start: begin_patch hunk+ end_patch\n' +
  'begin_patch: "*** Begin Patch" NEWLINE\n' +
  'end_patch: "*** End Patch" NEWLINE?\n' +
  "hunk: /[^\\n]+/ NEWLINE\n" +
  "%import common.NEWLINE";

// Codex's two representative tools: the `shell` function tool (flat
// Responses-API shape) and the `apply_patch` custom/freeform tool (serialized
// as type:"custom" with a lark grammar — the FreeformTool variant a generic SDK
// Responses call never sends).
function codexTools() {
  return [
    {
      type: "function",
      name: "shell",
      description: "Runs a shell command and returns its output.",
      strict: false,
      parameters: {
        type: "object",
        properties: {
          command: { type: "array", items: { type: "string" } },
          workdir: { type: "string" },
          timeout_ms: { type: "number" },
          with_escalated_permissions: { type: "boolean" },
          justification: { type: "string" },
        },
        required: ["command"],
        additionalProperties: false,
      },
    },
    {
      type: "custom",
      name: "apply_patch",
      description:
        "Use the `apply_patch` tool to edit files. This is a FREEFORM tool, so do not wrap the patch in JSON.",
      format: { type: "grammar", syntax: "lark", definition: APPLY_PATCH_GRAMMAR },
    },
  ];
}

// buildRequestBody constructs the Codex ResponsesApiRequest payload. The
// scenario controls the mock response, not the request, so the body varies only
// by (model, streaming).
function buildRequestBody(req) {
  return {
    model: req.model,
    instructions: BASE_INSTRUCTIONS,
    // get_formatted_input prepends synthetic user/input_text messages
    // (environment context, then the real turn).
    input: [
      { type: "message", role: "user", content: [{ type: "input_text", text: ENVIRONMENT_CONTEXT }] },
      { type: "message", role: "user", content: [{ type: "input_text", text: req.prompt }] },
    ],
    tools: codexTools(),
    tool_choice: "auto",
    parallel_tool_calls: false,
    // Some only for reasoning-capable models (gpt-5 / o-series) — Codex's target
    // family — so include the reasoning block and its companion include entry.
    reasoning: { effort: "medium", summary: "auto" },
    store: false,
    stream: !!req.stream,
    include: ["reasoning.encrypted_content"],
    prompt_cache_key: CONVERSATION_ID,
    text: { verbosity: "medium" },
  };
}

// codexHeaders applies Codex's identity header set (client.rs +
// default_client.rs + model_provider_info.rs).
function codexHeaders(req) {
  const headers = {
    "Content-Type": "application/json",
    Authorization: "Bearer " + req.api_key,
    "OpenAI-Beta": "responses=experimental",
    conversation_id: CONVERSATION_ID,
    session_id: CONVERSATION_ID,
    originator: "codex_cli_rs",
    version: CODEX_VERSION,
    "User-Agent": `codex_cli_rs/${CODEX_VERSION} (linux; x86_64) harness`,
  };
  if (req.stream) headers["Accept"] = "text/event-stream";
  return headers;
}

// ── response accumulation ─────────────────────────────────────────────────────

// normalizeResponse folds a full Responses object (non-streaming, or the
// terminal response.completed payload) into the driver's output shape.
function normalizeResponse(resp) {
  let content = "";
  let thinking = "";
  const toolCalls = [];
  for (const item of resp.output ?? []) {
    if (item.type === "message") {
      for (const part of item.content ?? []) {
        if (part.type === "output_text") content += part.text;
      }
    } else if (item.type === "function_call") {
      toolCalls.push({
        id: item.call_id ?? item.id ?? "",
        name: item.name,
        arguments: item.arguments ?? "",
      });
    } else if (item.type === "reasoning") {
      for (const part of item.summary ?? []) thinking += part.text ?? "";
    }
  }
  return {
    http_status: 200,
    role: "assistant",
    content,
    model: resp.model ?? "",
    finish_reason: resp.status ?? "",
    thinking,
    tool_calls: toolCalls,
    usage: resp.usage
      ? { input_tokens: resp.usage.input_tokens ?? 0, output_tokens: resp.usage.output_tokens ?? 0 }
      : null,
    raw_body: JSON.stringify(resp),
  };
}

// A Codex-style SSE accumulator (mirrors the client.rs ResponseEvent loop).
// Real content arrives as whole items on response.output_item.done and in the
// terminal response.completed/incomplete output[] array; text/reasoning deltas
// are for live UI; the terminal event carries status + usage. Robust to the
// gateway emitting either item-done frames or only the terminal output[].
class CodexAccumulator {
  constructor() {
    this.content = "";
    this.thinking = "";
    this.toolCalls = [];
    this.toolIndex = new Map(); // id/call_id -> index
    this.model = "";
    this.status = "";
    this.usage = null;
    this.failed = "";
  }

  toolSlot(callId, itemId) {
    for (const key of [callId, itemId]) {
      if (key && this.toolIndex.has(key)) return this.toolIndex.get(key);
    }
    const idx = this.toolCalls.length;
    this.toolCalls.push({ id: callId ?? "", name: "", arguments: "" });
    if (callId) this.toolIndex.set(callId, idx);
    if (itemId) this.toolIndex.set(itemId, idx);
    return idx;
  }

  ingestItem(item, done) {
    if (!item || typeof item !== "object") return;
    if (item.type === "message") {
      if (!done) return;
      let text = "";
      for (const part of item.content ?? []) {
        if (part.type === "output_text") text += part.text ?? "";
      }
      // Prefer streamed deltas; fall back to terminal item text when no deltas
      // were emitted (e.g. gateway sends only the terminal output[]).
      if (this.content.length === 0) this.content += text;
    } else if (item.type === "function_call") {
      const idx = this.toolSlot(item.call_id ?? item.id ?? "", item.id ?? "");
      if (item.name) this.toolCalls[idx].name = item.name;
      if (item.call_id) this.toolCalls[idx].id = item.call_id;
      if (item.arguments) this.toolCalls[idx].arguments = item.arguments;
    } else if (item.type === "reasoning") {
      for (const part of item.summary ?? []) this.thinking += part.text ?? "";
    }
  }

  ingestTerminal(resp) {
    if (!resp || typeof resp !== "object") return;
    if (resp.status) this.status = resp.status;
    if (resp.model) this.model = resp.model;
    if (resp.usage) {
      this.usage = {
        input_tokens: resp.usage.input_tokens ?? 0,
        output_tokens: resp.usage.output_tokens ?? 0,
      };
    }
    for (const item of resp.output ?? []) this.ingestItem(item, true);
  }

  handle(ev) {
    switch (ev.type) {
      case "response.output_text.delta":
        this.content += ev.delta ?? "";
        break;
      case "response.reasoning_summary_text.delta":
      case "response.reasoning_text.delta":
        this.thinking += ev.delta ?? "";
        break;
      case "response.output_item.added":
        this.ingestItem(ev.item, false);
        break;
      case "response.function_call_arguments.delta": {
        const itemId = ev.item_id ?? "";
        if (itemId && ev.delta) {
          const idx = this.toolIndex.has(itemId) ? this.toolIndex.get(itemId) : this.toolSlot("", itemId);
          this.toolCalls[idx].arguments += ev.delta;
        }
        break;
      }
      case "response.output_item.done":
        this.ingestItem(ev.item, true);
        break;
      case "response.completed":
      case "response.incomplete":
        this.ingestTerminal(ev.response);
        break;
      case "response.failed":
        this.failed = ev.response?.error?.message ?? "stream failed";
        break;
      default:
        // no-op arms (output_text.done, content_part.done, in_progress, …)
        break;
    }
  }

  result(count) {
    const resp = {
      http_status: 200,
      role: "assistant",
      content: this.content,
      model: this.model,
      finish_reason: this.status,
      thinking: this.thinking,
      tool_calls: this.toolCalls,
      usage: this.usage,
      stream_event_count: count,
      raw_body: this.content,
    };
    // A mid-stream response.failed / cut with nothing accumulated: surface the
    // message rather than masquerading empty content as success.
    if (this.failed && !this.content && this.toolCalls.length === 0) {
      resp.raw_body = this.failed;
      resp.error = { status: 0, type: "StreamFailed", message: this.failed };
    }
    return resp;
  }
}

// Parse an SSE byte stream into "data:" JSON payloads, feeding each to the
// accumulator. Mirrors codex's line-based SSE reader.
async function accumulateStream(body, acc) {
  let count = 0;
  let buf = "";
  const decoder = new TextDecoder();
  for await (const chunk of body) {
    buf += decoder.decode(chunk, { stream: true });
    let nl;
    while ((nl = buf.indexOf("\n")) >= 0) {
      const line = buf.slice(0, nl).replace(/\r$/, "");
      buf = buf.slice(nl + 1);
      const data = parseSSEData(line);
      if (data === null || data === "[DONE]") continue;
      count++;
      let ev;
      try {
        ev = JSON.parse(data);
      } catch {
        continue;
      }
      acc.handle(ev);
    }
  }
  return count;
}

function parseSSEData(line) {
  if (!line.startsWith("data:")) return null;
  return line.slice(5).trimStart();
}

// ── error mapping ─────────────────────────────────────────────────────────────

function apiErrorResponse(status, rawBody) {
  let type = "APIError";
  let message = rawBody;
  try {
    const parsed = JSON.parse(rawBody);
    if (parsed?.error) {
      type = parsed.error.type ?? type;
      message = parsed.error.message ?? message;
    }
  } catch {
    // non-JSON error body; keep raw
  }
  return {
    http_status: status,
    raw_body: rawBody,
    error: { status, type, message: String(message) },
  };
}

// ── main ──────────────────────────────────────────────────────────────────────

async function runCodexResponses(req) {
  const url = req.base_url + "/tingly/openai/v1/responses";
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), req.timeout_ms ?? 30000);
  try {
    const resp = await fetch(url, {
      method: "POST",
      headers: codexHeaders(req),
      body: JSON.stringify(buildRequestBody(req)),
      signal: controller.signal,
    });

    if (resp.status !== 200) {
      return apiErrorResponse(resp.status, await resp.text());
    }

    if (req.stream) {
      const acc = new CodexAccumulator();
      const count = await accumulateStream(resp.body, acc);
      return acc.result(count);
    }

    return normalizeResponse(await resp.json());
  } finally {
    clearTimeout(timer);
  }
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
  if (req.source !== "openai_responses") {
    throw new Error(`codex driver only speaks openai_responses, got: ${req.source}`);
  }
  const resp = await runCodexResponses(req);
  process.stdout.write(JSON.stringify(resp) + "\n");
}

main().catch((e) => {
  console.error(e?.stack ?? String(e));
  process.exit(1);
});
