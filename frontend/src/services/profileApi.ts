// Profile control-plane API: applied tool configs (claude/codex/dsh readback)
// and per-scenario profile CRUD incl. Claude Code profile preferences.
import type {components} from '@/client';
import {controlApi} from './openapi';

export type ClaudeCodeModels = components['schemas']['ClaudeCodeModelsData'];
export type ClaudeCodeModelTier = components['schemas']['ClaudeCodeModelTier'];
export type ClaudeCodeRoute = components['schemas']['ClaudeCodeRouteResponse'];

export const profileApi = {
    // Profile API

    getAppliedClaudeConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/config/claude', {headers}));
    },

    getAppliedCodexConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/config/codex', {headers}));
    },

    getAppliedDshConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/config/dsh', {headers}));
    },

    getProfiles: async (scenario: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/profiles', {
            headers,
            params: {path: {scenario}},
        }));
    },

    createProfile: async (scenario: string, name: string, unified?: boolean): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/scenario/{scenario}/profiles', {
            headers,
            params: {path: {scenario}},
            body: {name, unified},
        }));
    },

    updateProfile: async (scenario: string, id: string, name: string, unified?: boolean): Promise<any> => {
        const body: { name?: string; unified?: boolean } = {};
        if (name) {
            body.name = name;
        }
        if (unified !== undefined) {
            body.unified = unified;
        }
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/profiles/{id}', {
            headers,
            params: {path: {scenario, id}},
            body,
        }));
    },

    deleteProfile: async (scenario: string, id: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/scenario/{scenario}/profiles/{id}', {
            headers,
            params: {path: {scenario, id}},
        }));
    },

    // The model tiers Claude Code can be asked for under the main routing
    // ('') or a profile, each with its current route. Resolves undefined on
    // failure (controlApi reports errors in-band).
    getClaudeCodeModels: async (profile: string): Promise<ClaudeCodeModels | undefined> => {
        const res = await controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/models', {
            headers,
            params: {path: {scenario: 'claude_code'}, query: {profile}},
        }));
        return res?.success ? res.data : undefined;
    },

    // Where a Claude Code session's next request goes under a profile and
    // tier, with the routed provider's quota: a session's status line.
    // Resolves undefined on failure.
    getClaudeCodeRoute: async (q: {profile: string; tier: string; model?: string}): Promise<ClaudeCodeRoute | undefined> => {
        const res = await controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/route', {
            headers,
            params: {path: {scenario: 'claude_code'}, query: {profile: q.profile, tier: q.tier, model: q.model}},
        }));
        return res && typeof res.scenario === 'string' ? res as ClaudeCodeRoute : undefined;
    },

    getClaudeCodeProfileConfig: async (scenario: string, id: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/profiles/{id}/claude-config', {
            headers,
            params: {path: {scenario, id}},
        }));
    },

    updateClaudeCodeProfileConfig: async (
        scenario: string,
        id: string,
        preferences: Record<string, string>,
        defaultMode: string,
    ): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/profiles/{id}/claude-config', {
            headers,
            params: {path: {scenario, id}},
            body: {preferences, defaultMode},
        }));
    },

    resetClaudeCodeProfileConfig: async (scenario: string, id: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/scenario/{scenario}/profiles/{id}/claude-config', {
            headers,
            params: {path: {scenario, id}},
        }));
    },
};
