import type {
    MCPCatalogTool,
    MCPClientProfile,
    MCPClientRoute,
    MCPRouteSource,
    MCPRoutingSnapshot,
    MCPSourceConfig,
} from './types';

export type RelationshipState =
    | 'available'
    | 'mcpOff'
    | 'sourceOff'
    | 'discoveryFailed'
    | 'failedConnection'
    | 'advisorUnconfigured'
    | 'disconnected'
    | 'toolOff'
    | 'allowListOff'
    | 'ordinaryOff'
    | 'gatewayOff'
    | 'clientOff'
    | 'sourceNotGranted'
    | 'toolNotGranted'
    | 'advisorContext'
    | 'notConfirmed';

const includes = (grants: string[] | undefined, value: string) =>
    Boolean(grants?.includes('*') || grants?.includes(value));

function sharedBlock(
    source: MCPRouteSource,
    config: MCPSourceConfig | undefined,
    enabled: boolean
): RelationshipState | undefined {
    if (!enabled) return 'mcpOff';
    if (config?.enabled === false || source.state === 'disabled') return 'sourceOff';
    if (source.state === 'error') return 'failedConnection';
    if (source.state === 'unconfigured') return 'advisorUnconfigured';
    if (source.state === 'disconnected') return 'disconnected';
    if (source.state !== 'connected') return 'discoveryFailed';
}

export function clientToolState(
    source: MCPRouteSource,
    tool: MCPCatalogTool,
    client: MCPClientRoute,
    profile: MCPClientProfile | undefined,
    config: MCPSourceConfig | undefined,
    enabled: boolean
): RelationshipState {
    const blocked = sharedBlock(source, config, enabled);
    if (blocked) return blocked;
    if (source.processing === 'advisor') return 'advisorContext';
    if (!tool.enabled)
        return config?.tools?.length && !config.tools.includes('*') && !config.tools.includes(tool.name)
            ? 'allowListOff'
            : 'toolOff';
    if (!tool.usage.client) return 'ordinaryOff';
    if (!client.enabled) return 'clientOff';
    // Only the backend's effective projection can confirm availability. Saved
    // grants explain a missing edge, but must never manufacture a positive edge.
    if (
        client.sources.some(
            (branch) =>
                branch.id === source.id && branch.tools.some((item) => item.normalized_name === tool.normalized_name)
        )
    )
        return 'available';
    if (profile && !includes(profile.sources, source.id)) return 'sourceNotGranted';
    if (profile && !includes(profile.tools, tool.normalized_name)) return 'toolNotGranted';
    return 'notConfirmed';
}

export function gatewayToolState(
    source: MCPRouteSource,
    tool: MCPCatalogTool,
    snapshot: MCPRoutingSnapshot,
    config: MCPSourceConfig | undefined,
    enabled: boolean
): RelationshipState {
    const blocked = sharedBlock(source, config, enabled);
    if (blocked) return blocked;
    if (!tool.enabled)
        return config?.tools?.length && !config.tools.includes('*') && !config.tools.includes(tool.name)
            ? 'allowListOff'
            : 'toolOff';
    if (!tool.usage.gateway) return 'gatewayOff';
    return snapshot.server_tools.some(
        (branch) =>
            branch.id === source.id && branch.tools.some((item) => item.normalized_name === tool.normalized_name)
    )
        ? 'available'
        : 'notConfirmed';
}

export function configuredClientUsesSource(source: MCPRouteSource, profile: MCPClientProfile): boolean {
    if (source.processing === 'advisor' || !includes(profile.sources, source.id)) return false;
    if (profile.tools?.includes('*')) return true;
    if (source.tools.some((tool) => includes(profile.tools, tool.normalized_name))) return true;
    // Discovery failure means the tool catalog is unknown. A saved normalized
    // grant identifies a configured association, never confirmed availability.
    return (
        source.state !== 'connected' &&
        Boolean(profile.tools?.some((name) => name.startsWith(`tingly_box_mcp__${source.id}__`)))
    );
}
