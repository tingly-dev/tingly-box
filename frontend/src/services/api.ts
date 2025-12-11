// API service layer for communicating with the backend

import { Configuration, DefaultsApi, HistoryApi, ModelsApi, ProvidersApi, RulesApi, ServerApi, TestingApi, TokenApi } from '../client';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '';

// Get user auth token for UI and control API from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

// Get model token for OpenAI/Anthropic API from localStorage
const getModelToken = (): string | null => {
    return localStorage.getItem('model_token');
};

// Create API configuration
const createApiConfig = () => {
    const token = getUserAuthToken();
    return new Configuration({
        basePath: API_BASE_URL || undefined,
        baseOptions: token ? {
            headers: { Authorization: `Bearer ${token}` },
            validateStatus: (status: number) => status < 500, // Don't reject on 4xx errors
        } : {
            validateStatus: (status: number) => status < 500,
        },
    });
};

// Create API instances
const createApiInstances = () => {
    const config = createApiConfig();

    return {
        defaultsApi: new DefaultsApi(config),
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

export const api = {
    // Initialize API instances when needed
    _instances: null as any,

    get _api() {
        if (!this._instances) {
            this._instances = createApiInstances();
        }
        return this._instances;
    },

    // Status endpoints
    getStatus: async () => {
        try {
            const response = await api._api.serverApi.apiV1StatusGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getProviders: async () => {
        try {
            const response = await api._api.providersApi.apiV1ProvidersGet();
            const result = response.data;
            if (result.success && result.data) {
                // Sort providers alphabetically by name to reduce UI changes
                result.data.sort((a: any, b: any) => a.name.localeCompare(b.name));
            }
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getProviderModels: async () => {
        try {
            const response = await api._api.modelsApi.apiV1ProviderModelsGet();
            const result = response.data;
            if (result.success && result.data) {
                // Sort models within each provider alphabetically by model name
                Object.keys(result.data).forEach(providerName => {
                    if (Array.isArray(result.data[providerName])) {
                        result.data[providerName].sort((a: any, b: any) =>
                            (a.model || a.name || '').localeCompare(b.model || b.name || '')
                        );
                    }
                });
                // Sort provider keys alphabetically for consistent ordering
                const sortedData: any = {};
                Object.keys(result.data)
                    .sort((a, b) => a.localeCompare(b))
                    .forEach(providerName => {
                        sortedData[providerName] = result.data[providerName];
                    });
                result.data = sortedData;
            }
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getProviderModelsByName: async (name: string) => {
        try {
            // Note: The generated client has an issue with path parameters
            // We need to manually handle this for now
            const result = await fetchUIAPI(`/provider-models/${name}`, {
                method: 'POST',
            });
            if (result.success && result.data) {
                // Sort models alphabetically by model name to reduce UI changes
                if (Array.isArray(result.data)) {
                    result.data.sort((a: any, b: any) =>
                        (a.model || a.name || '').localeCompare(b.model || b.name || '')
                    );
                }
            }
            return result;
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    getHistory: async (limit?: number) => {
        try {
            const response = await api._api.historyApi.apiV1HistoryGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Provider management
    addProvider: async (data: any) => {
        try {
            const response = await api._api.providersApi.apiV1ProvidersPost(data);
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getProvider: async (name: string) => {
        try {
            // Note: The generated client has an issue with path parameters
            const result = await fetchUIAPI(`/providers/${name}`);
            return result;
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    updateProvider: async (name: string, data: any) => {
        try {
            // Note: The generated client has an issue with path parameters
            const result = await fetchUIAPI(`/providers/${name}`, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    deleteProvider: async (name: string) => {
        try {
            // Note: The generated client has an issue with path parameters
            const result = await fetchUIAPI(`/providers/${name}`, {
                method: 'DELETE',
            });
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    toggleProvider: async (name: string) => {
        try {
            // Note: The generated client has an issue with path parameters
            const result = await fetchUIAPI(`/providers/${name}/toggle`, {
                method: 'POST',
            });
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Server control
    startServer: async () => {
        try {
            const response = await api._api.serverApi.apiV1ServerStartPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    stopServer: async () => {
        try {
            const response = await api._api.serverApi.apiV1ServerStopPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    restartServer: async () => {
        try {
            const response = await api._api.serverApi.apiV1ServerRestartPost();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    generateToken: async (clientId: string) => {
        try {
            const response = await api._api.tokenApi.apiV1TokenPost({ client_id: clientId });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getToken: async () => {
        try {
            const response = await api._api.tokenApi.apiV1TokenGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Model API calls (OpenAI/Anthropic compatible)
    openAIChatCompletions: (data: any) => fetchModelAPI('/openai/v1/chat/completions', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    anthropicMessages: (data: any) => fetchModelAPI('/anthropic/v1/messages', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    listOpenAIModels: () => fetchModelAPI('/openai/v1/models'),
    listAnthropicModels: () => fetchModelAPI('/anthropic/v1/models'),

    // Rules API - Updated for new rule structure with services
    getRules: async () => {
        try {
            const response = await api._api.rulesApi.apiV1RulesGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    getRule: async (uuid: string) => {
        try {
            // Note: The generated client has an issue with path parameters
            const result = await fetchUIAPI(`/rule/${uuid}`);
            return result;
        } catch (error: any) {
            return { success: false, error: error.message };
        }
    },

    createRule: async (data: any) => {
        try {
            // Note: The API uses POST to /rules but generated client expects different structure
            const result = await fetchUIAPI('/rules', {
                method: 'POST',
                body: JSON.stringify(data),
            });
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    updateRule: async (uuid: string, data: any) => {
        try {
            // Note: The generated client has an issue with path parameters
            const result = await fetchUIAPI(`/rule/${uuid}`, {
                method: 'POST',
                body: JSON.stringify(data),
            });
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    deleteRule: async (uuid: string) => {
        try {
            // Note: The generated client has an issue with path parameters
            const result = await fetchUIAPI(`/rule/${uuid}`, {
                method: 'DELETE',
            });
            return result;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    probeRule: async (rule: any, provider: string, model: string) => {
        try {
            const response = await api._api.testingApi.apiV1ProbePost({
                rule: rule,
                provider: provider,
                model: model,
            });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },
    // Service management within rules
    addServiceToRule: (ruleName: string, serviceData: any) => fetchUIAPI(`/rule/${ruleName}/services`, {
        method: 'POST',
        body: JSON.stringify(serviceData),
    }),
    updateServiceInRule: (ruleName: string, serviceIndex: number, serviceData: any) => fetchUIAPI(`/rule/${ruleName}/services/${serviceIndex}`, {
        method: 'PUT',
        body: JSON.stringify(serviceData),
    }),
    deleteServiceFromRule: (ruleName: string, serviceIndex: number) => fetchUIAPI(`/rule/${ruleName}/services/${serviceIndex}`, {
        method: 'DELETE',
    }),

    // Defaults API
    getDefaults: async () => {
        try {
            const response = await api._api.defaultsApi.apiV1DefaultsGet();
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    setDefaults: async (requestConfigs: any[]) => {
        try {
            const response = await api._api.defaultsApi.apiV1DefaultsPost({
                request_configs: requestConfigs,
            });
            return response.data;
        } catch (error: any) {
            if (error.response?.status === 401) {
                localStorage.removeItem('user_auth_token');
                window.location.href = '/login';
                return { success: false, error: 'Authentication required' };
            }
            return { success: false, error: error.message };
        }
    },

    // Token management
    setUserToken: (token: string) => {
        localStorage.setItem('user_auth_token', token);
        // Reset API instances to refresh token
        api._instances = null;
    },
    getUserToken: () => getUserAuthToken(),
    removeUserToken: () => {
        localStorage.removeItem('user_auth_token');
        // Reset API instances to clear token
        api._instances = null;
    },
    setModelToken: (token: string) => {
        localStorage.setItem('model_token', token);
    },
    getModelToken: () => getModelToken(),
    removeModelToken: () => {
        localStorage.removeItem('model_token');
    },
};

export default api;
