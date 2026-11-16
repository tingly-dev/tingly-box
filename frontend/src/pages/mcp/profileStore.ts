import { useEffect, useMemo, useSyncExternalStore } from 'react';
import { api } from '@/services/api';
import { MCP_DEFAULT_PROFILE_ID, type MCPProfile } from './types';

// MCP profiles shared by the sidebar (one row per profile) and the Profile
// page. A page-free module on purpose — the sidebar imports it and must not
// pull a page into the eager bundle (frontend/CLAUDE.md). The page publishes
// every config it loads or saves, so the sidebar follows without refetching.
let profiles: MCPProfile[] = [];
let loaded = false;
const listeners = new Set<() => void>();

const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => {
        listeners.delete(listener);
    };
};

export const publishMCPProfiles = (next: MCPProfile[] = []) => {
    profiles = next;
    loaded = true;
    listeners.forEach((listener) => listener());
};

// Default is always listed first, even before anything is saved for it.
export const orderedMCPProfiles = (saved: MCPProfile[]): MCPProfile[] => [
    saved.find((p) => p.id === MCP_DEFAULT_PROFILE_ID) || { id: MCP_DEFAULT_PROFILE_ID, name: 'Default', tools: [] },
    ...saved.filter((p) => p.id !== MCP_DEFAULT_PROFILE_ID),
];

export function useMCPProfiles(): MCPProfile[] {
    const current = useSyncExternalStore(subscribe, () => profiles);
    useEffect(() => {
        if (loaded) return;
        loaded = true;
        void api
            .getMCPConfig()
            .then((response) => {
                if (response?.success) publishMCPProfiles(response.config?.profiles);
            })
            .catch(() => {
                loaded = false;
            });
    }, []);
    return useMemo(() => orderedMCPProfiles(current), [current]);
}
