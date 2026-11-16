import { BUILTIN_IDS, BUILTIN_SERVER_WEBTOOLS_ID, type MCPCatalogTool, type MCPSourceConfig } from './types';

export type MCPToolPatch = { enabled?: boolean };

export const isAdvisorTool = (tool: MCPCatalogTool, source?: MCPSourceConfig) =>
    source?.transport === 'advisor' ||
    !!source?.advisor ||
    (tool.source_id === 'advisor' && tool.implementation === 'virtual');

// Tingly Box's own sources (Advisor, web tools) have no connection to fix: their
// tools are switched on right here, which also turns the source on.
export const isBuiltinSource = (source?: MCPSourceConfig) =>
    source?.origin === 'builtin' || BUILTIN_IDS.some((id) => id === source?.id);

export const isServerWebtoolsSource = (source?: MCPSourceConfig) => source?.id === BUILTIN_SERVER_WEBTOOLS_ID;

export const needsSearchKey = (source?: MCPSourceConfig) =>
    isServerWebtoolsSource(source) && !source?.env?.SERPER_API_KEY?.trim();

// Server Tools have their own configuration and do not belong to MCP profiles.
export const isServerToolSource = (source?: MCPSourceConfig) =>
    source?.transport === 'advisor' || !!source?.advisor || source?.id === 'advisor' || isServerWebtoolsSource(source);

export const isToolRestricted = (tool: MCPCatalogTool, source?: MCPSourceConfig) => {
    const allowed = source?.tools || [];
    const off = source?.enabled === false && !isBuiltinSource(source);
    return off || (allowed.length > 0 && !allowed.includes('*') && !allowed.includes(tool.name));
};

export const toolPolicyPatch = (
    source: MCPSourceConfig,
    tool: MCPCatalogTool,
    patch: MCPToolPatch
): MCPSourceConfig => ({
    id: source.id,
    ...(patch.enabled && isBuiltinSource(source) ? { enabled: true } : {}),
    tool_policies: {
        ...source.tool_policies,
        [tool.name]: { ...source.tool_policies?.[tool.name], ...patch },
    },
});

// One patch that sets every given tool on or off; tools the connection restricts are left alone.
export const toolsPolicyPatch = (
    source: MCPSourceConfig,
    tools: MCPCatalogTool[],
    enabled: boolean
): MCPSourceConfig => ({
    id: source.id,
    ...(enabled && isBuiltinSource(source) ? { enabled: true } : {}),
    tool_policies: Object.fromEntries(
        tools
            .filter((tool) => !isToolRestricted(tool, source))
            .map((tool) => [tool.name, { ...source.tool_policies?.[tool.name], enabled }])
    ),
});

export type ConnectionSummary = {
    // Where the tools come from: URL, command line or Advisor's model.
    target: string;
    // Short facts about what is configured, e.g. "2 env vars".
    details: Array<{ key: 'headers' | 'env' | 'proxy'; count: number }>;
    // What the user still has to fill in before the connection can work.
    missing: Array<'endpoint' | 'command' | 'advisorModel' | 'searchKey' | 'envValue' | 'headerValue'>;
};

const emptyValues = (map?: Record<string, string>) => Object.values(map || {}).filter((v) => !v.trim()).length;

// What the connection card shows about a source's configuration, so the user
// sees what is set and what is still missing without opening the drawer.
export const connectionSummary = (source: MCPSourceConfig): ConnectionSummary => {
    const missing: ConnectionSummary['missing'] = [];
    let target = '';
    if (isServerWebtoolsSource(source)) {
        target = 'Serper / Jina Reader';
        if (needsSearchKey(source)) missing.push('searchKey');
    } else if (source.transport === 'advisor' || source.advisor) {
        target = source.advisor?.model || '';
        if (!target) missing.push('advisorModel');
    } else if (source.transport === 'stdio') {
        target = [source.command, ...(source.args || [])].filter(Boolean).join(' ');
        if (!source.command?.trim()) missing.push('command');
    } else {
        target = source.endpoint || '';
        if (!source.endpoint?.trim()) missing.push('endpoint');
    }
    if (emptyValues(source.env)) missing.push('envValue');
    if (emptyValues(source.headers)) missing.push('headerValue');
    const count = (map?: Record<string, string>) => Object.keys(map || {}).length;
    const details: ConnectionSummary['details'] = [];
    if (count(source.headers)) details.push({ key: 'headers', count: count(source.headers) });
    if (count(source.env)) details.push({ key: 'env', count: count(source.env) });
    if (source.proxy_url) details.push({ key: 'proxy', count: 1 });
    return { target, details, missing };
};
