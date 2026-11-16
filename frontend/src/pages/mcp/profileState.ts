import { MCP_DEFAULT_PROFILE_ID, type MCPCatalogTool, type MCPProfile, type MCPRoutingSnapshot } from './types';

export interface ProfileToolRow {
    name: string; // normalized
    tool?: MCPCatalogTool; // absent when the tool no longer exists
    sourceName: string;
    on: boolean; // the switch inside this profile
}

// Default is never required to be saved; it always exists and holds every tool.
export const defaultProfile = (): MCPProfile => ({ id: MCP_DEFAULT_PROFILE_ID, name: 'Default', tools: [] });

export const isDefaultProfile = (profile: MCPProfile) => profile.id === MCP_DEFAULT_PROFILE_ID;

// Server Tools run in the gateway and are never offered through a profile.
export const profileCandidates = (routing: MCPRoutingSnapshot | null) =>
    (routing?.sources || []).flatMap((source) =>
        source.tools
            .filter((tool) => tool.usage.client)
            .map((tool) => ({ tool, sourceName: source.name || source.id }))
    );

// Default lists every tool and stores only the ones switched off; a custom
// profile lists exactly the tools it contains. Either way the row's switch is
// the profile's own, independent of the tool's global switch.
export function profileRows(profile: MCPProfile, routing: MCPRoutingSnapshot | null): ProfileToolRow[] {
    const candidates = profileCandidates(routing);
    const own = new Map((profile.tools || []).map((tool) => [tool.name, tool.enabled !== false]));
    if (isDefaultProfile(profile)) {
        return candidates.map(({ tool, sourceName }) => ({
            name: tool.normalized_name,
            tool,
            sourceName,
            on: own.get(tool.normalized_name) ?? true,
        }));
    }
    return (profile.tools || []).map((entry) => {
        const found = candidates.find(({ tool }) => tool.normalized_name === entry.name);
        return {
            name: entry.name,
            tool: found?.tool,
            sourceName: found?.sourceName || '',
            on: entry.enabled !== false,
        };
    });
}

export function setProfileToolOn(profile: MCPProfile, name: string, on: boolean): MCPProfile {
    const rest = (profile.tools || []).filter((tool) => tool.name !== name);
    if (isDefaultProfile(profile)) {
        return { ...profile, tools: on ? rest : [...rest, { name, enabled: false }] };
    }
    return {
        ...profile,
        tools: (profile.tools || []).map((tool) => (tool.name === name ? { ...tool, enabled: on } : tool)),
    };
}

export const addProfileTools = (profile: MCPProfile, names: string[]): MCPProfile => ({
    ...profile,
    tools: [
        ...(profile.tools || []),
        ...names.filter((name) => !(profile.tools || []).some((tool) => tool.name === name)).map((name) => ({ name })),
    ],
});

export const removeProfileTool = (profile: MCPProfile, name: string): MCPProfile => ({
    ...profile,
    tools: (profile.tools || []).filter((tool) => tool.name !== name),
});

// The name a client registers this profile under on the user's machine. Display
// derived on purpose (the ID is addressing) and different per profile so
// installs do not overwrite each other.
export const profileServerName = (profile: MCPProfile) =>
    isDefaultProfile(profile)
        ? 'tb'
        : `tb-${(profile.name || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || profile.id}`;
