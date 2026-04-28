export interface MCPSourceConfig {
    id?: string;
    name?: string;
    enabled?: boolean;
    transport?: 'http' | 'stdio' | 'sse';
    endpoint?: string;
    headers?: Record<string, string>;
    tools?: string[];
    command?: string;
    args?: string[];
    cwd?: string;
    env?: Record<string, string>;
    proxy_url?: string;
    is_client_tool?: boolean; // undefined means servertool (default for backward compatibility)
    // Local mode specific fields
    connection_type?: 'stdio' | 'http' | 'sse';
    auth_type?: 'none' | 'headers' | 'oauth';
    tools_to_execute?: string[];
    tools_auto_exec?: string[];
    allowed_extra_headers?: string[];
    auto_registered?: boolean;
    advisor?: {
        base_url?: string;
        model?: string;
        api_key?: string;
        max_uses_per_request?: number;
        max_tokens?: number;
    };
}

export interface MCPRuntimeConfig {
    sources?: MCPSourceConfig[];
    request_timeout?: number;
    strip_disabled_mcp_tools?: boolean;
}

export interface MCPConfigResponse {
    success: boolean;
    config?: MCPRuntimeConfig;
    error?: string;
}

export const BUILTIN_WEBTOOLS_ID = 'webtools' as const;
export const BUILTIN_ADVISOR_ID = 'advisor' as const;
export const BUILTIN_IDS = [BUILTIN_WEBTOOLS_ID, BUILTIN_ADVISOR_ID] as const;
export type BuiltinId = typeof BUILTIN_IDS[number];

export interface MCPKVPair {
    key: string;
    value: string;
}

export interface MCPSourceFormValue {
    id: string;
    enabled: boolean;
    transport: 'http' | 'stdio' | 'sse';
    endpoint: string;
    command: string;
    args: string[];
    env: MCPKVPair[];
    envPassthrough: string[];
    cwd: string;
    tools: string[];
    useGlobalProxy: boolean;
    proxyUrl: string;
    isClientTool: boolean; // true if this source is a client tool
}

export const MCP_DEFAULT_CWD = '~/.tingly-box/mcp';

export const defaultMCPSourceFormValue = (): MCPSourceFormValue => ({
    id: '',
    enabled: true,
    transport: 'stdio',
    endpoint: '',
    command: '', // STDIO command (empty default; no special 'builtin' marker)
    args: [],
    env: [],
    envPassthrough: [],
    cwd: MCP_DEFAULT_CWD,
    tools: ['*'],
    useGlobalProxy: true,
    proxyUrl: '',
    isClientTool: false, // default is client tool
});

const isPassthroughValue = (key: string, value: string): boolean => value === `\${${key}}`;

export const sourceToFormValue = (source?: MCPSourceConfig): MCPSourceFormValue => {
    const form = defaultMCPSourceFormValue();
    if (!source) {
        return form;
    }
    const envEntries = Object.entries(source.env || {});
    const env: MCPKVPair[] = [];
    const envPassthrough: string[] = [];
    for (const [key, value] of envEntries) {
        if (isPassthroughValue(key, value)) {
            envPassthrough.push(key);
        } else {
            env.push({ key, value });
        }
    }

    let command = source.command || '';
    let args = source.args || [];
    // Detect builtin tools: tingly-box + mcp-builtin subcommand
    if (source.command === 'tingly-box' && source.args && source.args.includes('mcp-builtin')) {
        command = 'builtin';
        args = [];
    }

    const normalizedTransport = source.transport === 'http' || source.transport === 'sse' || source.transport === 'stdio'
        ? source.transport
        : 'stdio';

    // advisor is an in-process backend transport; keep frontend UX on stdio editor.
    if (source.transport === 'advisor' && !command) {
        command = 'builtin';
        args = [];
    }

    return {
        id: source.id || '',
        enabled: source.enabled ?? true,
        transport: normalizedTransport,
        endpoint: source.endpoint || '',
        command,
        args,
        env,
        envPassthrough,
        cwd: source.cwd || MCP_DEFAULT_CWD,
        tools: source.tools && source.tools.length > 0 ? source.tools : ['*'],
        useGlobalProxy: !source.proxy_url,
        proxyUrl: source.proxy_url || '',
        isClientTool: source.is_client_tool ?? false,
    };
};

export const formValueToSource = (form: MCPSourceFormValue): MCPSourceConfig => {
    const envMap: Record<string, string> = {};
    for (const row of form.env) {
        const key = row.key.trim();
        if (!key) continue;
        envMap[key] = row.value;
    }
    for (const keyRaw of form.envPassthrough) {
        const key = keyRaw.trim();
        if (!key) continue;
        envMap[key] = `\${${key}}`;
    }

    const source: MCPSourceConfig = {
        id: form.id.trim(),
        enabled: form.enabled,
        transport: form.transport,
        tools: (form.tools || []).map((t) => t.trim()).filter(Boolean),
        is_client_tool: form.isClientTool,
    };

    if (form.transport === 'http') {
        source.endpoint = form.endpoint.trim();
    } else {
        // Handle builtin command marker
        if (form.command === 'builtin') {
            // Convert builtin marker to actual tingly-box command
            source.command = 'tingly-box';
            source.args = ['mcp-builtin'];
        } else {
            source.command = form.command.trim();
            source.args = (form.args || []).map((a) => a.trim()).filter(Boolean);
        }
        source.cwd = form.cwd.trim();
    }

    if (Object.keys(envMap).length > 0) {
        source.env = envMap;
    }

    if (!form.useGlobalProxy && form.proxyUrl.trim()) {
        source.proxy_url = form.proxyUrl.trim();
    }

    return source;
};
