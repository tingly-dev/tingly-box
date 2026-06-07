// Probe V2 Types
// Note: These are custom types not in the codegen schema

export type ProbeV2TargetType = 'rule' | 'provider' | 'provider_config';
export type ProbeV2TestMode = 'simple' | 'streaming' | 'tool';

export interface ProbeV2Request {
    target_type: ProbeV2TargetType;

    // Rule test (required)
    scenario?: string;
    rule_uuid?: string;

    // Provider test (required)
    provider_uuid?: string;
    model?: string;

    // Test mode
    test_mode: ProbeV2TestMode;

    // Optional custom message
    message?: string;

    // Direct: skip the TB loopback and call the upstream provider directly.
    // Only meaningful for provider targets — used to isolate whether a failure
    // is in the upstream provider or in TB's own middleware stack.
    direct?: boolean;
}

export interface ProbeToolCall {
    id: string;
    name: string;
    input: Record<string, unknown>;
}

export interface ProbeV2Response {
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

        // Token usage (flattened)
        prompt_tokens?: number;
        completion_tokens?: number;
        total_tokens?: number;

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
