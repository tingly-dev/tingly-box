import type { components } from '@/client';

export type MCPSourceConfig = components['schemas']['MCPSourceConfig'];
export type MCPRuntimeConfig = components['schemas']['MCPRuntimeConfig'];
export type MCPConfigResponse = components['schemas']['MCPRuntimeConfigResponse'];
export type MCPCatalogTool = components['schemas']['CatalogTool'];
export type MCPSourceStatus = components['schemas']['SourceStatus'];
export type MCPClientProfile = components['schemas']['MCPClientProfile'];
export type MCPToolUsage = components['schemas']['MCPToolUsage'];
export type MCPRouteSource = components['schemas']['RouteSource'];
export type MCPClientRoute = components['schemas']['ClientRoute'];
export type MCPRoutingSnapshot = components['schemas']['RoutingSnapshot'];

export const BUILTIN_WEBTOOLS_ID = 'webtools' as const;
export const BUILTIN_ADVISOR_ID = 'advisor' as const;
export const BUILTIN_IDS = [BUILTIN_WEBTOOLS_ID, BUILTIN_ADVISOR_ID] as const;
export type BuiltinId = (typeof BUILTIN_IDS)[number];

export interface MCPKVPair {
    key: string;
    value: string;
}

export interface MCPSourceFormValue {
    id: string;
    name: string;
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
    visibility: 'client' | 'server';
    headers: MCPKVPair[];
    usage: MCPToolUsage;
    original?: MCPSourceConfig;
}

export const MCP_DEFAULT_CWD = '~/.tingly-box/mcp';

export const defaultMCPSourceFormValue = (): MCPSourceFormValue => ({
    id: '',
    name: '',
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
    visibility: 'client',
    headers: [],
    usage: { client: true, gateway: false },
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

    const normalizedTransport =
        source.transport === 'http' || source.transport === 'sse' || source.transport === 'stdio'
            ? source.transport
            : 'stdio';

    // advisor is an in-process backend transport; keep frontend UX on stdio editor.
    if (source.transport === 'advisor' && !command) {
        command = 'builtin';
        args = [];
    }

    return {
        original: source,
        headers: Object.entries(source.headers || {}).map(([key, value]) => ({ key, value })),
        usage: source.usage ?? {
            client: source.visibility !== 'server' && source.transport !== 'advisor',
            gateway: source.visibility === 'server' || source.transport === 'advisor',
        },
        id: source.id || '',
        name: source.name || '',
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
        visibility: source.visibility === 'server' ? 'server' : 'client',
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
        ...form.original,
        usage: form.usage,
        id: form.id.trim(),
        name: form.name.trim() || form.id.trim(),
        enabled: form.enabled,
        transport: form.transport,
        tools: (form.tools || []).map((t) => t.trim()).filter(Boolean),
        visibility: form.visibility,
    };

    if (form.transport === 'http' || form.transport === 'sse') {
        source.endpoint = form.endpoint.trim();
        source.headers = Object.fromEntries(
            form.headers.filter((row) => row.key.trim()).map((row) => [row.key.trim(), row.value])
        );
    } else {
        // Handle builtin command marker
        if (form.command === 'builtin') {
            // Convert builtin marker to actual tingly-box command
            source.origin = 'builtin';
            source.command = 'tingly-box';
            source.args = ['mcp-builtin'];
        } else {
            source.command = form.command.trim();
            source.args = [...form.args];
        }
        source.cwd = form.cwd.trim();
    }

    source.env = envMap;

    source.proxy_url = form.useGlobalProxy ? '' : form.proxyUrl.trim();

    return source;
};
