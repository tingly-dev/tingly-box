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

export interface ProbeV2Response {
    success: boolean;
    error?: {
        message: string;
        type: string;
    };
    data?: {
        content?: string;
        usage?: {
            prompt_tokens: number;
            completion_tokens: number;
            total_tokens: number;
        };
        latency_ms: number;
        request_url?: string;
    };
}
