// Probe Types
// Note: These are custom types not in the codegen schema

export type ProbeTargetType = 'rule' | 'provider' | 'provider_config';

// Concrete client-side wire protocol (brand-first labels in the UI: OpenAI
// Chat / OpenAI Responses / Anthropic). No "auto" value — the panel always
// speaks a concrete protocol, defaulting to the target's primary one.
export type ProbeProtocol = 'openai_chat' | 'openai_responses' | 'anthropic_v1';

// Extended-thinking effort ladder (subset of the backend protocol thinking
// ladder, mirroring the rule flag's thinking_effort options — see
// .design/rule-flags.md). Orthogonal to the stream/tool axes — composes with
// all four combinations. '' (absent) == 'none' == no thinking param sent.
export type ProbeThinking = 'none' | 'low' | 'medium' | 'high' | 'max';

// Vision channel: attach the canonical probe image (backend
// internal/protocol/vision — a 256×256 red PNG + "what color?" prompt) in the
// user message or as a synthetic tool-result turn. A vision-capable route
// answers "red"; anything else reveals a drop or corruption along the path.
// 'none' (default) sends no image. Not supported for Google targets.
export type ProbeVision = 'none' | 'user' | 'tool';

export interface ProbeRequest {
    target_type: ProbeTargetType;

    // Rule test (required)
    scenario?: string;
    rule_uuid?: string;

    // Provider test (required)
    provider_uuid?: string;
    model?: string;

    // Orthogonal axes.
    stream?: boolean;
    tool?: boolean;

    // Optional custom message
    message?: string;

    // Direct: skip the TB loopback and call the upstream provider directly.
    // Only meaningful for provider targets — used to isolate whether a failure
    // is in the upstream provider or in TB's own middleware stack.
    direct?: boolean;

    // Protocol: force the client-side wire protocol. Empty keeps the target's
    // primary protocol. Not supported for rule targets (scenario fixes it).
    protocol?: ProbeProtocol;

    // Thinking: extended-thinking effort. 'none' (default) sends no thinking
    // param; 'low'/'medium'/'high'/'max' map to the provider's native thinking
    // knob. Orthogonal to stream/tool.
    thinking?: ProbeThinking;

    // Vision: attach the canonical probe image in the user message ('user')
    // or a synthetic tool-result turn ('tool'). Omitted/'none' sends no image.
    vision?: ProbeVision;

    // System prompt override; empty keeps the probe's echo instruction.
    system?: string;
    // Custom conversation replacing the single-message fixture. Exclusive
    // with `message` and with `vision`; the last turn must be a user turn.
    messages?: ProbeMessage[];
    // Send the loopback request as a real client: 'claude_code' hands it to
    // TB's own Claude Code client implementation. Through-TB + Anthropic only.
    client?: ProbeClient;

    // ── Playground ──────────────────────────────────────────────────────
    // Per-request rule-flag overlay (registry keys → values). Only keys
    // present are applied; through-TB only. Nothing is persisted.
    flags?: Record<string, unknown>;
    // Post-serialization body edits: JSON path → value (null deletes).
    body_overrides?: Record<string, unknown>;
    // Header set/override; empty value removes the header.
    headers?: Record<string, string>;
    // Rule targets: 'natural' (default) lets TB match the rule from the
    // request model as for real traffic; 'pinned' forces the chosen rule.
    routing?: ProbeRouting;
}

export type ProbeRouting = 'natural' | 'pinned';
export type ProbeClient = 'claude_code';
// The clients a probe can send as (mirrors the backend's ProbeClient values).
export const PROBE_CLIENTS: { id: ProbeClient; protocol: ProbeProtocol }[] = [{ id: 'claude_code', protocol: 'anthropic_v1' }];
export type ProbeMessageRole = 'user' | 'assistant' | 'system';

export interface ProbeMessage {
    role: ProbeMessageRole;
    text: string;
}

export interface ProbeToolCall {
    id: string;
    name: string;
    input: Record<string, unknown>;
}

// Result payload of POST /api/v2/probe (backend probe.ProbeResult).
export interface ProbeTokenUsage {
    input_tokens: number;
    output_tokens: number;
    cache_read_tokens?: number;
    cache_write_tokens?: number;
    reasoning_tokens?: number;
}

export interface ProbeResultData {
    content?: string;
    latency_ms: number;
    request_url?: string;
    stream?: boolean;
    // Request-echo axes — the exact combination that produced this result.
    // The probe panel does not persist axes across opens, so reopening a
    // stored result restores its control state from this echo.
    tool?: boolean;
    direct?: boolean;
    protocol?: ProbeProtocol;
    thinking?: ProbeThinking;
    vision?: ProbeVision;
    // Canonical token usage (same shape as protocol.TokenUsage on the backend):
    // input_tokens / output_tokens / cache_read_tokens / cache_write_tokens /
    // reasoning_tokens. Present for OpenAI Chat/Responses and Anthropic probes
    // (non-stream always; stream when the provider emits a final usage block);
    // absent for Google and cache hits.
    usage?: ProbeTokenUsage;
    tool_calls?: ProbeToolCall[];
    // Routing trace — populated for TB-loopback probes.
    selected_provider?: string;
    selected_provider_uuid?: string;
    selected_model?: string;
    routing_source?: string;
    matched_smart_rule?: number;
    // Execution-level facts (real upstream endpoint, matched rule, applied flags).
    upstream_api?: string;
    upstream_url?: string;
    matched_rule?: string;
    matched_rule_desc?: string;
    applied_flags?: string;
}

// Envelope of POST /api/v2/probe.
export interface ProbeResult {
    success: boolean;
    error?: { message: string; type: string };
    data?: ProbeResultData;
}

