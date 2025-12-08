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
        url: '/api/probe',
        method: 'post',
        response: () => {
            // Get the current rule configuration
            const currentRule = mockRules.tingly;

            // Simulate a request that would be sent
            const mockRequest = {
                messages: [
                    {
                        role: "user",
                        content: "hi"
                    }
                ],
                model: currentRule.model,
                max_tokens: 100,
                temperature: 0.7
            };

            // Simulate different responses based on the provider
            const mockResponses = {
                openai: "Hello! I'm your AI assistant powered by OpenAI. How can I help you today? This is a mock response confirming that your rule configuration is working correctly.",
                anthropic: "Hi there! I'm your AI assistant powered by Anthropic. I'm responding to your simple 'hi' message to validate that your rule configuration is functioning properly.",
                default: "Hello! This is a mock response from the probe API, confirming that your rule configuration with provider '${currentRule.provider}' and model '${currentRule.model}' is working correctly."
            };

            const mockResponse = mockResponses[currentRule.provider as keyof typeof mockResponses] ||
                                mockResponses.default.replace('${currentRule.provider}', currentRule.provider).replace('${currentRule.model}', currentRule.model);

            // Add some processing time simulation
            const processingTime = Math.floor(Math.random() * 1000) + 500; // 500-1500ms

            return {
                success: true,
                data: {
                    request: {
                        ...mockRequest,
                        provider: currentRule.provider,
                        timestamp: new Date().toISOString(),
                        processing_time_ms: processingTime
                    },
                    response: {
                        content: mockResponse,
                        model: currentRule.model,
                        provider: currentRule.provider,
                        usage: {
                            prompt_tokens: 10,
                            completion_tokens: 25,
                            total_tokens: 35
                        },
                        finish_reason: "stop"
                    },
                    rule_tested: {
                        name: "tingly",
                        provider: currentRule.provider,
                        model: currentRule.model,
                        timestamp: new Date().toISOString()
                    },
                    test_result: {
                        success: true,
                        message: "Rule configuration is valid and working correctly"
                    }
                }
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