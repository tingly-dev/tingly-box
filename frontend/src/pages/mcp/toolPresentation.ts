import type { MCPCatalogTool, MCPSourceConfig } from './types';

export type MCPToolMode = 'asset' | 'client' | 'gateway';
export type MCPToolPatch = { enabled?: boolean; usage?: MCPCatalogTool['usage'] };

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
