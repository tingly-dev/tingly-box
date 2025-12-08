import { MockMethod } from 'vite-plugin-mock';

const mockRules = {
    tingly: {
        provider: "openai",
        model: "gpt-3.5-turbo"
    }
};

const mockProviders = [
    {
        name: "openai",
        api_base: "https://api.openai.com",
        api_style: "openai",
        enabled: true
    },
    {
        name: "anthropic",
        api_base: "https://api.anthropic.com",
        api_style: "anthropic",
        enabled: true
    }
];

const mockProviderModels = {
    "openai": {
        models: [
            "gpt-4",
            "gpt-3.5-turbo",
            "gpt-4-turbo"
        ]
    },
    "anthropic": {
        models: [
            "claude-3-opus",
            "claude-3-sonnet",
            "claude-3-haiku"
        ]
    }
};

const mockDefaults = {
    request_configs: [
        {
            name: "tingly",
            provider: "openai",
            model: "gpt-3.5-turbo"
        }
    ]
};

export default [
    // Rules API endpoints
    {
        url: '/api/rules',
        method: 'get',
        response: () => ({
            success: true,
            data: mockRules
        })
    },
    {
        url: '/api/rules/:name',
        method: 'get',
        response: ({ query }: any) => {
            const name = query.name;
            if (mockRules[name as keyof typeof mockRules]) {
                return {
                    success: true,
                    data: mockRules[name as keyof typeof mockRules]
                };
            } else {
                return {
                    success: false,
                    error: `Rule '${name}' not found`
                };
            }
        }
    },
    {
        url: '/api/rules/:name',
        method: 'post',
        response: ({ query, body }: any) => {
            const name = query.name;
            mockRules[name as keyof typeof mockRules] = body;
            return {
                success: true,
                data: mockRules[name as keyof typeof mockRules]
            };
        }
    },

    // Existing API endpoints
    {
        url: '/api/providers',
        method: 'get',
        response: () => ({
            success: true,
            data: mockProviders
        })
    },
    {
        url: '/api/provider-models',
        method: 'get',
        response: () => ({
            success: true,
            data: mockProviderModels
        })
    },
    {
        url: '/api/provider-models/:name',
        method: 'post',
        response: ({ query }: any) => {
            const name = query.name;
            if (mockProviderModels[name as keyof typeof mockProviderModels]) {
                return {
                    success: true,
                    data: mockProviderModels[name as keyof typeof mockProviderModels]
                };
            } else {
                return {
                    success: false,
                    error: `Provider '${name}' not found`
                };
            }
        }
    },
    {
        url: '/api/defaults',
        method: 'get',
        response: () => ({
            success: true,
            data: mockDefaults
        })
    },
    {
        url: '/api/defaults',
        method: 'post',
        response: ({ body }: any) => {
            mockDefaults.request_configs = body.request_configs || [];
            return {
                success: true,
                data: mockDefaults
            };
        }
    },
    {
        url: '/api/status',
        method: 'get',
        response: () => ({
            success: true,
            data: {
                status: "mock",
                message: "Running with mock data"
            }
        })
    }
] as MockMethod[]