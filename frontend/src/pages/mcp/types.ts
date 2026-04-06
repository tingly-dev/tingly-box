export interface MCPSourceConfig {
    id?: string;
    transport?: 'http' | 'stdio';
    endpoint?: string;
    headers?: Record<string, string>;
    tools?: string[];
    command?: string;
    args?: string[];
    cwd?: string;
    env?: Record<string, string>;
    proxy_url?: string;
}

export interface MCPRuntimeConfig {
    sources?: MCPSourceConfig[];
    request_timeout?: number;
}

export interface MCPConfigResponse {
    success: boolean;
    config?: MCPRuntimeConfig;
    error?: string;
}

export const BUILTIN_IDS = ['webtools'] as const;
export type BuiltinId = typeof BUILTIN_IDS[number];
