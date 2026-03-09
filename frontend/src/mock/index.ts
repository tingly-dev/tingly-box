import { MockMethod } from 'vite-plugin-mock';

// Mock remote graphs data
const mockRemoteGraphs: any[] = [
    {
        uuid: 'mock-graph-001',
        name: 'Default Graph',
        description: 'Default remote graph for agent connections',
        connections: [
            {
                uuid: 'mock-connection-001',
                graph_uuid: 'mock-graph-001',
                imbot_uuid: 'mock-imbot-001',
                agent_uuid: 'mock-agent-001',
                guide_config: null,
                agent_config: {
                    uuid: 'mock-agent-001',
                    name: 'Claude Code Agent',
                    agent_type: 'claude-code',
                    system_prompt: 'You are a helpful coding assistant.',
                    temperature: 0.7,
                    max_tokens: 4096,
                    tools: ['read', 'write', 'execute'],
                    enabled: true,
                },
                routing_mode: 'direct',
                enabled: true,
                status: 'active',
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString(),
            }
        ],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
    }
];

// Helper to find graph by UUID
const findGraph = (uuid: string) => mockRemoteGraphs.find(g => g.uuid === uuid);

const mockRules = {
    tingly: {
        provider: "openai",
        model: "gpt-3.5-turbo"
    }
};

// Counter for alternating probe responses
let probeRequestCount = 0;

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
    // ============================================
    // Remote Agents / Remote Graphs API endpoints
    // ============================================

    // List all remote agents
    {
        url: '/api/remote-agents',
        method: 'get',
        response: () => ({
            success: true,
            graphs: mockRemoteGraphs
        })
    },

    // Get a specific remote agent
    {
        url: '/api/remote-agents/:uuid',
        method: 'get',
        response: ({ query }: any) => {
            const uuid = query.uuid;
            const graph = findGraph(uuid);
            if (graph) {
                return {
                    success: true,
                    graph: graph
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Create a new remote agent
    {
        url: '/api/remote-agents',
        method: 'post',
        response: ({ body }: any) => {
            const newGraph = {
                uuid: `mock-graph-${Date.now()}`,
                name: body.name,
                description: body.description || '',
                connections: [],
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString(),
            };
            mockRemoteGraphs.push(newGraph);
            return {
                success: true,
                graph: newGraph
            };
        }
    },

    // Update a remote agent
    {
        url: '/api/remote-agents/:uuid',
        method: 'put',
        response: ({ query, body }: any) => {
            const uuid = query.uuid;
            const graph = findGraph(uuid);
            if (graph) {
                if (body.name) graph.name = body.name;
                if (body.description !== undefined) graph.description = body.description;
                graph.updated_at = new Date().toISOString();
                return {
                    success: true,
                    graph: graph
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Delete a remote agent
    {
        url: '/api/remote-agents/:uuid',
        method: 'delete',
        response: ({ query }: any) => {
            const uuid = query.uuid;
            const index = mockRemoteGraphs.findIndex(g => g.uuid === uuid);
            if (index >= 0) {
                mockRemoteGraphs.splice(index, 1);
                return {
                    success: true,
                    message: 'Graph deleted successfully'
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Create a binding in a remote agent
    {
        url: '/api/remote-agents/:agentUuid/bindings',
        method: 'post',
        response: ({ query, body }: any) => {
            const agentUuid = query.agentUuid;
            const graph = findGraph(agentUuid);
            if (graph) {
                const newConnection = {
                    uuid: `mock-connection-${Date.now()}`,
                    graph_uuid: agentUuid,
                    imbot_uuid: body.imbot_uuid,
                    agent_uuid: body.agent_uuid,
                    guide_config: null,
                    agent_config: body.agent_config,
                    routing_mode: 'direct',
                    enabled: true,
                    status: 'active',
                    created_at: new Date().toISOString(),
                    updated_at: new Date().toISOString(),
                };
                graph.connections.push(newConnection);
                graph.updated_at = new Date().toISOString();
                return {
                    success: true,
                    connection: newConnection
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Update a binding
    {
        url: '/api/remote-agents/:agentUuid/bindings/:bindingUuid',
        method: 'put',
        response: ({ query, body }: any) => {
            const agentUuid = query.agentUuid;
            const bindingUuid = query.bindingUuid;
            const graph = findGraph(agentUuid);
            if (graph) {
                const connection = graph.connections.find((c: any) => c.uuid === bindingUuid);
                if (connection) {
                    Object.assign(connection, body);
                    connection.updated_at = new Date().toISOString();
                    graph.updated_at = new Date().toISOString();
                    return {
                        success: true,
                        connection: connection
                    };
                }
                return {
                    success: false,
                    error: 'Connection not found'
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Delete a binding
    {
        url: '/api/remote-agents/:agentUuid/bindings/:bindingUuid',
        method: 'delete',
        response: ({ query }: any) => {
            const agentUuid = query.agentUuid;
            const bindingUuid = query.bindingUuid;
            const graph = findGraph(agentUuid);
            if (graph) {
                const index = graph.connections.findIndex((c: any) => c.uuid === bindingUuid);
                if (index >= 0) {
                    graph.connections.splice(index, 1);
                    graph.updated_at = new Date().toISOString();
                    return {
                        success: true,
                        message: 'Connection deleted successfully'
                    };
                }
                return {
                    success: false,
                    error: 'Connection not found'
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Set guide agent for a binding
    {
        url: '/api/remote-agents/:agentUuid/bindings/:bindingUuid/guide',
        method: 'put',
        response: ({ query, body }: any) => {
            const agentUuid = query.agentUuid;
            const bindingUuid = query.bindingUuid;
            const graph = findGraph(agentUuid);
            if (graph) {
                const connection = graph.connections.find((c: any) => c.uuid === bindingUuid);
                if (connection) {
                    connection.guide_config = JSON.stringify(body);
                    connection.updated_at = new Date().toISOString();
                    graph.updated_at = new Date().toISOString();
                    return {
                        success: true,
                        guide: body
                    };
                }
                return {
                    success: false,
                    error: 'Connection not found'
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Set routing mode for a binding
    {
        url: '/api/remote-agents/:agentUuid/bindings/:bindingUuid/routing-mode',
        method: 'put',
        response: ({ query, body }: any) => {
            const agentUuid = query.agentUuid;
            const bindingUuid = query.bindingUuid;
            const graph = findGraph(agentUuid);
            if (graph) {
                const connection = graph.connections.find((c: any) => c.uuid === bindingUuid);
                if (connection) {
                    connection.routing_mode = body.routing_mode;
                    connection.updated_at = new Date().toISOString();
                    graph.updated_at = new Date().toISOString();
                    return {
                        success: true,
                        routing_mode: body.routing_mode
                    };
                }
                return {
                    success: false,
                    error: 'Connection not found'
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Update agent configuration for a binding
    {
        url: '/api/remote-agents/:agentUuid/bindings/:bindingUuid/agent-config',
        method: 'put',
        response: ({ query, body }: any) => {
            const agentUuid = query.agentUuid;
            const bindingUuid = query.bindingUuid;
            const graph = findGraph(agentUuid);
            if (graph) {
                const connection = graph.connections.find((c: any) => c.uuid === bindingUuid);
                if (connection) {
                    connection.agent_config = body;
                    connection.updated_at = new Date().toISOString();
                    graph.updated_at = new Date().toISOString();
                    return {
                        success: true,
                        config: body
                    };
                }
                return {
                    success: false,
                    error: 'Connection not found'
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // Update binding position
    {
        url: '/api/remote-agents/:agentUuid/bindings/:bindingUuid/position',
        method: 'put',
        response: ({ query, body }: any) => {
            const agentUuid = query.agentUuid;
            const bindingUuid = query.bindingUuid;
            const graph = findGraph(agentUuid);
            if (graph) {
                const connection = graph.connections.find((c: any) => c.uuid === bindingUuid);
                if (connection) {
                    connection.position = { x: body.x, y: body.y };
                    connection.updated_at = new Date().toISOString();
                    graph.updated_at = new Date().toISOString();
                    return {
                        success: true
                    };
                }
                return {
                    success: false,
                    error: 'Connection not found'
                };
            }
            return {
                success: false,
                error: 'Graph not found'
            };
        }
    },

    // ============================================
    // Existing API endpoints (Rules, Providers, etc.)
    // ============================================

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
                    error: `API Key '${name}' not found`
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
            // Increment request counter
            probeRequestCount++;

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

            // Add some processing time simulation
            const processingTime = Math.floor(Math.random() * 1000) + 500; // 500-1500ms

            // Alternate between success and failure
            const isSuccess = probeRequestCount % 2 === 1;

            if (isSuccess) {
                // Success response
                const mockResponses = {
                    openai: "Hello! I'm your AI assistant powered by OpenAI. How can I help you today? This is a mock response confirming that your rule configuration is working correctly.",
                    anthropic: "Hi there! I'm your AI assistant powered by Anthropic. I'm responding to your simple 'hi' message to validate that your rule configuration is functioning properly.",
                    default: "Hello! This is a mock response from the probe API, confirming that your rule configuration with provider '${currentRule.provider}' and model '${currentRule.model}' is working correctly."
                };

                const mockResponse = mockResponses[currentRule.provider as keyof typeof mockResponses] ||
                                    mockResponses.default.replace('${currentRule.provider}', currentRule.provider).replace('${currentRule.model}', currentRule.model);

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
            } else {
                // Failure response
                const errorTypes = [
                    "Authentication failed",
                    "Rate limit exceeded",
                    "Model not available",
                    "Connection timeout",
                    "Invalid API key"
                ];

                const randomError = errorTypes[Math.floor(Math.random() * errorTypes.length)];

                return {
                    success: false,
                    error: {
                        code: "PROBE_FAILED",
                        message: randomError,
                        details: {
                            provider: currentRule.provider,
                            model: currentRule.model,
                            timestamp: new Date().toISOString(),
                            processing_time_ms: processingTime
                        }
                    },
                    data: {
                        request: {
                            ...mockRequest,
                            provider: currentRule.provider,
                            timestamp: new Date().toISOString(),
                            processing_time_ms: processingTime
                        },
                        response: {
                            content: null,
                            model: currentRule.model,
                            provider: currentRule.provider,
                            usage: {
                                prompt_tokens: 0,
                                completion_tokens: 0,
                                total_tokens: 0
                            },
                            finish_reason: "error",
                            error: randomError
                        },
                        rule_tested: {
                            name: "tingly",
                            provider: currentRule.provider,
                            model: currentRule.model,
                            timestamp: new Date().toISOString()
                        },
                        test_result: {
                            success: false,
                            message: `Probe failed: ${randomError}`
                        }
                    }
                };
            }
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
