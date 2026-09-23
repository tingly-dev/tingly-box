// System / provider / status control-plane API: provider management
// (including import/export and the lightweight connection probe), token
// minting, version/health/shortcut/imagegen info, and the system + MCP
// runtime config get/set endpoints.
import type {components} from '@/client';
import {
    controlApi,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
} from './openapi';

export const systemApi = {
    // Status endpoints
    getStatus: async (): Promise<any> => controlApi((client, headers) => client.GET('/api/v1/status', {headers})),

    getProviders: async (): Promise<any> => {
        const body = await controlApi((client, headers) => client.GET('/api/v2/providers', {headers}));
        if (body?.success && body?.data) {
            // Sort providers alphabetically by name to reduce UI changes
            body.data.sort((a: any, b: any) => a.name.localeCompare(b.name));
        }
        return body;
    },

    // Get provider catalog entries (service providers for dropdown)
    getProviderCatalogs: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/provider-catalog', {headers})),

    // Model ordering is authoritative from the backend (config.SortProviderModels); do not re-sort here.
    updateProviderModelsByUUID: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/provider-models/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    getProviderModelsByUUID: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/provider-models/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    // Provider management
    addProvider: async (data: any, force: boolean = false): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/providers', {
            headers,
            params: {query: {force}},
            body: data
        })),

    getProvider: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/providers/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    updateProvider: async (uuid: string, data: any): Promise<any> =>
        controlApi((client, headers) => client.PUT('/api/v2/providers/{uuid}', {
            headers,
            params: {path: {uuid}},
            body: data
        })),

    deleteProvider: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v2/providers/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    toggleProvider: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/providers/{uuid}/toggle', {
            headers,
            params: {path: {uuid}}
        })),

    generateToken: async (clientId: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/token', {
            headers,
            body: {client_id: clientId}
        })),

    getToken: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/token', {headers})),

    // Imports providers from a base64/JSONL export bundle. Every imported
    // provider is always created with a freshly minted UUID.
    importProvider: async (data: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/provider-import', {
            headers,
            body: {data},
        })),

    // Exports a single provider (with its real, unmasked token) as a
    // base64 (default) or JSONL bundle.
    exportProvider: async (uuid: string, format: 'base64' | 'jsonl' = 'base64'): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/provider-export', {
            headers,
            params: {query: {uuid, format}},
        })),

    // Lightweight probe for optional key validation using OPTIONS and models endpoint
    // This is used by the "Test Connection" button - results are informational only
    probeProviderLightweight: async (name: string, api_style: string, api_base: string, token: string, auth_type?: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/probe/lightweight', {
            headers,
            body: {
                name: name,
                api_style: api_style as any,
                api_base: api_base,
                token: token,
                auth_type: auth_type,
            }
        })),

    getVersion: async (): Promise<string> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/info/version', {headers});
            // This route has no declared error response, so openapi-fetch's
            // generated type narrows `error` to `never` on the (only) success
            // branch; widen to read it defensively (a non-2xx is still
            // possible at runtime, e.g. a 401 from auth middleware).
            const err = (response as {error?: unknown}).error;
            if (err || !response.data) {
                console.error('Failed to get version:', err || 'No data in response');
                return 'Unknown';
            }
            return response.data?.data?.version || 'Unknown';
        } catch (error: any) {
            console.error('Failed to get version:', error);
            return 'Unknown';
        }
    },

    getLatestVersion: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/info/version/check', {headers})),

    // Desktop / start-menu shortcut. getShortcutStatus is a read-only check
    // (does every artifact createShortcut would write already exist?); it
    // never writes to disk. createShortcut is idempotent — safe to call again
    // any time (after an upgrade, a source change, or to recover a deleted
    // shortcut).
    getShortcutStatus: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/shortcut', {headers})),

    createShortcut: async (name?: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/shortcut', {
            headers,
            body: name ? {name} : {},
        })),

    // Read-only imagegen scenario info — currently just where generated/edited
    // images are persisted on disk (~/.tingly-box/image). The frontend shows
    // the path for the user to navigate to themselves; this server never
    // reaches into the local OS to open it.
    getImageGenInfo: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imagegen/info', {headers})),

    healthCheck: async (): Promise<boolean> => {
        try {
            const client = await getClient();
            const response = await client.GET('/api/v1/info/health');
            return response.data?.health === true;
        } catch {
            return false;
        }
    },

    // ========== System Configuration API ==========

    // Get system configuration
    getConfig: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/config', {headers})),

    // Update system configuration
    updateConfig: async (config: any): Promise<any> =>
        controlApi((client, headers) => client.PUT('/api/v1/config', {
            headers,
            body: config
        })),

    // ========== MCP Runtime API ==========

    // Get MCP runtime config
    getMCPConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/mcp/config', {headers}));
    },

    // Set MCP runtime config
    setMCPConfig: async (config: components['schemas']['MCPRuntimeConfigRequest']): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/mcp/config', {
            headers,
            body: config,
        }));
    },
};
