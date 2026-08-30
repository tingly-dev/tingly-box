// API service layer for communicating with the backend

import TinglyService from "@/bindings";
import type {components} from '@/client';
import type {BotChat, BotSettings} from '@/types/bot';
import {getApiBaseUrl} from '../utils/protocol';
import {
    controlApi,
    errorMessage,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
    resetControlApiClient as resetClient,
    unwrap,
} from './openapi';

// Get user auth token for UI and control API from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

// Get model token for OpenAI/Anthropic API from localStorage
const getModelToken = (): string | null => {
    return localStorage.getItem('model_token');
};

// Fetch helper for model API endpoints (OpenAI/Anthropic compatible)
async function modelAPI(url: string, options: RequestInit = {}): Promise<any> {
    let token = getModelToken();

    // Try to get model token from GUI if available
    if (!token && import.meta.env.VITE_PKG_MODE === "gui") {
        const svc = TinglyService;
        if (svc) {
            try {
                const guiToken = await svc.GetUserAuthToken();
                if (guiToken) {
                    token = guiToken;
                }
            } catch (err) {
                console.error('Failed to get GUI token for modelAPI:', err);
            }
        }
    }

    const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        ...options.headers as Record<string, string>,
    };
    if (token) headers['Authorization'] = `Bearer ${token}`;

    try {
        const response = await fetch(url, {headers, ...options});
        return await response.json();
    } catch (error) {
        return {success: false, error: (error as Error).message};
    }
}

// Temporary raw control-plane call for the Bot Access endpoints. The backend
// models are already in Swagger; this helper can be removed after SDK codegen.
async function botAccessAPI(path: string, options: RequestInit = {}): Promise<any> {
    const base = await getApiBaseUrl();
    const headers = await getAuthHeaders();
    const response = await fetch(`${base}${path}`, {
        ...options,
        headers: {...headers, 'Content-Type': 'application/json', ...(options.headers || {})},
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `request failed (${response.status})`);
    return data;
}

// Temporary raw control-plane call for Team endpoints. The backend routes and
// Swagger models are authoritative; remove this shim after the generated SDK
// is refreshed from the new schema.
async function teamControlAPI(path: string, options: RequestInit = {}): Promise<any> {
    try {
        const base = await getApiBaseUrl();
        const headers = await getAuthHeaders();
        const response = await fetch(`${base}${path}`, {
            ...options,
            headers: {...headers, 'Content-Type': 'application/json', ...(options.headers || {})},
        });
        const data = await response.json().catch(() => ({}));
        if (!response.ok) {
            return {success: false, error: data.error || {message: `request failed (${response.status})`}};
        }
        return {success: true, data};
    } catch (error: any) {
        return {success: false, error: {message: error.message || 'Team API request failed'}};
    }
}

const listBotCapabilities = (botUUID: string) =>
    botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/capabilities`);


// Capability records are exposed separately from the generated bot settings
// model. Keep the join in one place so every bot surface gets the same
// per-bot failure fallback while SDK codegen catches up.
export async function enrichBotsWithCapabilities(bots: BotSettings[]): Promise<BotSettings[]> {
    return Promise.all(bots.map(async (bot) => {
        try {
            const result = await listBotCapabilities(bot.uuid!);
            return {...bot, capabilities: result.capabilities || []};
        } catch {
            return {...bot, capabilities: []};
        }
    }));
}

export const api = {
    // Initialize API client
    initialize: async (): Promise<void> => {
        await getClient();
    },

    // Status endpoints
    getStatus: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/status', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getProviders: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/providers', {headers});
            const body = unwrap(response);
            if (body?.success && body?.data) {
                // Sort providers alphabetically by name to reduce UI changes
                body.data.sort((a: any, b: any) => a.name.localeCompare(b.name));
            }
            return body;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get provider templates (service providers for dropdown)
    getProviderTemplates: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/provider-templates', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    updateProviderModelsByUUID: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/provider-models/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            // Model ordering is authoritative from the backend
            // (config.SortProviderModels); do not re-sort here.
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getProviderModelsByUUID: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/provider-models/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            // Model ordering is authoritative from the backend
            // (config.SortProviderModels); do not re-sort here.
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getHistory: async (limit?: number): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/history', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Provider management
    addProvider: async (data: any, force: boolean = false): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/providers', {
                headers,
                params: {query: {force} as any},
                body: data
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getProvider: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/providers/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    updateProvider: async (uuid: string, data: any): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.PUT('/api/v2/providers/{uuid}', {
                headers,
                params: {path: {uuid}},
                body: data
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    deleteProvider: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.DELETE('/api/v2/providers/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    toggleProvider: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/providers/{uuid}/toggle', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // List virtual models registered in the in-process registries.
    getAvailableVirtualModels: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/vmodel/available-models', {headers}));
    },

    // Server control
    startServer: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/server/start', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    stopServer: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/server/stop', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    restartServer: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/server/restart', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    generateToken: async (clientId: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/token', {
                headers,
                body: {client_id: clientId}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getToken: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/token', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Rules API
    getRules: async (scenario: string): Promise<any> => {
        if (!scenario.trim()) {
            return {success: false, error: 'Scenario is required', data: []};
        }

        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/rules', {
                headers,
                params: {query: {scenario}}
            });
            if (response.error) {
                return {success: false, error: errorMessage(response.error), data: []};
            }
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message, data: []};
        }
    },

    getRule: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/rule/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    createRule: async (uuid: string, data: any): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/rule', {
                headers,
                body: data
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    updateRule: async (uuid: string, data: any): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/rule/{uuid}', {
                headers,
                params: {path: {uuid}},
                body: data
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    deleteRule: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.DELETE('/api/v1/rule/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getRuleFlagRegistry: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/rule/flags/registry', {headers}));
    },

    // Imports providers from a base64/JSONL export bundle. Every imported
    // provider is always created with a freshly minted UUID.
    importProvider: async (data: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/provider-import', {
                headers,
                body: {
                    data,
                },
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Exports a single provider (with its real, unmasked token) as a
    // base64 (default) or JSONL bundle.
    exportProvider: async (uuid: string, format: 'base64' | 'jsonl' = 'base64'): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/provider-export', {
                headers,
                params: {query: {uuid, format}},
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Scenario API
    getScenarios: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenarios', {headers}));
    },

    getScenarioConfig: async (scenario: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}', {
            headers,
            params: {path: {scenario}},
        }));
    },

    setScenarioConfig: async (scenario: string, config: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/scenario/{scenario}', {
            headers,
            params: {path: {scenario}},
            body: config,
        }));
    },

    getScenarioFlag: async (scenario: string, flag: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
        }));
    },

    setScenarioFlag: async (scenario: string, flag: string, value: boolean): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
            body: {value},
        }));
    },

    getScenarioStringFlag: async (scenario: string, flag: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/string-flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
        }));
    },

    setScenarioStringFlag: async (scenario: string, flag: string, value: string): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/string-flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
            body: {value},
        }));
    },

    getScenarioIntFlag: async (scenario: string, flag: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/int-flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
        }));
    },

    setScenarioIntFlag: async (scenario: string, flag: string, value: number): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/int-flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
            body: {value},
        }));
    },

    // Scenario descriptors (includes supports_profiles flag)
    getScenarioDescriptors: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario-descriptors', {headers}));
    },

    // Profile API

    // Placeholder calls for the new Claude Code config endpoints. These use
    // the generated client's runtime transport; remove the casts after the
    // next OpenAPI client regeneration includes these routes.
    getAppliedClaudeConfig: async (): Promise<any> => {
        return controlApi((client, headers) => (client as any).GET('/api/v1/config/claude', {headers}));
    },

    // Placeholder for the Codex applied-config endpoint. Uses the generated
    // client's runtime transport; remove the casts after the next OpenAPI
    // client regeneration includes this route.
    getAppliedCodexConfig: async (): Promise<any> => {
        return controlApi((client, headers) => (client as any).GET('/api/v1/config/codex', {headers}));
    },

    // Placeholder for the DeepSeek Harness (dsh) applied-config endpoint. Uses
    // the generated client's runtime transport; remove the casts after the
    // next OpenAPI client regeneration includes this route.
    getAppliedDshConfig: async (): Promise<any> => {
        return controlApi((client, headers) => (client as any).GET('/api/v1/config/dsh', {headers}));
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

    getClaudeCodeProfileConfig: async (scenario: string, id: string): Promise<any> => {
        return controlApi((client, headers) => (client as any).GET('/api/v1/scenario/{scenario}/profiles/{id}/claude-config', {
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
        return controlApi((client, headers) => (client as any).PUT('/api/v1/scenario/{scenario}/profiles/{id}/claude-config', {
            headers,
            params: {path: {scenario, id}},
            body: {preferences, defaultMode},
        }));
    },

    resetClaudeCodeProfileConfig: async (scenario: string, id: string): Promise<any> => {
        return controlApi((client, headers) => (client as any).DELETE('/api/v1/scenario/{scenario}/profiles/{id}/claude-config', {
            headers,
            params: {path: {scenario, id}},
        }));
    },

    // Guardrails API
    getGuardrailsConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/config', {headers}));
    },
    getGuardrailsBuiltins: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/builtins', {headers}));
    },
    getGuardrailsRegistry: async (forceRefresh = false): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/registry', {
            headers,
            params: {query: {refresh: forceRefresh ? '1' : undefined}},
        }));
    },
    installGuardrailsRegistryPolicy: async (id: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/registry/install', {
            headers,
            body: {id},
        }));
    },
    getGuardrailsCredentials: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/credentials', {headers}));
    },
    getGuardrailsCredential: async (credentialId: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/credential/{id}', {
            headers,
            params: {path: {id: credentialId}},
        }));
    },
    createGuardrailsCredential: async (payload: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/credential', {
            headers,
            body: payload,
        }));
    },
    updateGuardrailsCredential: async (credentialId: string, payload: any): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/guardrails/credential/{id}', {
            headers,
            params: {path: {id: credentialId}},
            body: payload,
        }));
    },
    deleteGuardrailsCredential: async (credentialId: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/credential/{id}', {
            headers,
            params: {path: {id: credentialId}},
        }));
    },
    getGuardrailsHistory: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/history', {headers}));
    },
    clearGuardrailsHistory: async (): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/history', {headers}));
    },
    createGuardrailsPolicy: async (payload: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/policy', {
            headers,
            body: payload,
        }));
    },
    updateGuardrailsPolicy: async (policyId: string, payload: any): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/guardrails/policy/{id}', {
            headers,
            params: {path: {id: policyId}},
            body: payload,
        }));
    },
    deleteGuardrailsPolicy: async (policyId: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/policy/{id}', {
            headers,
            params: {path: {id: policyId}},
        }));
    },
    createGuardrailsGroup: async (payload: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/group', {
            headers,
            body: payload,
        }));
    },
    updateGuardrailsGroup: async (groupId: string, payload: any): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/guardrails/group/{id}', {
            headers,
            params: {path: {id: groupId}},
            body: payload,
        }));
    },
    deleteGuardrailsGroup: async (groupId: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/group/{id}', {
            headers,
            params: {path: {id: groupId}},
        }));
    },

    updateGuardrailsConfig: async (content: string): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/guardrails/config', {
            headers,
            body: {content},
        }));
    },
    importGuardrailsFragment: async (content: string, fileName?: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/fragment/import', {
            headers,
            body: {content, file_name: fileName},
        }));
    },
    exportGuardrailsFragments: async (paths: string[]): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/fragment/export', {
            headers,
            body: {paths},
        }));
    },

    reloadGuardrailsConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/reload', {headers}));
    },

    probeModel: async (uuid: string, model: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/probe', {
                headers,
                body: {
                    target_type: 'provider' as const,
                    provider_uuid: uuid,
                    model: model,
                    stream: false,
                    message: 'Hello, this is a test message. Please respond with a short greeting.',
                }
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Lightweight probe for optional key validation using OPTIONS and models endpoint
    // This is used by the "Test Connection" button - results are informational only
    probeProviderLightweight: async (name: string, api_style: string, api_base: string, token: string, auth_type?: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/probe/lightweight', {
                headers,
                body: {
                    name: name,
                    api_style: api_style as any,
                    api_base: api_base,
                    token: token,
                    auth_type: auth_type,
                }
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    probeProvider: async (api_style: string, api_base: string, token: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/probe', {
                headers,
                body: {
                    target_type: 'provider_config' as const,
                    api_style: api_style as any,
                    api_base: api_base,
                    token: token,
                    stream: false,
                    message: 'Hello, this is a test message. Please respond with a short greeting.',
                }
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },



    getVersion: async (): Promise<string> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/info/version', {headers});
            // openapi-fetch returns { data, error, response }
            if ((response as any).error || !response.data) {
                console.error('Failed to get version:', (response as any).error || 'No data in response');
                return 'Unknown';
            }
            return response.data?.data?.version || 'Unknown';
        } catch (error: any) {
            console.error('Failed to get version:', error);
            return 'Unknown';
        }
    },

    getLatestVersion: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/info/version/check', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Desktop / start-menu shortcut. getShortcutStatus is a read-only check
    // (does every artifact createShortcut would write already exist?); it
    // never writes to disk. createShortcut is idempotent — safe to call again
    // any time (after an upgrade, a source change, or to recover a deleted
    // shortcut).
    getShortcutStatus: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/shortcut', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    createShortcut: async (name?: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/shortcut', {
                headers,
                body: name ? {name} : {},
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    healthCheck: async (): Promise<boolean> => {
        try {
            const client = await getClient();
            const response = await client.GET('/api/v1/info/health');
            return (response.data as any)?.health === true;
        } catch {
            return false;
        }
    },

    // Model API calls (OpenAI/Anthropic compatible)
    openAIChatCompletions: (data: any): Promise<any> => modelAPI('/openai/v1/chat/completions', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    anthropicMessages: (data: any): Promise<any> => modelAPI('/anthropic/v1/messages', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    listOpenAIModels: (): Promise<any> => modelAPI('/openai/v1/models'),
    listAnthropicModels: (): Promise<any> => modelAPI('/anthropic/v1/models'),
    // Token management
    setUserToken: (token: string): void => {
        localStorage.setItem('user_auth_token', token);
        resetClient();
    },
    getUserToken: (): string | null => getUserAuthToken(),
    removeUserToken: (): void => {
        localStorage.removeItem('user_auth_token');
        resetClient();
    },
    setModelToken: (token: string): void => {
        localStorage.setItem('model_token', token);
    },
    removeModelToken: (): void => {
        localStorage.removeItem('model_token');
    },

    // Usage Dashboard API calls
    getUsageStats: async (params: {
        group_by?: string;
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
        limit?: number;
        sort_by?: 'total_tokens' | 'request_count' | 'avg_latency';
        sort_order?: 'asc' | 'desc';
    } = {}): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/usage/stats', {
                headers,
                params: {
                    query: {
                        group_by: params.group_by as any,
                        start_time: params.start_time,
                        end_time: params.end_time,
                        provider: params.provider,
                        model: params.model,
                        scenario: params.scenario,
                        user_id: params.user_id,
                        limit: params.limit,
                        sort_by: params.sort_by,
                        sort_order: params.sort_order,
                    }
                }
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getUsageTimeSeries: async (params: {
        interval?: string;
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
    } = {}): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/usage/timeseries', {
                headers,
                params: {
                    query: {
                        interval: params.interval as any,
                        start_time: params.start_time,
                        end_time: params.end_time,
                        provider: params.provider,
                        model: params.model,
                        scenario: params.scenario,
                        user_id: params.user_id,
                    } as any
                }
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getUsageRecords: async (params: {
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
        status?: string;
        limit?: number;
        offset?: number;
    } = {}): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/usage/records', {
                headers,
                params: {
                    query: {
                        start_time: params.start_time,
                        end_time: params.end_time,
                        provider: params.provider,
                        model: params.model,
                        scenario: params.scenario,
                        user_id: params.user_id,
                        status: params.status as any,
                        limit: params.limit,
                        offset: params.offset,
                    } as any
                }
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getUsagePerformance: async (params: {
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
    } = {}): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/usage/performance', {
                headers,
                params: {
                    query: {
                        start_time: params.start_time,
                        end_time: params.end_time,
                        provider: params.provider,
                        model: params.model,
                        scenario: params.scenario,
                        user_id: params.user_id,
                    }
                }
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // ============================================
    // OAuth API
    // ============================================

    // Initiate OAuth authorization flow
    oauthAuthorize: async (data: {
        provider: string;
        proxy_url?: string;
        redirect?: string;
        state?: string;
        // When set, re-authenticate this existing provider in place (preserves
        // its UUID and all rule/service references) instead of creating a new one.
        provider_uuid?: string;
    }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/oauth/authorize', {
                headers,
                body: data as any
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get OAuth session status
    oauthStatus: async (session_id: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/oauth/status', {
                headers,
                params: {query: {session_id}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Cancel an in-progress OAuth session
    oauthCancel: async (data: { session_id: string }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/oauth/cancel', {
                headers,
                body: data
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Refresh OAuth token
    oauthRefresh: async (data: { provider_uuid: string }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/oauth/refresh', {
                headers,
                body: data
            });
            // On a non-2xx the body lands in response.error (e.g. the backend's
            // {success:false, error:"..."}); surface it so callers can show the
            // real reason and decide whether to guide the user to reauthorize.
            const err = (response as any).error;
            if (err) {
                return {success: false, error: 'Request failed', data: err};
            }
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get available OAuth providers
    oauthProviders: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/oauth/providers', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get OAuth provider configuration
    oauthProviderConfig: async (type: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/oauth/providers/{type}', {
                headers,
                params: {path: {type}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Config Apply API - Safe endpoints that generate config from system state.
    // `preferences` is the source of truth: each key is a Claude Code env
    // var name (e.g. ANTHROPIC_MODEL), and the backend writes them straight
    // into ~/.claude/settings.json under "env".
    applyClaudeConfig: async (preferences: Record<string, string>, installStatusLine?: boolean, defaultMode: string = 'acceptEdits', showThinkingSummaries: boolean = true): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/config/apply/claude', {
            headers,
            body: {preferences, installStatusLine, defaultMode, showThinkingSummaries},
        }));
    },

    applyOpenCodeConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/config/apply/opencode', {headers}));
    },

    getOpenCodeConfigPreview: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/config/preview/opencode', {headers}));
    },

    applyCodexConfig: async (
        preferences?: Record<string, string>,
        writeCatalog?: boolean,
        authMode?: 'apikey' | 'chatgpt' | 'hybrid',
        oauthProviderUuid?: string,
    ): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/config/apply/codex', {
                headers,
                body: {
                    preferences: preferences ?? {},
                    writeCatalog: writeCatalog ?? true,
                    authMode: authMode ?? 'apikey',
                    oauthProviderUuid: oauthProviderUuid ?? '',
                },
            });
            // Callers read `message` (not `error`) on this endpoint — keep the
            // shape but carry the backend's real message instead of a generic.
            if (response.error) {
                return { success: false, message: errorMessage(response.error) };
            }
            return unwrap(response);
        } catch (error: any) {
            return { success: false, message: error?.message || 'Failed to apply Codex configuration' };
        }
    },

    getCodexConfigPreview: async (
        preferences?: Record<string, string>,
        writeCatalog?: boolean,
        authMode?: 'apikey' | 'chatgpt' | 'hybrid',
        oauthProviderUuid?: string,
    ): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/config/preview/codex', {
                headers,
                body: {
                    preferences: preferences ?? {},
                    writeCatalog: writeCatalog ?? true,
                    authMode: authMode ?? 'apikey',
                    oauthProviderUuid: oauthProviderUuid ?? '',
                },
            });
            if (response.error) {
                return { success: false, message: errorMessage(response.error) };
            }
            return unwrap(response);
        } catch (error: any) {
            return { success: false, message: error?.message || 'Failed to preview Codex configuration' };
        }
    },

    applyDshConfig: async (
        preferences?: Record<string, string>,
    ): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await (client as any).POST('/api/v1/config/apply/dsh', {
                headers,
                body: {
                    preferences: preferences ?? {},
                },
            });
            // Callers read `message` (not `error`) on this endpoint — keep the
            // shape but carry the backend's real message instead of a generic.
            if (response.error) {
                return { success: false, message: errorMessage(response.error) };
            }
            return unwrap(response);
        } catch (error: any) {
            return { success: false, message: error?.message || 'Failed to apply DeepSeek Harness configuration' };
        }
    },

    getDshConfigPreview: async (
        preferences?: Record<string, string>,
    ): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await (client as any).POST('/api/v1/config/preview/dsh', {
                headers,
                body: {
                    preferences: preferences ?? {},
                },
            });
            if (response.error) {
                return { success: false, message: errorMessage(response.error) };
            }
            return unwrap(response);
        } catch (error: any) {
            return { success: false, message: error?.message || 'Failed to preview DeepSeek Harness configuration' };
        }
    },

    importCodexOpenAISessions: async (payload: {
        sourceProvider?: string;
        targetProvider?: string;
        codexHome?: string;
        sqliteHome?: string;
        stateDbPath?: string;
        includeArchived?: boolean;
        createBackup?: boolean;
        dryRun?: boolean;
    } = {}): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/codex/import/openai', {
            headers,
            body: payload,
        }));
    },

    // ============================================
    // Skill Management API
    // ============================================

    // Get all skill locations
    getSkillLocations: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/skill-locations', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Add a new skill location
    addSkillLocation: async (data: {
        name: string;
        path: string;
        ide_source: string;
    }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/skill-locations', {
                headers,
                body: data
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get a specific skill location
    getSkillLocation: async (id: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/skill-locations/{id}', {
                headers,
                params: {path: {id}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Remove a skill location
    removeSkillLocation: async (id: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.DELETE('/api/v2/skill-locations/{id}', {
                headers,
                params: {path: {id}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Refresh/scan a skill location
    refreshSkillLocation: async (id: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/skill-locations/{id}/refresh', {
                headers,
                params: {path: {id}}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Discover IDEs with skills
    discoverIdes: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/skill-locations/discover', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Import discovered skill locations
    importSkillLocations: async (locations: any[]): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/skill-locations/import', {
                headers,
                body: {locations}
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Scan all IDE locations for skills (comprehensive scan)
    scanIdes: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/skill-locations/scan', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get skill content with file content
    // NOTE: query params (location_id, skill_id, skill_path) are not yet documented in the OpenAPI spec.
    getSkillContent: async (locationId: string, skillId: string, skillPath?: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/skill-content', {
                headers,
                params: {query: {
                    location_id: locationId,
                    ...(skillId && {skill_id: skillId}),
                    ...(skillPath && {skill_path: skillPath}),
                } as any},
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // ========== ImBot Settings API ==========

    // Get ImBot platform configurations
    getImBotPlatforms: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/imbot-platforms', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // List all ImBot settings
    getImBotSettingsList: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/imbot-settings', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    listBotCapabilities,
    setBotCapability: (botUUID: string, capability: 'notify' | 'remote_control', enabled: boolean) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/capabilities/${capability}`, {
            method: 'PUT', body: JSON.stringify({enabled, config: {}}),
        }),
    listBotDirectChats: (botUUID: string) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/chats`),
    setBotDirectChatBlocked: (botUUID: string, chatID: string, blocked: boolean) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/chats/${encodeURIComponent(chatID)}/blocked`, {
            method: 'PUT', body: JSON.stringify({blocked}),
        }),
    deleteBotDirectChat: (botUUID: string, chatID: string) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/chats/${encodeURIComponent(chatID)}`, {
            method: 'DELETE',
        }),
    setBotDirectChatPermission: (botUUID: string, chatID: string, capability: string, action: string, effect: 'allow' | 'deny') =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/chats/${encodeURIComponent(chatID)}/permissions/${capability}/${action}`, {
            method: 'PUT', body: JSON.stringify({effect}),
        }),
    setBotDirectChatPermissions: (botUUID: string, chatID: string, permissions: Array<{capability: string; action: string; effect: 'allow' | 'deny'}>) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/chats/${encodeURIComponent(chatID)}/permissions`, {
            method: 'PUT', body: JSON.stringify({permissions}),
        }),
    listBotGroups: (botUUID: string) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/groups`),
    getBotGroup: (botUUID: string, groupID: string) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/groups/${encodeURIComponent(groupID)}`),
    setBotGroupBlocked: (botUUID: string, groupID: string, blocked: boolean) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/groups/${encodeURIComponent(groupID)}/blocked`, {
            method: 'PUT', body: JSON.stringify({blocked}),
        }),
    setBotGroupCapability: (botUUID: string, groupID: string, capability: string, effect: 'allow' | 'deny') =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/groups/${encodeURIComponent(groupID)}/capabilities/${capability}`, {
            method: 'PUT', body: JSON.stringify({effect}),
        }),
    addBotGroupActor: (botUUID: string, groupID: string, actorID: string, externalActorID: string, displayName?: string) =>
        botAccessAPI(`/api/v1/bots/${encodeURIComponent(botUUID)}/groups/${encodeURIComponent(groupID)}/actors/${encodeURIComponent(actorID)}`, {
            method: 'PUT', body: JSON.stringify({external_actor_id: externalActorID, display_name: displayName, label: 'Controller'}),
        }),

    // List the chats a bot can reach (GET /api/v1/bots/:bot/chats).
    // Placeholder until codegen regenerates the client SDK for the new
    // bot-interaction endpoint — calls the raw path directly.
    listBotChats: async (botUUID: string): Promise<{chats?: BotChat[]; running?: boolean; error?: string}> => {
        try {
            const base = await getApiBaseUrl();
            const headers = await getAuthHeaders();
            const response = await fetch(`${base}/api/v1/bots/${encodeURIComponent(botUUID)}/chats`, {
                headers: {...headers, 'Content-Type': 'application/json'},
            });
            if (!response.ok) {
                return {error: `failed to list chats (${response.status})`};
            }
            const payload = await response.json();
            return {
                chats: (payload.chats || []).map((item: any) => ({
                    chat_id: item.chat?.external_chat_id,
                    id: item.chat?.id,
                    platform: item.chat?.platform,
                    is_paired: Boolean(item.chat?.peer_actor_id),
                    blocked: item.chat?.blocked,
                    can_notify: (item.permissions || []).some((permission: any) =>
                        permission.capability === 'notify' &&
                        permission.action === 'notify.receive' &&
                        permission.effect === 'allow'),
                })),
                running: true,
            };
        } catch (error: any) {
            return {error: error.message};
        }
    },

    // Send a one-way notification to a running bot's chat
    // (POST /api/v1/bots/:bot/notify). Placeholder until codegen regenerates
    // the client SDK — calls the raw path directly. Field names mirror the
    // backend notifyRequest: stable internal target + body required.
    notifyBot: async (
        botUUID: string,
        body: {target: {kind: 'direct_chat' | 'group'; id: string}; title?: string; body: string; level?: string},
    ): Promise<{ok?: boolean; error?: string}> => {
        try {
            const base = await getApiBaseUrl();
            const headers = await getAuthHeaders();
            const response = await fetch(`${base}/api/v1/bots/${encodeURIComponent(botUUID)}/notify`, {
                method: 'POST',
                headers: {...headers, 'Content-Type': 'application/json'},
                body: JSON.stringify(body),
            });
            if (!response.ok) {
                const data = await response.json().catch(() => null);
                return {error: data?.error || `notify failed (${response.status})`};
            }
            return await response.json();
        } catch (error: any) {
            return {error: error.message};
        }
    },

    // Start an interactive prompt on a running bot's chat
    // (POST /api/v1/bots/:bot/interact). Placeholder until codegen regenerates
    // the client SDK. Returns request_id + wait_url + expires_at, or {error}.
    // Field names mirror backend interactRequest: target/kind/title required,
    // options required for confirm/choose, timeout_seconds optional (≤30m).
    interactBot: async (
        botUUID: string,
        body: {
            target: {kind: 'direct_chat' | 'group'; id: string};
            kind: 'confirm' | 'choose' | 'ask';
            title: string;
            body?: string;
            options?: Array<{value: string; label: string; style?: string}>;
            timeout_seconds?: number;
        },
    ): Promise<{request_id?: string; wait_url?: string; expires_at?: string; error?: string}> => {
        try {
            const base = await getApiBaseUrl();
            const headers = await getAuthHeaders();
            const response = await fetch(`${base}/api/v1/bots/${encodeURIComponent(botUUID)}/interact`, {
                method: 'POST',
                headers: {...headers, 'Content-Type': 'application/json'},
                body: JSON.stringify(body),
            });
            if (!response.ok) {
                const data = await response.json().catch(() => null);
                return {error: data?.error || `interact failed (${response.status})`};
            }
            return await response.json();
        } catch (error: any) {
            return {error: error.message};
        }
    },

    // Long-poll for the reply to an interactive prompt
    // (GET /api/v1/bots/:bot/interact/:request_id?timeout=Ns). Placeholder
    // until codegen. Returns a normalized status:
    //   'answered' | 'cancelled' (200, carries decision)
    //   'timeout' | 'error'     (410, carries decision/reason)
    //   'pending'               (504 — caller retries)
    //   'expired'               (404)
    //   'unavailable'           (503)
    // Transport failures fold into {error} (mirrors runProbe.ts).
    waitBotInteract: async (
        botUUID: string,
        requestID: string,
        timeoutMs = 45000,
    ): Promise<{status?: string; decision?: Record<string, unknown>; reason?: string; error?: string}> => {
        try {
            const base = await getApiBaseUrl();
            const headers = await getAuthHeaders();
            const response = await fetch(
                `${base}/api/v1/bots/${encodeURIComponent(botUUID)}/interact/${encodeURIComponent(requestID)}?timeout=${Math.floor(timeoutMs / 1000)}s`,
                {headers: {...headers, 'Content-Type': 'application/json'}},
            );
            const data = await response.json().catch(() => null);
            if (response.status === 504) return {status: 'pending'};
            if (response.status === 404) return {status: 'expired'};
            if (response.status === 503) return {status: 'unavailable'};
            if (!response.ok) {
                return {error: data?.error || `wait failed (${response.status})`};
            }
            // 200 (answered/cancelled) or 410 (timeout/error) carry a status body.
            return {
                status: data?.status,
                decision: data?.decision,
                reason: data?.reason,
            };
        } catch (error: any) {
            return {error: error.message};
        }
    },

    getImBotSetting: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/imbot-settings/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            if (error.response?.status === 404) {
                return {success: false, error: 'ImBot setting not found'};
            }
            return {success: false, error: error.message};
        }
    },

    createImBotSetting: async (data: {
        name?: string;
        platform: string;
        auth_type: string;
        auth?: Record<string, string>;
        proxy_url?: string;
        chat_id_lock?: string;
        bash_allowlist?: string[];
        default_agent?: string;
        agent_type?: string;
        default_cwd?: string;
        enabled?: boolean;
        require_pairing?: boolean;
    }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/imbot-settings', {
                headers,
                body: data as any
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    updateImBotSetting: async (uuid: string, data: {
        name?: string;
        auth_type?: string;
        auth?: Record<string, string>;
        proxy_url?: string;
        chat_id_lock?: string;
        bash_allowlist?: string[];
        enabled?: boolean;
        default_agent?: string;
        default_cwd?: string;
        require_pairing?: boolean;
        smartguide_provider?: string;
        smartguide_model?: string;
        remote_agent?: boolean;
    }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.PUT('/api/v1/imbot-settings/{uuid}', {
                headers,
                params: {path: {uuid}},
                body: data
            });
            return unwrap(response);
        } catch (error: any) {
            if (error.response?.status === 404) {
                return {success: false, error: 'ImBot setting not found'};
            }
            return {success: false, error: error.message};
        }
    },

    deleteImBotSetting: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.DELETE('/api/v1/imbot-settings/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            if (error.response?.status === 404) {
                return {success: false, error: 'ImBot setting not found'};
            }
            return {success: false, error: error.message};
        }
    },

    restartImBot: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/imbot-admin/restart/{uuid}', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            if (error.response?.status === 404) {
                return {success: false, error: 'ImBot setting not found'};
            }
            return {success: false, error: error.message};
        }
    },

    toggleImBotSetting: async (uuid: string): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/imbot-settings/{uuid}/toggle', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response);
        } catch (error: any) {
            if (error.response?.status === 404) {
                return {success: false, error: 'ImBot setting not found'};
            }
            return {success: false, error: error.message};
        }
    },

    // Reveal current TOFU pairing code (audit-logged on every call).
    getImBotPairingCode: async (uuid: string): Promise<{
        success: boolean;
        active?: boolean;
        code?: string;
        expires_at?: string;
        message?: string;
        error?: string;
    }> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/imbot-settings/{uuid}/pairing-code', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response) as any;
        } catch (error: any) {
            if (error.response?.status === 404) {
                return {success: false, error: 'ImBot setting not found'};
            }
            return {success: false, error: error.message};
        }
    },

    // Mint a fresh TOFU pairing code, invalidating the previous one.
    rotateImBotPairingCode: async (uuid: string): Promise<{
        success: boolean;
        active?: boolean;
        code?: string;
        expires_at?: string;
        message?: string;
        error?: string;
    }> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/imbot-settings/{uuid}/pairing-code/rotate', {
                headers,
                params: {path: {uuid}}
            });
            return unwrap(response) as any;
        } catch (error: any) {
            if (error.response?.status === 404) {
                return {success: false, error: 'ImBot setting not found'};
            }
            return {success: false, error: error.message};
        }
    },

    // User Token Management APIs
    // Get current user token (masked)
    getUserAuthTokenInfo: async (): Promise<{
        success: boolean;
        data?: { token: string; is_default: boolean };
        error?: string
    }> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/auth/token', {headers});
            return {success: true, data: response.data?.data as { token: string; is_default: boolean } | undefined};
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Reset user token to a new secure random value
    resetUserToken: async (): Promise<{ success: boolean; data?: { token: string }; error?: string }> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/auth/token/reset', {headers});
            const data = response.data?.data as { token: string } | undefined;
            if (data?.token) {
                // Update localStorage with new token
                localStorage.setItem('user_auth_token', data.token);
                resetClient();
            }
            return {success: true, data};
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Reset model token to a new secure random value
    resetModelToken: async (): Promise<{ success: boolean; data?: { token: string }; error?: string }> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/auth/model-token/reset', {headers});
            return {success: true, data: (response.data as any)?.data};
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // ========== Weixin QR Login API ==========

    // Start Weixin QR login flow
    weixinQRStart: async (botUUID: string, platform?: string, botName?: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/weixin/qr-start', {
            headers,
            params: {path: {uuid: botUUID}},
            body: {bot_uuid: botUUID, bot_platform: platform, bot_name: botName},
        }));
    },

    // Poll Weixin QR login status
    weixinQRStatus: async (botUUID: string, qrCodeId: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}/weixin/qr-status', {
            headers,
            params: {path: {uuid: botUUID}, query: {qrcode_id: qrCodeId}},
        }));
    },

    // Cancel Weixin QR login flow
    weixinQRCancel: async (botUUID: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/weixin/qr-cancel', {
            headers,
            params: {path: {uuid: botUUID}},
        }));
    },

    // ========== Feishu/Lark One-Click Registration API ==========

    // Start Feishu/Lark one-click app registration; returns a QR verification link
    feishuRegStart: async (botUUID: string, platform?: string, botName?: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/feishu/qr-start', {
            headers,
            params: {path: {uuid: botUUID}},
            body: {bot_uuid: botUUID, bot_platform: platform, bot_name: botName},
        }));
    },

    // Poll Feishu/Lark one-click registration status
    feishuRegStatus: async (botUUID: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}/feishu/qr-status', {
            headers,
            params: {path: {uuid: botUUID}},
        }));
    },

    // Cancel a pending Feishu/Lark one-click registration
    feishuRegCancel: async (botUUID: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/feishu/qr-cancel', {
            headers,
            params: {path: {uuid: botUUID}},
        }));
    },

    // ========== System Configuration API ==========

    // Get system configuration
    getConfig: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/config', {headers});
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Update system configuration
    updateConfig: async (config: any): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.PUT('/api/v1/config', {
                headers,
                body: config
            });
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

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

    // List all registered MCP clients
    listMCPClients: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/mcp/clients', {headers}));
    },

    // Get a specific MCP client by ID
    getMCPClient: async (id: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/mcp/client/{id}', {
            headers,
            params: {path: {id}},
        }));
    },

    // Create a new MCP client
    createMCPClient: async (data: components['schemas']['CreateClientRequest']): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/mcp/client', {
            headers,
            body: data,
        }));
    },

    // Update an MCP client
    updateMCPClient: async (id: string, data: components['schemas']['UpdateClientRequest']): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/mcp/client/{id}', {
            headers,
            params: {path: {id}},
            body: data,
        }));
    },

    // Delete an MCP client
    deleteMCPClient: async (id: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/mcp/client/{id}', {
            headers,
            params: {path: {id}},
        }));
    },

    // Reconnect an MCP client
    reconnectMCPClient: async (id: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/mcp/client/{id}/reconnect', {
            headers,
            params: {path: {id}},
        }));
    },

    // Get install command for an MCP client
    getMCPInstallCommand: async (name: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/mcp/install/{name}', {
            headers,
            params: {path: {name}},
        }));
    },

    // ========== MCP Tool Testing API ==========

    // Execute an MCP tool (for tool testing interface)
    executeMCPTool: async (
        clientId: string,
        toolName: string,
        args: Record<string, unknown>
    ): Promise<{
        success: boolean;
        result?: string;
        error?: string;
        executionTime?: number;
    }> => {
        try {
            return controlApi((client, headers) => client.POST('/api/v1/mcp/execute', {
                headers,
                body: {
                    client_id: clientId,
                    tool_name: toolName,
                    arguments: args,
                },
            }));
        } catch (error: any) {
            return {
                success: false,
                error: error.message || 'Tool execution API not implemented',
            };
        }
    },

    // ============================================
    // API Token Management (Multi-Tenant)
    // ============================================

    // List all API tokens
    listAPITokens: async (params?: {
        user_id?: string;
        team_id?: string;
        enabled?: boolean;
        limit?: number;
        offset?: number;
    }): Promise<any> => {
        const data = await controlApi((client, headers) => client.GET('/api/v1/tokens', {
            headers,
            params: {query: params as any}
        }));
        if (data?.success === false) {
            return data;
        }
        return {success: true, data};
    },

    // Get a specific API token
    getAPIToken: async (tokenId: string): Promise<any> => {
        const data = await controlApi((client, headers) => client.GET('/api/v1/tokens/{token_id}', {
                headers,
                params: {path: {token_id: tokenId}}
            }));
        if (data?.success === false) {
            return data;
        }
        return {success: true, data};
    },

    // Create a new API token
    createAPIToken: async (data: {
        display_name: string;
        team_id?: string;
    }): Promise<any> => {
        const response = await controlApi((client, headers) => client.POST('/api/v1/tokens', {
            headers,
            body: data as any
        }));
        if (response?.success === false) {
            return response;
        }
        return {success: true, data: response};
    },

    // Delete an API token
    deleteAPIToken: async (tokenId: string): Promise<any> => {
        const data = await controlApi((client, headers) => client.DELETE('/api/v1/tokens/{token_id}', {
                headers,
                params: {path: {token_id: tokenId}}
            }));
        if (data?.success === false) {
            return data;
        }
        return {success: true, data};
    },

    // Enable an API token
    setAPITokenEnabled: async (tokenId: string, enabled: boolean): Promise<any> => {
        const endpoint = enabled
                ? '/api/v1/tokens/{token_id}/enable'
                : '/api/v1/tokens/{token_id}/disable';
        const data = await controlApi((client, headers) => client.PUT(endpoint, {
                headers,
                params: {path: {token_id: tokenId}}
            }));
        if (data?.success === false) {
            return data;
        }
        return {success: true, data};
    },

    moveAPITokenToTeam: async (tokenId: string, teamId: string): Promise<any> =>
        teamControlAPI(`/api/v1/tokens/${encodeURIComponent(tokenId)}/team`, {
            method: 'PUT',
            body: JSON.stringify({team_id: teamId}),
        }),

    listTeams: async (): Promise<any> => teamControlAPI('/api/v1/teams'),

    createTeam: async (data: {name: string}): Promise<any> =>
        teamControlAPI('/api/v1/teams', {method: 'POST', body: JSON.stringify(data)}),

    updateTeam: async (teamId: string, data: {name: string}): Promise<any> =>
        teamControlAPI(`/api/v1/teams/${encodeURIComponent(teamId)}`, {
            method: 'PUT',
            body: JSON.stringify(data),
        }),

    setTeamEnabled: async (teamId: string, enabled: boolean): Promise<any> =>
        teamControlAPI(`/api/v1/teams/${encodeURIComponent(teamId)}/${enabled ? 'enable' : 'disable'}`, {
            method: 'PUT',
        }),

    deleteTeam: async (teamId: string): Promise<any> =>
        teamControlAPI(`/api/v1/teams/${encodeURIComponent(teamId)}`, {method: 'DELETE'}),
};

export default api;
