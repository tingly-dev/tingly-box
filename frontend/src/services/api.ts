// API service layer for communicating with the backend

import type {components} from '@/client';
import * as botApi from './botApi';
import * as modelApi from './modelApi';
import {getApiBaseUrl} from '../utils/protocol';
import {
    controlApi,
    errorMessage,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
    resetControlApiClient as resetClient,
    unwrap,
} from './openapi';

// IM bot interaction (capabilities/chats/groups/permissions + notify/interact/
// wait) lives in its own module — see botApi.ts for why it follows a
// different contract than the rest of this file.
export {enrichBotsWithCapabilities} from './botApi';

// Get user auth token for UI and control API from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

// Team endpoints don't follow the rest of this file's {success,data}/
// {success,error:string} convention — callers (UseTeamPage.tsx,
// SharingKeysDialog.tsx) read `result.error?.message`, an object. Keep that
// contract here rather than forcing it through controlApi()/unwrap().
async function teamApiCall<T>(
    call: (client: Awaited<ReturnType<typeof getClient>>, headers: Record<string, string>) => Promise<{data?: T; error?: unknown; response: Response}>,
): Promise<{success: boolean; data?: T; error?: {message: string}}> {
    try {
        const client = await getClient();
        const headers = await getAuthHeaders();
        const {data, error, response} = await call(client, headers);
        if (data === undefined || error !== undefined) {
            return {success: false, error: {message: errorMessage(error) || `request failed (${response.status})`}};
        }
        return {success: true, data};
    } catch (error: any) {
        return {success: false, error: {message: error.message || 'Team API request failed'}};
    }
}

// Raw fetch helper for tingly-box's own control-plane API (`/api/v1/...`),
// authenticated with the browser's `user_auth_token`. Resolves the base URL
// through getApiBaseUrl() rather than window.location.origin so it also
// works in GUI/Wails mode, where the frontend's own origin does not
// necessarily match the backend's port.
export async function fetchUIAPI(url: string, options: RequestInit = {}): Promise<any> {
    const base = await getApiBaseUrl();
    const fullUrl = `${base}/api/v1${url}`;

    const token = getUserAuthToken();

    const response = await fetch(fullUrl, {
        ...options,
        headers: {
            'Content-Type': 'application/json',
            ...(token && {Authorization: `Bearer ${token}`}),
            ...options.headers,
        },
    });

    if (!response.ok) {
        // The status rides along so callers can tell "this provider has no
        // quota" (404) from a real failure, and stay quiet about the former.
        throw Object.assign(new Error(`API error: ${response.status}`), {status: response.status});
    }

    return response.json();
}

export const api = {
    // Initialize API client
    initialize: async (): Promise<void> => {
        await getClient();
    },

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

    // Get provider templates (service providers for dropdown)
    getProviderTemplates: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/provider-templates', {headers})),

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

    getHistory: async (limit?: number): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/history', {headers})),

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

    // List virtual models registered in the in-process registries.
    getAvailableVirtualModels: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/vmodel/available-models', {headers}));
    },

    // Server control
    startServer: async (): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/server/start', {headers})),

    stopServer: async (): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/server/stop', {headers})),

    restartServer: async (): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/server/restart', {headers})),

    generateToken: async (clientId: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/token', {
            headers,
            body: {client_id: clientId}
        })),

    getToken: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/token', {headers})),

    // Rules API
    getRules: async (scenario: string): Promise<any> => {
        if (!scenario.trim()) {
            return {success: false, error: 'Scenario is required', data: []};
        }
        const result = await controlApi((client, headers) => client.GET('/api/v1/rules', {
            headers,
            params: {query: {scenario}}
        }));
        return result?.success === false ? {...result, data: result.data ?? []} : result;
    },

    // Every rule across every scenario (the handler only filters when a
    // scenario query is present). Used by surfaces that pick a target from the
    // whole catalog, e.g. the Playground target picker.
    getAllRules: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/rules', {
            headers,
            // An empty scenario query means "no filter" server-side.
            params: {query: {scenario: ''}},
        })),

    getRule: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/rule/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    createRule: async (uuid: string, data: any): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/rule', {
            headers,
            body: data
        })),

    updateRule: async (uuid: string, data: any): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/rule/{uuid}', {
            headers,
            params: {path: {uuid}},
            body: data
        })),

    deleteRule: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v1/rule/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    getRuleFlagRegistry: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/rule/flags/registry', {headers}));
    },

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

    probeModel: async (uuid: string, model: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/probe', {
            headers,
            body: {
                target_type: 'provider' as const,
                provider_uuid: uuid,
                model: model,
                stream: false,
                message: 'Hello, this is a test message. Please respond with a short greeting.',
            }
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

    probeProvider: async (api_style: string, api_base: string, token: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/probe', {
            headers,
            body: {
                target_type: 'provider_config' as const,
                api_style: api_style as any,
                api_base: api_base,
                token: token,
                stream: false,
                message: 'Hello, this is a test message. Please respond with a short greeting.',
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

    healthCheck: async (): Promise<boolean> => {
        try {
            const client = await getClient();
            const response = await client.GET('/api/v1/info/health');
            return response.data?.health === true;
        } catch {
            return false;
        }
    },

    // Model gateway API (OpenAI/Anthropic-compatible) — see modelApi.ts.
    openAIChatCompletions: modelApi.openAIChatCompletions,
    anthropicMessages: modelApi.anthropicMessages,
    listOpenAIModels: modelApi.listOpenAIModels,
    listAnthropicModels: modelApi.listAnthropicModels,
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
    setModelToken: modelApi.setModelToken,
    removeModelToken: modelApi.removeModelToken,

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
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/stats', {
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
        })),

    getUsageTimeSeries: async (params: {
        interval?: string;
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/timeseries', {
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
        })),

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
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/records', {
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
        })),

    getUsagePerformance: async (params: {
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/performance', {
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
        })),

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
    }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/oauth/authorize', {
            headers,
            body: data as any
        })),

    // Get OAuth session status
    oauthStatus: async (session_id: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/oauth/status', {
            headers,
            params: {query: {session_id}}
        })),

    // Cancel an in-progress OAuth session
    oauthCancel: async (data: { session_id: string }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/oauth/cancel', {
            headers,
            body: data
        })),

    // Refresh OAuth token. Deliberately not controlApi(): on a non-2xx the
    // backend's real error body ({success:false, error:"..."}) is preserved
    // under `data` (not just its extracted message) so callers
    // (CredentialPage.tsx) can read response.data?.error and decide whether
    // to guide the user to reauthorize.
    oauthRefresh: async (data: { provider_uuid: string }): Promise<any> => {
        try {
            const client = await getClient();
            const headers = await getAuthHeaders();
            const response = await client.POST('/api/v1/oauth/refresh', {
                headers,
                body: data
            });
            // No declared error response on this route narrows `error` to
            // `never` on the success branch; widen to read it defensively.
            const err = (response as {error?: unknown}).error;
            if (err) {
                return {success: false, error: 'Request failed', data: err};
            }
            return unwrap(response);
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    // Get available OAuth providers
    oauthProviders: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/oauth/providers', {headers})),

    // Get OAuth provider configuration
    oauthProviderConfig: async (type: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/oauth/providers/{type}', {
            headers,
            params: {path: {type}}
        })),

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
            // No declared error response narrows `error` to `never` on the
            // success branch; widen to read it defensively.
            const err = (response as {error?: unknown}).error;
            if (err) {
                return { success: false, message: errorMessage(err) };
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
            // No declared error response narrows `error` to `never` on the
            // success branch; widen to read it defensively.
            const err = (response as {error?: unknown}).error;
            if (err) {
                return { success: false, message: errorMessage(err) };
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
            const response = await client.POST('/api/v1/config/apply/dsh', {
                headers,
                body: {
                    preferences: preferences ?? {},
                },
            });
            // Callers read `message` (not `error`) on this endpoint — keep the
            // shape but carry the backend's real message instead of a generic.
            // No declared error response narrows `error` to `never` on the
            // success branch; widen to read it defensively.
            const err = (response as {error?: unknown}).error;
            if (err) {
                return { success: false, message: errorMessage(err) };
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
            const response = await client.POST('/api/v1/config/preview/dsh', {
                headers,
                body: {
                    preferences: preferences ?? {},
                },
            });
            // No declared error response narrows `error` to `never` on the
            // success branch; widen to read it defensively.
            const err = (response as {error?: unknown}).error;
            if (err) {
                return { success: false, message: errorMessage(err) };
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
    getSkillLocations: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/skill-locations', {headers})),

    // Add a new skill location
    addSkillLocation: async (data: {
        name: string;
        path: string;
        ide_source: string;
    }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/skill-locations', {
            headers,
            body: data
        })),

    // Get a specific skill location
    getSkillLocation: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/skill-locations/{id}', {
            headers,
            params: {path: {id}}
        })),

    // Remove a skill location
    removeSkillLocation: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v2/skill-locations/{id}', {
            headers,
            params: {path: {id}}
        })),

    // Refresh/scan a skill location
    refreshSkillLocation: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/skill-locations/{id}/refresh', {
            headers,
            params: {path: {id}}
        })),

    // Discover IDEs with skills
    discoverIdes: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/skill-locations/discover', {headers})),

    // Import discovered skill locations
    importSkillLocations: async (locations: any[]): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/skill-locations/import', {
            headers,
            body: {locations}
        })),

    // Scan all IDE locations for skills (comprehensive scan)
    scanIdes: async (): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/skill-locations/scan', {headers})),

    // Get skill content with file content
    // NOTE: query params (location_id, skill_id, skill_path) are not yet documented in the OpenAPI spec.
    getSkillContent: async (locationId: string, skillId: string, skillPath?: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/skill-content', {
            headers,
            params: {query: {
                location_id: locationId,
                ...(skillId && {skill_id: skillId}),
                ...(skillPath && {skill_path: skillPath}),
            } as any},
        })),

    // ========== ImBot Settings API ==========

    // Get ImBot platform configurations
    getImBotPlatforms: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-platforms', {headers})),

    // List all ImBot settings
    getImBotSettingsList: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-settings', {headers})),

    // IM bot capabilities/chats/groups/permissions + notify/interact/wait —
    // see botApi.ts.
    listBotCapabilities: botApi.listBotCapabilities,
    setBotCapability: botApi.setBotCapability,
    listBotDirectChats: botApi.listBotDirectChats,
    setBotDirectChatBlocked: botApi.setBotDirectChatBlocked,
    deleteBotDirectChat: botApi.deleteBotDirectChat,
    setBotDirectChatPermission: botApi.setBotDirectChatPermission,
    setBotDirectChatPermissions: botApi.setBotDirectChatPermissions,
    listBotGroups: botApi.listBotGroups,
    getBotGroup: botApi.getBotGroup,
    setBotGroupBlocked: botApi.setBotGroupBlocked,
    setBotGroupCapability: botApi.setBotGroupCapability,
    addBotGroupActor: botApi.addBotGroupActor,
    listBotChats: botApi.listBotChats,
    notifyBot: botApi.notifyBot,
    interactBot: botApi.interactBot,
    waitBotInteract: botApi.waitBotInteract,

    getImBotSetting: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

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
    }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-settings', {
            headers,
            body: data as any
        })),

    // 404 ("ImBot setting not found") already comes back verbatim from the
    // backend in the unwrap()ped error message — no special-casing needed.
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
    }): Promise<any> =>
        controlApi((client, headers) => client.PUT('/api/v1/imbot-settings/{uuid}', {
            headers,
            params: {path: {uuid}},
            body: data
        })),

    deleteImBotSetting: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v1/imbot-settings/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    restartImBot: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-admin/restart/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    toggleImBotSetting: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/toggle', {
            headers,
            params: {path: {uuid}}
        })),

    // Reveal current TOFU pairing code (audit-logged on every call).
    getImBotPairingCode: async (uuid: string): Promise<{
        success: boolean;
        active?: boolean;
        code?: string;
        expires_at?: string;
        message?: string;
        error?: string;
    }> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}/pairing-code', {
            headers,
            params: {path: {uuid}}
        })),

    // Mint a fresh TOFU pairing code, invalidating the previous one.
    rotateImBotPairingCode: async (uuid: string): Promise<{
        success: boolean;
        active?: boolean;
        code?: string;
        expires_at?: string;
        message?: string;
        error?: string;
    }> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/pairing-code/rotate', {
            headers,
            params: {path: {uuid}}
        })),

    // User Token Management APIs
    // Get current user token (masked)
    getUserAuthTokenInfo: async (): Promise<{
        success: boolean;
        data?: { token: string; is_default: boolean };
        error?: string
    }> => controlApi((client, headers) => client.GET('/api/v1/auth/token', {headers})),

    // Reset user token to a new secure random value
    resetUserToken: async (): Promise<{ success: boolean; data?: { token: string }; error?: string }> => {
        const result = await controlApi((client, headers) => client.POST('/api/v1/auth/token/reset', {headers}));
        if (result?.success && result.data?.token) {
            localStorage.setItem('user_auth_token', result.data.token);
            resetClient();
        }
        return result;
    },

    // Reset model token to a new secure random value
    resetModelToken: async (): Promise<{ success: boolean; data?: { token: string }; error?: string }> =>
        controlApi((client, headers) => client.POST('/api/v1/auth/model-token/reset', {headers})),

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
        teamApiCall((client, headers) => client.PUT('/api/v1/tokens/{token_id}/team', {
            headers,
            params: {path: {token_id: tokenId}},
            body: {team_id: teamId},
        })),

    listTeams: async (): Promise<any> =>
        teamApiCall((client, headers) => client.GET('/api/v1/teams', {headers})),

    createTeam: async (data: {name: string}): Promise<any> =>
        teamApiCall((client, headers) => client.POST('/api/v1/teams', {headers, body: data})),

    updateTeam: async (teamId: string, data: {name: string}): Promise<any> =>
        teamApiCall((client, headers) => client.PUT('/api/v1/teams/{team_id}', {
            headers,
            params: {path: {team_id: teamId}},
            body: data,
        })),

    setTeamEnabled: async (teamId: string, enabled: boolean): Promise<any> =>
        teamApiCall((client, headers) => enabled
            ? client.PUT('/api/v1/teams/{team_id}/enable', {headers, params: {path: {team_id: teamId}}})
            : client.PUT('/api/v1/teams/{team_id}/disable', {headers, params: {path: {team_id: teamId}}})),

    deleteTeam: async (teamId: string): Promise<any> =>
        teamApiCall((client, headers) => client.DELETE('/api/v1/teams/{team_id}', {
            headers,
            params: {path: {team_id: teamId}},
        })),
};

export default api;
