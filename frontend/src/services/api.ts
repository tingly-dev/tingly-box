// API service layer for communicating with the backend

import ProxyService from "@/bindings";
import {
    Configuration,
    type FetchProviderModelsResponse,
    HistoryApi,
    ModelsApi,
    ProbeProviderRequestApiStyleEnum,
    type ProviderResponse,
    ProvidersApi,
    RulesApi,
    ServerApi,
    TestingApi,
    TokenApi
} from '../client';


const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '';

// Type definition for API instances
interface ApiInstances {
    historyApi: HistoryApi;
    modelsApi: ModelsApi;
    providersApi: ProvidersApi;
    rulesApi: RulesApi;
    serverApi: ServerApi;
    testingApi: TestingApi;
    tokenApi: TokenApi;
}


// Get user auth token for UI and control API from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

// Get model token for OpenAI/Anthropic API from localStorage
const getModelToken = (): string | null => {
    return localStorage.getItem('model_token');
};

export const getBaseUrl = async (): Promise<string> => {
    let basePath = API_BASE_URL || "";

    // Check if we're in GUI mode
    if (import.meta.env.VITE_PKG_MODE === "gui") {
        const proxy = ProxyService;
        if (proxy) {
            console.log(proxy)
            try {
                const port = await proxy.GetPort();
                basePath = `http://localhost:${port}`;
                console.log("Using GUI mode base path:", basePath);
            } catch (err) {
                console.error('Failed to get port from ProxyService:', err);
            }
        }
    } else {
        const host = window.location.host.replace(/\/$/, "")
        basePath = `http://${host}`
    }

    return basePath
}

// Create API configuration
const createApiConfig = async () => {
    let token = getUserAuthToken();
    let basePath = API_BASE_URL || undefined;

    // Check if we're in GUI mode
    if (import.meta.env.VITE_PKG_MODE === "gui") {
        const proxy = ProxyService;
        if (proxy) {
            try {
                // Get token from GUI
                const guiToken = await proxy.GetUserAuthToken();
                if (guiToken) {
                    token = guiToken;
                    console.log("Using GUI mode token");
                }

                // Get port and construct base path
                const port = await proxy.GetPort();
                basePath = `http://localhost:${port}`;
                console.log("Using GUI mode base path:", basePath);
            } catch (err) {
                console.error('Failed to get configuration from ProxyService:', err);
            }
        }
    }

    return new Configuration({
        basePath: basePath,
        baseOptions: token ? {
            headers: {Authorization: `Bearer ${token}`},
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
        testingApi: new TestingApi(config),
        tokenApi: new TokenApi(config),
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
            localStorage.removeItem('user_auth_token');
            window.location.href = '/login';
            return {success: false, error: 'Authentication required'};
        }

        return await response.json();
    } catch (error) {
        console.error('UI API Error:', error);
        return {success: false, error: (error as Error).message};
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
        return {success: false, error: (error as Error).message};
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
async function getApiInstances(): Promise<ApiInstances> {
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
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
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
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    updateProviderModelsByUUID: async (uuid: string): Promise<FetchProviderModelsResponse> => {
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
            return {success: false, error: error.message};
        }
    },

    getProviderModelsByUUID: async (uuid: string): Promise<FetchProviderModelsResponse> => {
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
            return {success: false, error: error.message};
        }
    },

    getHistory: async (limit?: number): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.historyApi.apiV1HistoryGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    // Provider management
    addProvider: async (data: any): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersPost(data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    getProvider: async (name: string): Promise<ProviderResponse> => {
        // Note: The generated client has an issue with path parameters
        const apiInstances = await getApiInstances();
        const response = await apiInstances.providersApi.apiV2ProvidersUuidGet(name);
        return response.data;
    },

    updateProvider: async (name: string, data: any): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersUuidPut(name, data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    deleteProvider: async (name: string): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersUuidDelete(name);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    toggleProvider: async (name: string): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.providersApi.apiV2ProvidersUuidTogglePost(name);
            return response.data
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
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
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    stopServer: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.serverApi.apiV1ServerStopPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    restartServer: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.serverApi.apiV1ServerRestartPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    generateToken: async (clientId: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.tokenApi.apiV1TokenPost({client_id: clientId});
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    getToken: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.tokenApi.apiV1TokenGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },


    // Rules API - Updated for new rule structure with services
    getRules: async (): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RulesGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    getRule: async (uuid: string): Promise<any> => {
        try {
            // Note: The generated client has an issue with path parameters
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RuleUuidGet(uuid);
            return response.data;
        } catch (error: any) {
            return {success: false, error: error.message};
        }
    },

    createRule: async (uuid: string, data: any): Promise<any> => {
        try {
            // Note: The API uses POST to /rules but generated client expects different structure
            const apiInstances = await getApiInstances();
            const response = await apiInstances.rulesApi.apiV1RuleUuidPost(uuid, data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
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
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
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
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
        }
    },

    probeModel: async (provider: string, model: string): Promise<any> => {
        try {
            const apiInstances = await getApiInstances();
            const response = await apiInstances.testingApi.apiV1ProbePost({
                provider: provider,
                model: model
            });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
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
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return {success: false, error: 'Authentication required'};
            }
            return {success: false, error: error.message};
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
};

export default api;
