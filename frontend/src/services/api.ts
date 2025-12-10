// API service layer for communicating with the backend

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '';

// Get user auth token for UI and control API from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

// Get model token for OpenAI/Anthropic API from localStorage
const getModelToken = (): string | null => {
    return localStorage.getItem('model_token');
};

async function fetchUIAPI(url: string, options: RequestInit = {}): Promise<any> {
    try {
        const fullUrl = url.startsWith('/api/') ? url : `/api${url}`;
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

async function fetchServerAPI(url: string, options: RequestInit = {}): Promise<any> {
    try {
        const fullUrl = url.startsWith('/api/') ? API_BASE_URL + url : url;
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

        return { success: true, data: await response.json() };
    } catch (error) {
        console.error('Server API Error:', error);
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
    // Status endpoints
    getStatus: () => fetchUIAPI('/status'),
    getProviders: async () => {
        const result = await fetchUIAPI('/providers');
        if (result.success && result.data) {
            // Sort providers alphabetically by name to reduce UI changes
            result.data.sort((a: any, b: any) => a.name.localeCompare(b.name));
        }
        return result;
    },
    getProviderModels: async () => {
        const result = await fetchUIAPI('/provider-models');
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
    },
    getProviderModelsByName: async (name: string) => {
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
    },
    getHistory: (limit?: number) => fetchUIAPI(`/history${limit ? `?limit=${limit}` : ''}`),

    // Provider management
    addProvider: (data: any) => fetchUIAPI('/providers', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    getProvider: (name: string) => fetchUIAPI(`/providers/${name}`),
    updateProvider: (name: string, data: any) => fetchUIAPI(`/providers/${name}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    }),
    deleteProvider: (name: string) => fetchUIAPI(`/providers/${name}`, {
        method: 'DELETE',
    }),
    toggleProvider: (name: string) => fetchUIAPI(`/providers/${name}/toggle`, {
        method: 'POST',
    }),

    // Server control
    startServer: (port: number) => fetchServerAPI('/api/server/start', {
        method: 'POST',
        body: JSON.stringify({ port }),
    }),
    stopServer: () => fetchServerAPI('/api/server/stop', { method: 'POST' }),
    restartServer: (port: number) => fetchServerAPI('/api/server/restart', {
        method: 'POST',
        body: JSON.stringify({ port }),
    }),
    generateToken: (clientId: string) => fetchServerAPI(`/api/token`, {
        method: 'POST',
        body: JSON.stringify({ client_id: clientId }),
    }),
    getToken: () => fetchServerAPI('/api/token', { method: 'GET' }),

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
    getRules: () => fetchUIAPI('/rules'),
    getRule: (uuid: string) => fetchUIAPI(`/rule/${uuid}`),
    createRule: (data: any) => fetchUIAPI('/rules', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    updateRule: (uuid: string, data: any) => fetchUIAPI(`/rule/${uuid}`, {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    deleteRule: (uuid: string) => fetchUIAPI(`/rule/${uuid}`, {
        method: 'DELETE',
    }),
    probeRule: (rule, provider, model) => fetchUIAPI('/probe', {
        method: 'POST',
        body: JSON.stringify({ rule, provider, default_model: model }),
    }),
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

    // Token management
    setUserToken: (token: string) => {
        localStorage.setItem('user_auth_token', token);
    },
    getUserToken: () => getUserAuthToken(),
    removeUserToken: () => {
        localStorage.removeItem('user_auth_token');
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
