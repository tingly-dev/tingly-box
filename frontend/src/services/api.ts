// API service layer for communicating with the backend

import TinglyService from "@/bindings";
import type {components} from '@/client';
import {getApiBaseUrl} from '../utils/protocol';
import {
    controlApi,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
    resetControlApiClient as resetClient,
} from './openapi';

// Get user auth token for UI and control API from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

// Get user auth token for remote-coder calls (also consult GUI binding)
const getRemoteCCAuthToken = async (): Promise<string | null> => {
    let token = getUserAuthToken();
    if (!token && import.meta.env.VITE_PKG_MODE === "gui") {
        const svc = TinglyService;
        if (svc) {
            try {
                const guiToken = await svc.GetUserAuthToken();
                if (guiToken) {
                    token = guiToken;
                }
            } catch (err) {
                console.error('Failed to get GUI token for remote-coder:', err);
            }
        }
    }
    return token;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getProviders: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v2/providers', {headers});
            const body = response.data;
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
            // openapi-fetch returns { data, error, response }
            // Check for error in response first
            if (response.error) {
                return {success: false, error: 'Request failed'};
            }
            return response.data;
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
            const body = response.data;
            // Model ordering is authoritative from the backend
            // (config.SortProviderModels); do not re-sort here.
            return body;
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
            const body = response.data;
            // Model ordering is authoritative from the backend
            // (config.SortProviderModels); do not re-sort here.
            return body;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getHistory: async (limit?: number): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/history', {headers});
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    stopServer: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/server/stop', {headers});
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    restartServer: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/server/restart', {headers});
            return response.data;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getToken: async (): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.GET('/api/v1/token', {headers});
            return response.data;
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
            // openapi-fetch returns { data, error, response }
            if (response.error) {
                return {success: false, error: 'Request failed', data: []};
            }
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    getRuleFlagRegistry: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/rule/flags/registry', {headers}));
    },

    // Imports providers from a base64/JSONL export bundle.
    importProvider: async (data: string, onProviderConflict: string = 'use'): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v2/provider-import', {
                headers,
                body: {
                    data,
                    on_provider_conflict: onProviderConflict,
                },
            });
            return response.data;
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
            return response.data;
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
                    test_mode: 'simple' as const,
                    message: 'Hello, this is a test message. Please respond with a short greeting.',
                }
            });
            return response.data;
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
            return response.data;
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
                    test_mode: 'simple' as const,
                    message: 'Hello, this is a test message. Please respond with a short greeting.',
                }
            });
            return response.data;
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
            // openapi-fetch returns { data, error, response }
            if (response.error) {
                return {success: false, error: 'Request failed'};
            }
            return response.data;
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
                    }
                }
            });
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Config Apply API - Safe endpoints that generate config from system state.
    // `preferences` is the source of truth: each key is a Claude Code env
    // var name (e.g. ANTHROPIC_MODEL), and the backend writes them straight
    // into ~/.claude/settings.json under "env".
    applyClaudeConfig: async (preferences: Record<string, string>, installStatusLine?: boolean, defaultMode: string = 'acceptEdits'): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/config/apply/claude', {
            headers,
            body: {preferences, installStatusLine, defaultMode},
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
            if (response.error) {
                return { success: false, message: 'Failed to apply Codex configuration' };
            }
            return response.data;
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
                return { success: false, message: 'Failed to preview Codex configuration' };
            }
            return response.data;
        } catch (error: any) {
            return { success: false, message: error?.message || 'Failed to preview Codex configuration' };
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // ============================================
    // Remote Control API (Session management only)
    // ============================================

    // Get the base URL for remote-coder service
    getRemoteCCBaseUrl: (): string => {
        return `${window.location.protocol}//${window.location.hostname}:18080`;
    },

    // Check if remote-coder service is available
    checkRemoteCCAvailable: async (): Promise<boolean> => {
        try {
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/available`, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                },
            });
            const data = await response.json();
            return data.available === true;
        } catch (error: any) {
            console.error('Remote Control availability check failed:', error);
            return false;
        }
    },

    // Get remote-coder sessions
    getRemoteCCSessions: async (params: { page?: number; limit?: number; status?: string } = {}): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const queryParams = new URLSearchParams();
            if (params.page) queryParams.set('page', params.page.toString());
            if (params.limit) queryParams.set('limit', params.limit.toString());
            if (params.status) queryParams.set('status', params.status.toString());

            const baseUrl = api.getRemoteCCBaseUrl();
            const url = `${baseUrl}/remote-coder/sessions${queryParams.toString() ? `?${queryParams.toString()}` : ''}`;
            const response = await fetch(url, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && {'Authorization': `Bearer ${token}`}),
                },
            });

            if (response.status === 401) {
                // Remote-coder auth failures should not force UI logout.
                return {success: false, error: 'Authentication required'};
            }

            return await response.json();
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get a specific remote-coder session
    getRemoteCCSession: async (sessionId: string): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/sessions/${sessionId}`, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && {'Authorization': `Bearer ${token}`}),
                },
            });

            if (response.status === 401) {
                // Remote-coder auth failures should not force UI logout.
                return {success: false, error: 'Authentication required'};
            }

            if (response.status === 404) {
                return {success: false, error: 'Session not found'};
            }

            return await response.json();
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get messages for a specific remote-coder session
    getRemoteCCSessionMessages: async (sessionId: string): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/sessions/${sessionId}/messages`, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && {'Authorization': `Bearer ${token}`}),
                },
            });

            if (response.status === 401) {
                return {success: false, error: 'Authentication required'};
            }

            if (response.status === 404) {
                return {success: false, error: 'Session not found'};
            }

            return await response.json();
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get UI/session state for a specific remote-coder session
    getRemoteCCSessionState: async (sessionId: string): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/sessions/${sessionId}/state`, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && {'Authorization': `Bearer ${token}`}),
                },
            });

            if (response.status === 401) {
                return {success: false, error: 'Authentication required'};
            }

            if (response.status === 404) {
                return {success: false, error: 'Session not found'};
            }

            return await response.json();
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Update UI/session state for a specific remote-coder session
    updateRemoteCCSessionState: async (sessionId: string, data: {
        project_path?: string;
        expanded_messages?: number[]
    }): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/sessions/${sessionId}/state`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && {'Authorization': `Bearer ${token}`}),
                },
                body: JSON.stringify(data),
            });

            if (response.status === 401) {
                return {success: false, error: 'Authentication required'};
            }

            if (response.status === 404) {
                return {success: false, error: 'Session not found'};
            }

            return await response.json();
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Send chat message to remote-coder
    sendRemoteCCChat: async (data: {
        session_id?: string;
        message: string;
        context?: Record<string, any>
    }): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/chat`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && {'Authorization': `Bearer ${token}`}),
                },
                body: JSON.stringify(data),
            });

            if (response.status === 401) {
                // Remote-coder auth failures should not force UI logout.
                return {success: false, error: 'Authentication required'};
            }

            if (response.status === 404) {
                return {success: false, error: 'Session not found'};
            }

            return await response.json();
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Clear all remote-coder sessions
    clearRemoteCCSessions: async (): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/sessions/clear`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && {'Authorization': `Bearer ${token}`}),
                },
            });

            if (response.status === 401) {
                return {success: false, error: 'Authentication required'};
            }

            return await response.json();
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
            return response.data;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
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
            return response.data;
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
        chat_id?: string;
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
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    updateImBotSetting: async (uuid: string, data: {
        name?: string;
        auth_type?: string;
        auth?: Record<string, string>;
        proxy_url?: string;
        chat_id?: string;
        bash_allowlist?: string[];
        enabled?: boolean;
        default_agent?: string;
        default_cwd?: string;
        require_pairing?: boolean;
    }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.PUT('/api/v1/imbot-settings/{uuid}', {
                headers,
                params: {path: {uuid}},
                body: data
            });
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data;
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
            return response.data as any;
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
            return response.data as any;
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
            return response.data;
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
            return response.data;
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
        enabled?: boolean;
        limit?: number;
        offset?: number;
    }): Promise<any> => {
        const data = await controlApi((client, headers) => client.GET('/api/v1/tokens', {
                headers,
                params: {query: params}
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
    }): Promise<any> => {
        const response = await controlApi((client, headers) => client.POST('/api/v1/tokens', {
                headers,
                body: data
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
};

export default api;
