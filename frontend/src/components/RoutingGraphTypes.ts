/**
 * Shared types for RuleGraphV2 and TabTemplatePage
 */

export interface ConfigProvider {
    uuid: string;
    provider: string;
    model: string;
    isManualInput?: boolean;
    weight?: number;
    active?: boolean;
    time_window?: number;
    // Tier within a rule. Lower number = tried first (T0 is highest priority);
    // 0 / undefined = unset, sinks to the bottom tier. Setting this on any
    // service flips the rule's load-balancing tactic to "tier" (direct + fallback).
    tier?: number;
}

export interface SmartOp {
    uuid: string;
    position: 'model' | 'thinking' | 'context_system' | 'context_user' | 'latest_user' | 'tool_use' | 'token' | 'service_ttft' | 'service_capacity' | 'agent.claude_code' | 'time';
    operation: string;
    value: string;
    meta?: {
        description?: string;
        type?: 'string' | 'int' | 'bool' | 'float' | 'time_range';
    };
}

export interface SmartRouting {
    uuid: string;
    description: string;
    ops: SmartOp[];
    services: ConfigProvider[];
}

export interface ConfigRecord {
    uuid: string;
    scenario?: string;
    requestModel: string;
    responseModel: string;
    active: boolean;
    providers: ConfigProvider[];
    description?: string;
    flags?: RuleFlags;
    // Smart routing fields
    smartEnabled?: boolean;
    smartRouting?: SmartRouting[];
    // Current load-balancing tactic name. Round-tripped so the tier
    // tactic flips on automatically once a user assigns service order.
    lbTactic?: string;
}

export interface VisionProxyServiceRef {
    provider: string;
    model: string;
}

export interface RuleFlags {
    cursorCompat?: boolean;
    cursorCompatAuto?: boolean;
    skipUsage?: boolean;
    customUserAgent?: string;
    useMaxCompletionTokens?: boolean;
    useMaxTokens?: boolean;
    openaiEndpointOverride?: string;
    blockTools?: string;
    thinkingEffort?: string;
    sessionAffinity?: number;
    visionProxyService?: VisionProxyServiceRef;
    claudeCodeCompat?: boolean;
    cleanHeader?: boolean;
    context1m?: boolean;
}

export interface RuleFlagsApi {
    cursor_compat?: boolean;
    cursor_compat_auto?: boolean;
    skip_usage?: boolean;
    custom_user_agent?: string;
    use_max_completion_tokens?: boolean;
    use_max_tokens?: boolean;
    openai_endpoint_override?: string;
    block_tools?: string;
    thinking_effort?: string;
    session_affinity?: number;
    vision_proxy_service?: VisionProxyServiceRef;
    claude_code_compat?: boolean;
    clean_header?: boolean;
    context_1m?: boolean;
}

export type FlagValueType = 'bool' | 'string' | 'enum' | 'int' | 'service_ref';

export interface FlagOption {
    value: string;
    label: string;
}

export interface FlagSpec {
    key: string;
    label: string;
    description: string;
    type: FlagValueType;
    category: string;
    placeholder?: string;
    options?: FlagOption[];
    // Non-exhaustive recommended values for string flags (e.g. custom_user_agent
    // User-Agent presets). Rendered as quick-pick chips; free-form input is still
    // allowed. Populated from the backend registry (typ.DefaultUserAgents()).
    suggestions?: FlagOption[];
    shared?: boolean;
    inheritanceMode?: string;
}

export interface Rule {
    uuid: string;
    scenario: string;
    request_model: string;
    response_model?: string;
    active?: boolean;
    description?: string;
    flags?: RuleFlagsApi;
    services?: Array<{
        id?: string;
        uuid?: string;
        provider: string;
        model: string;
        weight?: number;
        active?: boolean;
        time_window?: number;
        tier?: number;
    }>;
    // Smart routing fields
    smart_enabled?: boolean;
    smart_routing?: SmartRouting[];
    lb_tactic?: {
        type?: string;
        params?: Record<string, unknown>;
    };
}
