// Probe V2 Types
// Note: These are custom types not in the codegen schema

export type ProbeV2TargetType = 'rule' | 'provider' | 'provider_config';
export type ProbeV2TestMode = 'simple' | 'streaming';

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

        // Other fields
        models_count?: number;
        error_message?: string;
    };
}
