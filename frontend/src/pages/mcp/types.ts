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

export interface MCPKVPair {
    key: string;
    value: string;
}

export interface MCPSourceFormValue {
    id: string;
    transport: 'http' | 'stdio';
    endpoint: string;
    command: string;
    args: string[];
    env: MCPKVPair[];
    envPassthrough: string[];
    cwd: string;
    tools: string[];
    useGlobalProxy: boolean;
    proxyUrl: string;
}

export const MCP_DEFAULT_CWD = '~/.tingly-box/mcp';

export const defaultMCPSourceFormValue = (): MCPSourceFormValue => ({
    id: '',
    transport: 'stdio',
    endpoint: '',
    command: 'python3',
    args: ['mcp_web_tools.py'],
    env: [],
    envPassthrough: [],
    cwd: MCP_DEFAULT_CWD,
    tools: ['*'],
    useGlobalProxy: true,
    proxyUrl: '',
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
    return {
        id: source.id || '',
        transport: (source.transport as 'http' | 'stdio') || 'stdio',
        endpoint: source.endpoint || '',
        command: source.command || 'python3',
        args: source.args || [],
        env,
        envPassthrough,
        cwd: source.cwd || MCP_DEFAULT_CWD,
        tools: source.tools && source.tools.length > 0 ? source.tools : ['*'],
        useGlobalProxy: !source.proxy_url,
        proxyUrl: source.proxy_url || '',
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
        transport: form.transport,
        tools: (form.tools || []).map((t) => t.trim()).filter(Boolean),
    };

    if (form.transport === 'http') {
        source.endpoint = form.endpoint.trim();
    } else {
        source.command = form.command.trim();
        source.args = (form.args || []).map((a) => a.trim()).filter(Boolean);
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
