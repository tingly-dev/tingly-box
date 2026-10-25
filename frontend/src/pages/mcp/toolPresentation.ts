import type { MCPCatalogTool, MCPSourceConfig } from './types';

export type MCPToolPatch = { enabled?: boolean };

export const isAdvisorTool = (tool: MCPCatalogTool, source?: MCPSourceConfig) =>
    source?.transport === 'advisor' ||
    !!source?.advisor ||
    (tool.source_id === 'advisor' && tool.implementation === 'virtual');

export const isToolRestricted = (tool: MCPCatalogTool, source?: MCPSourceConfig) => {
    const allowed = source?.tools || [];
    return source?.enabled === false || (allowed.length > 0 && !allowed.includes('*') && !allowed.includes(tool.name));
};

export const toolPolicyPatch = (
    source: MCPSourceConfig,
    tool: MCPCatalogTool,
    patch: MCPToolPatch
): MCPSourceConfig => ({
    id: source.id,
    tool_policies: {
        ...source.tool_policies,
        [tool.name]: { ...source.tool_policies?.[tool.name], ...patch },
    },
});

export type ConnectionSummary = {
    // Where the tools come from: URL, command line or Advisor's model.
    target: string;
    // Short facts about what is configured, e.g. "2 env vars".
    details: Array<{ key: 'headers' | 'env' | 'proxy'; count: number }>;
    // What the user still has to fill in before the connection can work.
    missing: Array<'endpoint' | 'command' | 'advisorModel' | 'envValue' | 'headerValue'>;
};

const emptyValues = (map?: Record<string, string>) => Object.values(map || {}).filter((v) => !v.trim()).length;

// What the connection card shows about a source's configuration, so the user
// sees what is set and what is still missing without opening the drawer.
export const connectionSummary = (source: MCPSourceConfig): ConnectionSummary => {
    const missing: ConnectionSummary['missing'] = [];
    let target = '';
    if (source.transport === 'advisor' || source.advisor) {
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
