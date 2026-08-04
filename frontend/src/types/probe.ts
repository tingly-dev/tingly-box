// Probe Types
// Note: These are custom types not in the codegen schema

export type ProbeTargetType = 'rule' | 'provider' | 'provider_config';
export type ProbeTestMode = 'simple' | 'streaming' | 'tool';

export interface ProbeRequest {
    target_type: ProbeTargetType;

    // Rule test (required)
    scenario?: string;
    rule_uuid?: string;

    // Provider test (required)
    provider_uuid?: string;
    model?: string;

    // Test mode
    test_mode: ProbeTestMode;

    // Optional custom message
    message?: string;

    // Direct: skip the TB loopback and call the upstream provider directly.
    // Only meaningful for provider targets — used to isolate whether a failure
    // is in the upstream provider or in TB's own middleware stack.
    direct?: boolean;

    // Endpoint: force which OpenAI endpoint to probe ('chat' | 'responses').
    // OpenAI-style providers only; ignored otherwise. Empty keeps the default
    // (Codex OAuth providers probe responses, everything else probes chat).
    endpoint?: 'chat' | 'responses';
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

export interface ProbeResponse {
    success: boolean;
    error?: {
        message: string;
        type: string;
    };
    data?: {
        success?: boolean;
        message?: string;
        content?: string;
        latency_ms: number;
        request_url?: string;
        stream?: boolean;

        // Canonical token usage (protocol.TokenUsage shape).
        usage?: ProbeTokenUsage;

        // Tool calls
        tool_calls?: ProbeToolCall[];

        // Routing trace — populated for TB-loopback probes (provider/rule
        // through-TB). Empty for direct and provider_config probes.
        selected_provider?: string;
        selected_provider_uuid?: string;
        selected_model?: string;
        routing_source?: string;
        matched_smart_rule?: number;

        // Other fields
        models_count?: number;
        error_message?: string;
    };
}
