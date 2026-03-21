// Probe V3 Types

export type ProbeV2TargetType = 'rule' | 'provider';
export type ProbeV2TestMode = 'simple' | 'streaming' | 'tool';

export interface ProbeV2Request {
  target_type: ProbeV2TargetType;

  // Rule test (required)
  scenario?: string;
  rule_uuid?: string;

  // Provider test (both required)
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
    tool_calls?: ProbeV2ToolCall[];
    usage?: {
      prompt_tokens: number;
      completion_tokens: number;
      total_tokens: number;
    };
    latency_ms: number;
    request_url?: string;
  };
}

export interface ProbeV2ToolCall {
  id: string;
  name: string;
  arguments: Record<string, unknown>;
}
