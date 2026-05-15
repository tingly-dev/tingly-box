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
}

export interface SmartOp {
    uuid: string;
    position: 'model' | 'thinking' | 'context_system' | 'context_user' | 'latest_user' | 'tool_use' | 'token' | 'service_ttft' | 'service_capacity' | 'agent.claude_code' | 'proxy_vision';
    operation: string;
    value: string;
    meta?: {
        description?: string;
        type?: 'string' | 'int' | 'bool' | 'float';
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
}

export interface RuleFlags {
    cursorCompat?: boolean;
    cursorCompatAuto?: boolean;
    skipUsage?: boolean;
    customUserAgent?: string;
    useMaxCompletionTokens?: boolean;
    useMaxTokens?: boolean;
}

export interface RuleFlagsApi {
    cursor_compat?: boolean;
    cursor_compat_auto?: boolean;
    skip_usage?: boolean;
    custom_user_agent?: string;
    use_max_completion_tokens?: boolean;
    use_max_tokens?: boolean;
}

export type FlagValueType = 'bool' | 'string';

export interface FlagSpec {
    key: string;
    label: string;
    description: string;
    type: FlagValueType;
    category: string;
    placeholder?: string;
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
    }>;
    // Smart routing fields
    smart_enabled?: boolean;
    smart_routing?: SmartRouting[];
}
