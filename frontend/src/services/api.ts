// API service layer for communicating with the backend

import { authEvents } from './authState';

import TinglyService from "@/bindings";
import {
    Configuration,
    HistoryApi,
    InfoApi,
    LogsApi,
    ModelsApi,
    OauthApi,
    ProbeProviderRequestApiStyleEnum,
    type ProbeResponse,
    type ProviderModelsResponse,
    type ProviderResponse,
    ProvidersApi,
    type RuleResponse,
    RulesApi,
    ServerApi,
    SkillsApi,
    TestingApi,
    TokenApi,
    UsageApi,
} from '../client';
import {
    getApiBaseUrl,
    getApiProtocol,
    getDisplayOrigin,
} from '../utils/protocol';

const DEFAULT_BASE_PATH = getDisplayOrigin().replace(/\/+$/, "");

// Type definition for API instances
interface ApiInstances {
    historyApi: HistoryApi;
    modelsApi: ModelsApi;
    providersApi: ProvidersApi;
    rulesApi: RulesApi;
    serverApi: ServerApi;
    skillsApi: SkillsApi;
    testingApi: TestingApi;
    tokenApi: TokenApi;
    infoApi: InfoApi;
    oauthApi: OauthApi;
    logsApi: LogsApi;
    usageApi: UsageApi;
}


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

// Handle 401 Unauthorized response - centralize auth failure handling
const handleAuthFailure = () => {
    localStorage.removeItem('user_auth_token');
    // Notify AuthContext that auth failed (401 occurred)
    authEvents.notifyAuthFailure();
    // Also dispatch custom event for cross-tab sync
    window.dispatchEvent(new CustomEvent('auth-state-change', { detail: { type: 'logout' } }));
};

// Get model token for OpenAI/Anthropic API from localStorage
const getModelToken = (): string | null => {
    return localStorage.getItem('model_token');
};

// Get base URL for API calls using centralized protocol utility
// @deprecated Use getApiBaseUrl from utils/protocol.ts directly
export const getBaseUrl = async (): Promise<string> => {
    return getApiBaseUrl();
}

// Create API configuration
const createApiConfig = async () => {
    let token = getUserAuthToken();

    // Get token from GUI if available
    if (import.meta.env.VITE_PKG_MODE === "gui") {
        const svc = TinglyService;
        if (svc) {
            try {
                const guiToken = await svc.GetUserAuthToken();
                if (guiToken) {
                    token = guiToken;
                }
            } catch (err) {
                console.error('Failed to get GUI token:', err);
            }
        }
    }

    const basePath = await getApiBaseUrl();
    console.log("api config", basePath);

    return new Configuration({
        basePath: basePath,
        baseOptions: token ? {
            headers: { Authorization: `Bearer ${token}` },
            validateStatus: (status: number) => status < 500, // Don't reject on 4xx errors
        } : {
            validateStatus: (status: number) => status < 500,
        },
    });
};

// Create API instances
const createApiInstances = async () => {
    const config = await createApiConfig();

    return {
        historyApi: new HistoryApi(config),
        modelsApi: new ModelsApi(config),
        providersApi: new ProvidersApi(config),
        rulesApi: new RulesApi(config),
        serverApi: new ServerApi(config),
        skillsApi: new SkillsApi(config),
        testingApi: new TestingApi(config),
        tokenApi: new TokenApi(config),
        infoApi: new InfoApi(config),
        oauthApi: new OauthApi(config),
        logsApi: new LogsApi(config),
        usageApi: new UsageApi(config),
    };
};

async function fetchUIAPI(url: string, options: RequestInit = {}): Promise<any> {
    try {
        const fullUrl = url.startsWith('/api/v1') ? url : `/api/v1${url}`;
        const token = getUserAuthToken();

        const headers: Record<string, string> = {
            'Content-Type': 'application/json',
            ...options.headers as Record<string, string>,
        };

        if (token) {
            headers['Authorization'] = `Bearer ${token}`;
        }

        const response = await fetch(fullUrl, {
            headers,
            ...options,
        });

        // Handle 401 Unauthorized - token is invalid or expired
        if (response.status === 401) {
            handleAuthFailure();
            return { success: false, error: 'Authentication required' };
        }

        return await response.json();
    } catch (error) {
        console.error('UI API Error:', error);
        return { success: false, error: (error as Error).message };
    }
}

// Fetch function for model API calls (OpenAI/Anthropic)
async function fetchModelAPI(url: string, options: RequestInit = {}): Promise<any> {
    try {
        const token = getModelToken();

        const headers: Record<string, string> = {
            'Content-Type': 'application/json',
            ...options.headers as Record<string, string>,
        };

        if (token) {
            headers['Authorization'] = `Bearer ${token}`;
        }

        const response = await fetch(url, {
            headers,
            ...options,
        });

        return await response.json();
    } catch (error) {
        console.error('Model API Error:', error);
        return { success: false, error: (error as Error).message };
    }
}


// Initialize API instances immediately
let apiInstances: ApiInstances | null = null;
let initializationPromise: Promise<ApiInstances> | null = null;

// Async initialization function
async function initializeApiInstances(): Promise<ApiInstances> {
    if (!apiInstances) {
        apiInstances = await createApiInstances();
    }
    return apiInstances;
}

// Get API instances (async)
export async function getApiInstances(): Promise<ApiInstances> {
    if (!initializationPromise) {
        initializationPromise = initializeApiInstances();
    }
    return initializationPromise;
}

export const api = {
    // Initialize API instances
    initialize: async (): Promise<void> => {
        if (!initializationPromise) {
            await getApiInstances();
        }
    },

    // Status endpoints
    getStatus: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.serverApi.apiV1StatusGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getProviders: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersGet();
            const body = response.data;
            if (body.success && body.data) {
                // Sort providers alphabetically by name to reduce UI changes
                body.data.sort((a: ProviderResponse, b: ProviderResponse) => a.name.localeCompare(b.name));
            }
            return body;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    updateProviderModelsByUUID: async (uuid: string): Promise<ProviderModelsResponse> => {
        try {
            // Note: The generated client has an issue with path parameters
            // We need to manually handle this for now
            const apiInstances = await getApiInstances();
            const response = await apiInstances.modelsApi.apiV1ProviderModelsUuidPost(uuid);
            const body = response.data
            if (body.success && body.data) {
                // Sort models alphabetically by model name to reduce UI changes
                body.data.models.sort((a: any, b: any) =>
                    a.localeCompare(b)
                );
            }
            return body;
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    getProviderModelsByUUID: async (uuid: string): Promise<ProviderModelsResponse> => {
        try {
            // Note: The generated client has an issue with path parameters
            // We need to manually handle this for now
            const apiInstances = await getApiInstances();
            const response = await apiInstances.modelsApi.apiV1ProviderModelsUuidGet(uuid);
            const body = response.data
            if (body.success && body.data) {
                // Sort models alphabetically by model name to reduce UI changes
                body.data.models.sort((a: any, b: any) =>
                    a.localeCompare(b)
                );
            }
            return body;
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    getHistory: async (limit?: number): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.historyApi.apiV1HistoryGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Provider management
    addProvider: async (data: any, force: boolean = false): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersPost(data, force);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getProvider: async (uuid: string): Promise<ProviderResponse> => {
        // Note: The generated client has an issue with path parameters
        const apiInstances = await getApiInstances();
        const response = await apiInstances.providersApi.apiV2ProvidersUuidGet(uuid);
        return response.data;
    },

    updateProvider: async (uuid: string, data: any): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersUuidPut(uuid, data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    deleteProvider: async (uuid: string): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersUuidDelete(uuid);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    toggleProvider: async (uuid: string): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersUuidTogglePost(uuid);
            return response.data
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Server control
    startServer: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.serverApi.apiV1ServerStartPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    stopServer: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.serverApi.apiV1ServerStopPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    restartServer: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.serverApi.apiV1ServerRestartPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    generateToken: async (clientId: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.tokenApi.apiV1TokenPost({ client_id: clientId });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getToken: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.tokenApi.apiV1TokenGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },


    // Rules API - Updated for new rule structure with services
    getRules: async (scenario: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RulesGet(scenario);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getRule: async (uuid: string): Promise<RuleResponse> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RuleUuidGet(uuid);
            return response.data;
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    createRule: async (uuid: string, data: any): Promise<any> => {
        try {
            // Note: The API uses POST to /rules but generated client expects different structure
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RulePost(data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    updateRule: async (uuid: string, data: any): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RuleUuidPost(uuid, data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    deleteRule: async (uuid: string): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RuleUuidDelete(uuid);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Scenario API
    getScenarios: async (): Promise<any> => {
        return fetchUIAPI('/scenarios');
    },

    getScenarioConfig: async (scenario: string): Promise<any> => {
        return fetchUIAPI(`/scenario/${scenario}`);
    },

    setScenarioConfig: async (scenario: string, config: any): Promise<any> => {
        return fetchUIAPI(`/scenario/${scenario}`, {
            method: 'POST',
            body: JSON.stringify(config),
        });
    },

    getScenarioFlag: async (scenario: string, flag: string): Promise<any> => {
        return fetchUIAPI(`/scenario/${scenario}/flag/${flag}`);
    },

    setScenarioFlag: async (scenario: string, flag: string, value: boolean): Promise<any> => {
        return fetchUIAPI(`/scenario/${scenario}/flag/${flag}`, {
            method: 'PUT',
            body: JSON.stringify({ value }),
        });
    },

    probeModel: async (uuid: string, model: string): Promise<ProbeResponse> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances
                .testingApi.apiV1ProbePost({
                provider: uuid,
                model: model
            });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    probeProvider: async (api_style: string, api_base: string, token: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.testingApi.apiV1ProbeProviderPost({
                name: "placeholder",
                api_style: (api_style) as ProbeProviderRequestApiStyleEnum,
                api_base: api_base,
                token: token
            });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getVersion: async (): Promise<string> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.infoApi.apiV1InfoVersionGet();
            return response.data.data.version;
        } catch (error: any) {
            console.error('Failed to get version:', error);
            return 'Unknown';
        }
    },

    getLatestVersion: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.infoApi.apiV1InfoVersionCheckGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    healthCheck: async (): Promise<boolean> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.infoApi.apiV1InfoHealthGet();
            return response.data.health === true;
        } catch {
            return false;
        }
    },

    // Model API calls (OpenAI/Anthropic compatible)
    openAIChatCompletions: (data: any): Promise<any> => fetchModelAPI('/openai/v1/chat/completions', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    anthropicMessages: (data: any): Promise<any> => fetchModelAPI('/anthropic/v1/messages', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    listOpenAIModels: (): Promise<any> => fetchModelAPI('/openai/v1/models'),
    listAnthropicModels: (): Promise<any> => fetchModelAPI('/anthropic/v1/models'),


    // Service management within rules
    addServiceToRule: (ruleName: string, serviceData: any): Promise<any> => fetchUIAPI(`/rule/${ruleName}/services`, {
        method: 'POST',
        body: JSON.stringify(serviceData),
    }),
    updateServiceInRule: (ruleName: string, serviceIndex: number, serviceData: any): Promise<any> => fetchUIAPI(`/rule/${ruleName}/services/${serviceIndex}`, {
        method: 'PUT',
        body: JSON.stringify(serviceData),
    }),
    deleteServiceFromRule: (ruleName: string, serviceIndex: number): Promise<any> => fetchUIAPI(`/rule/${ruleName}/services/${serviceIndex}`, {
        method: 'DELETE',
    }),
    // Token management
    setUserToken: (token: string): void => {
        localStorage.setItem('user_auth_token', token);
        // Reset API instances to refresh token
        apiInstances = null;
        initializationPromise = null;
    },
    getUserToken: (): string | null => getUserAuthToken(),
    removeUserToken: (): void => {
        localStorage.removeItem('user_auth_token');
        // Reset API instances to clear token
        apiInstances = null;
        initializationPromise = null;
    },
    setModelToken: (token: string): void => {
        localStorage.setItem('model_token', token);
    },
    removeModelToken: (): void => {
        localStorage.removeItem('model_token');
    },

    // Direct access to raw API instances for advanced usage
    // Usage: const { providersApi, modelsApi } = await api.instances();
    instances: getApiInstances,

    // Usage Dashboard API calls
    getUsageStats: async (params: {
        group_by?: string;
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        limit?: number;
    } = {}): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.usageApi.apiV1UsageStatsGet(
                params.group_by as any,
                params.start_time,
                params.end_time,
                params.provider,
                params.model,
                params.scenario,
                undefined, // rule_uuid
                undefined, // status
                params.limit,
                undefined, // sort_by
                undefined, // sort_order
            );
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getUsageTimeSeries: async (params: {
        interval?: string;
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
    } = {}): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.usageApi.apiV1UsageTimeseriesGet(
                params.interval as any,
                params.start_time,
                params.end_time,
                params.provider,
                params.model,
                params.scenario,
            );
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Config Apply API - Safe endpoints that generate config from system state
    applyClaudeConfig: async (mode: string): Promise<any> => {
        return fetchUIAPI('/config/apply/claude', {
            method: 'POST',
            body: JSON.stringify({ mode }),
        });
    },

    applyOpenCodeConfig: async (): Promise<any> => {
        return fetchUIAPI('/config/apply/opencode', {
            method: 'POST',
            body: JSON.stringify({}),
        });
    },

    getOpenCodeConfigPreview: async (): Promise<any> => {
        return fetchUIAPI('/config/preview/opencode', {
            method: 'GET',
        });
    },

    // ============================================
    // Skill Management API
    // ============================================

    // Get all skill locations
    getSkillLocations: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.skillsApi.apiV2SkillLocationsGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Add a new skill location
    addSkillLocation: async (data: {
        name: string;
        path: string;
        ide_source: string;
    }): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.skillsApi.apiV2SkillLocationsPost(data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Get a specific skill location
    getSkillLocation: async (id: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.skillsApi.apiV2SkillLocationsIdGet(id);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Remove a skill location
    removeSkillLocation: async (id: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.skillsApi.apiV2SkillLocationsIdDelete(id);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Refresh/scan a skill location
    refreshSkillLocation: async (id: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.skillsApi.apiV2SkillLocationsIdRefreshPost(id);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Discover IDEs with skills
    discoverIdes: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.skillsApi.apiV2SkillLocationsDiscoverGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Import discovered skill locations
    importSkillLocations: async (locations: any[]): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.skillsApi.apiV2SkillLocationsImportPost({ locations });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Scan all IDE locations for skills (comprehensive scan)
    scanIdes: async (): Promise<any> => {
        // TODO: Regenerate swagger client
        try {
            const token = getUserAuthToken();
            const response = await fetch(`${await getApiBaseUrl()}/api/v2/skill-locations/scan`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    // Get skill content with file content
    getSkillContent: async (locationId: string, skillId: string, skillPath?: string): Promise<any> => {
        try {
            const token = getUserAuthToken();
            const params = new URLSearchParams({
                location_id: locationId,
                ...(skillId && { skill_id: skillId }),
                ...(skillPath && { skill_path: skillPath }),
            });
            const response = await fetch(`${await getApiBaseUrl()}/api/v2/skill-content?${params}`, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                handleAuthFailure();
                return { success: false, error: 'Authentication required' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    // ============================================
    // Remote Coder API
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
            console.error('Remote Coder availability check failed:', error);
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
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                // Remote-coder auth failures should not force UI logout.
                return { success: false, error: 'Authentication required' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
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
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                // Remote-coder auth failures should not force UI logout.
                return { success: false, error: 'Authentication required' };
            }

            if (response.status === 404) {
                return { success: false, error: 'Session not found' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
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
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                return { success: false, error: 'Authentication required' };
            }

            if (response.status === 404) {
                return { success: false, error: 'Session not found' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
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
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                return { success: false, error: 'Authentication required' };
            }

            if (response.status === 404) {
                return { success: false, error: 'Session not found' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    // Update UI/session state for a specific remote-coder session
    updateRemoteCCSessionState: async (sessionId: string, data: { project_path?: string; expanded_messages?: number[] }): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/sessions/${sessionId}/state`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
                body: JSON.stringify(data),
            });

            if (response.status === 401) {
                return { success: false, error: 'Authentication required' };
            }

            if (response.status === 404) {
                return { success: false, error: 'Session not found' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    // Send chat message to remote-coder
    sendRemoteCCChat: async (data: { session_id?: string; message: string; context?: Record<string, any> }): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/chat`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
                body: JSON.stringify(data),
            });

            if (response.status === 401) {
                // Remote-coder auth failures should not force UI logout.
                return { success: false, error: 'Authentication required' };
            }

            if (response.status === 404) {
                return { success: false, error: 'Session not found' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
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
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                return { success: false, error: 'Authentication required' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    // Get remote-coder bot settings
    getRemoteCCBotSettings: async (): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/bot/settings`, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
            });

            if (response.status === 401) {
                return { success: false, error: 'Authentication required' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    // Update remote-coder bot settings
    updateRemoteCCBotSettings: async (data: { token: string; platform?: string; proxy_url?: string; chat_id?: string; bash_allowlist?: string[] }): Promise<any> => {
        try {
            const token = await getRemoteCCAuthToken();
            const baseUrl = api.getRemoteCCBaseUrl();
            const response = await fetch(`${baseUrl}/remote-coder/bot/settings`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                    ...(token && { 'Authorization': `Bearer ${token}` }),
                },
                body: JSON.stringify(data),
            });

            if (response.status === 401) {
                return { success: false, error: 'Authentication required' };
            }

            return await response.json();
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },
};

export default api;
