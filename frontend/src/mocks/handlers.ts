import { http, HttpResponse } from 'msw'

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
]

const mockRules = {
    tingly: {
        provider: "openai",
        model: "gpt-3.5-turbo"
    }
}

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
]

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
}

const mockDefaults = {
    request_configs: [
        {
            name: "tingly",
            provider: "openai",
            model: "gpt-3.5-turbo"
        }
    ]
}

// Counter for alternating probe responses
let probeRequestCount = 0

export const handlers = [
    // Remote Agents / Remote Graphs API endpoints
    http.get('/api/remote-agents', () => {
        return HttpResponse.json({
            success: true,
            graphs: mockRemoteGraphs
        })
    }),

    http.get('/api/remote-agents/:uuid', ({ params }) => {
        const { uuid } = params
        const graph = mockRemoteGraphs.find(g => g.uuid === uuid)
        if (graph) {
            return HttpResponse.json({
                success: true,
                graph: graph
            })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.post('/api/remote-agents', async ({ request }) => {
        const body = await request.json() as any
        const newGraph = {
            uuid: `mock-graph-${Date.now()}`,
            name: body.name,
            description: body.description || '',
            connections: [],
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
        }
        mockRemoteGraphs.push(newGraph)
        return HttpResponse.json({
            success: true,
            graph: newGraph
        })
    }),

    http.put('/api/remote-agents/:uuid', async ({ params, request }) => {
        const { uuid } = params
        const body = await request.json() as any
        const graph = mockRemoteGraphs.find(g => g.uuid === uuid)
        if (graph) {
            if (body.name) graph.name = body.name
            if (body.description !== undefined) graph.description = body.description
            graph.updated_at = new Date().toISOString()
            return HttpResponse.json({
                success: true,
                graph: graph
            })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.delete('/api/remote-agents/:uuid', ({ params }) => {
        const { uuid } = params
        const index = mockRemoteGraphs.findIndex(g => g.uuid === uuid)
        if (index >= 0) {
            mockRemoteGraphs.splice(index, 1)
            return HttpResponse.json({
                success: true,
                message: 'Graph deleted successfully'
            })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.post('/api/remote-agents/:agentUuid/bindings', async ({ params, request }) => {
        const { agentUuid } = params
        const body = await request.json() as any
        const graph = mockRemoteGraphs.find(g => g.uuid === agentUuid)
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
            }
            graph.connections.push(newConnection)
            graph.updated_at = new Date().toISOString()
            return HttpResponse.json({
                success: true,
                connection: newConnection
            })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.put('/api/remote-agents/:agentUuid/bindings/:bindingUuid', async ({ params, request }) => {
        const { agentUuid, bindingUuid } = params
        const body = await request.json() as any
        const graph = mockRemoteGraphs.find(g => g.uuid === agentUuid)
        if (graph) {
            const connection = graph.connections.find((c: any) => c.uuid === bindingUuid)
            if (connection) {
                Object.assign(connection, body)
                connection.updated_at = new Date().toISOString()
                graph.updated_at = new Date().toISOString()
                return HttpResponse.json({
                    success: true,
                    connection: connection
                })
            }
            return HttpResponse.json({
                success: false,
                error: 'Connection not found'
            }, { status: 404 })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.delete('/api/remote-agents/:agentUuid/bindings/:bindingUuid', ({ params }) => {
        const { agentUuid, bindingUuid } = params
        const graph = mockRemoteGraphs.find(g => g.uuid === agentUuid)
        if (graph) {
            const index = graph.connections.findIndex((c: any) => c.uuid === bindingUuid)
            if (index >= 0) {
                graph.connections.splice(index, 1)
                graph.updated_at = new Date().toISOString()
                return HttpResponse.json({
                    success: true,
                    message: 'Connection deleted successfully'
                })
            }
            return HttpResponse.json({
                success: false,
                error: 'Connection not found'
            }, { status: 404 })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.put('/api/remote-agents/:agentUuid/bindings/:bindingUuid/guide', async ({ params, request }) => {
        const { agentUuid, bindingUuid } = params
        const body = await request.json() as any
        const graph = mockRemoteGraphs.find(g => g.uuid === agentUuid)
        if (graph) {
            const connection = graph.connections.find((c: any) => c.uuid === bindingUuid)
            if (connection) {
                connection.guide_config = JSON.stringify(body)
                connection.updated_at = new Date().toISOString()
                graph.updated_at = new Date().toISOString()
                return HttpResponse.json({
                    success: true,
                    guide: body
                })
            }
            return HttpResponse.json({
                success: false,
                error: 'Connection not found'
            }, { status: 404 })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.put('/api/remote-agents/:agentUuid/bindings/:bindingUuid/routing-mode', async ({ params, request }) => {
        const { agentUuid, bindingUuid } = params
        const body = await request.json() as any
        const graph = mockRemoteGraphs.find(g => g.uuid === agentUuid)
        if (graph) {
            const connection = graph.connections.find((c: any) => c.uuid === bindingUuid)
            if (connection) {
                connection.routing_mode = body.routing_mode
                connection.updated_at = new Date().toISOString()
                graph.updated_at = new Date().toISOString()
                return HttpResponse.json({
                    success: true,
                    routing_mode: body.routing_mode
                })
            }
            return HttpResponse.json({
                success: false,
                error: 'Connection not found'
            }, { status: 404 })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.put('/api/remote-agents/:agentUuid/bindings/:bindingUuid/agent-config', async ({ params, request }) => {
        const { agentUuid, bindingUuid } = params
        const body = await request.json() as any
        const graph = mockRemoteGraphs.find(g => g.uuid === agentUuid)
        if (graph) {
            const connection = graph.connections.find((c: any) => c.uuid === bindingUuid)
            if (connection) {
                connection.agent_config = body
                connection.updated_at = new Date().toISOString()
                graph.updated_at = new Date().toISOString()
                return HttpResponse.json({
                    success: true,
                    config: body
                })
            }
            return HttpResponse.json({
                success: false,
                error: 'Connection not found'
            }, { status: 404 })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    http.put('/api/remote-agents/:agentUuid/bindings/:bindingUuid/position', async ({ params, request }) => {
        const { agentUuid, bindingUuid } = params
        const body = await request.json() as any
        const graph = mockRemoteGraphs.find(g => g.uuid === agentUuid)
        if (graph) {
            const connection = graph.connections.find((c: any) => c.uuid === bindingUuid)
            if (connection) {
                connection.position = { x: body.x, y: body.y }
                connection.updated_at = new Date().toISOString()
                graph.updated_at = new Date().toISOString()
                return HttpResponse.json({
                    success: true
                })
            }
            return HttpResponse.json({
                success: false,
                error: 'Connection not found'
            }, { status: 404 })
        }
        return HttpResponse.json({
            success: false,
            error: 'Graph not found'
        }, { status: 404 })
    }),

    // Rules API endpoints
    http.get('/api/rules', () => {
        return HttpResponse.json({
            success: true,
            data: mockRules
        })
    }),

    http.get('/api/rules/:name', ({ params }) => {
        const { name } = params
        if (mockRules[name as keyof typeof mockRules]) {
            return HttpResponse.json({
                success: true,
                data: mockRules[name as keyof typeof mockRules]
            })
        }
        return HttpResponse.json({
            success: false,
            error: `Rule '${name}' not found`
        }, { status: 404 })
    }),

    http.post('/api/rules/:name', async ({ params, request }) => {
        const { name } = params
        const body = await request.json() as any
        mockRules[name as keyof typeof mockRules] = body
        return HttpResponse.json({
            success: true,
            data: mockRules[name as keyof typeof mockRules]
        })
    }),

    // Providers API endpoints
    http.get('/api/providers', () => {
        return HttpResponse.json({
            success: true,
            data: mockProviders
        })
    }),

    http.get('/api/provider-models', () => {
        return HttpResponse.json({
            success: true,
            data: mockProviderModels
        })
    }),

    http.post('/api/provider-models/:name', ({ params }) => {
        const { name } = params
        if (mockProviderModels[name as keyof typeof mockProviderModels]) {
            return HttpResponse.json({
                success: true,
                data: mockProviderModels[name as keyof typeof mockProviderModels]
            })
        }
        return HttpResponse.json({
            success: false,
            error: `API Key '${name}' not found`
        }, { status: 404 })
    }),

    http.get('/api/defaults', () => {
        return HttpResponse.json({
            success: true,
            data: mockDefaults
        })
    }),

    http.post('/api/defaults', async ({ request }) => {
        const body = await request.json() as any
        mockDefaults.request_configs = body.request_configs || []
        return HttpResponse.json({
            success: true,
            data: mockDefaults
        })
    }),

    http.post('/api/probe', () => {
        probeRequestCount++
        const currentRule = mockRules.tingly

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
        }

        const processingTime = Math.floor(Math.random() * 1000) + 500
        const isSuccess = probeRequestCount % 2 === 1

        if (isSuccess) {
            const mockResponses = {
                openai: "Hello! I'm your AI assistant powered by OpenAI. How can I help you today? This is a mock response confirming that your rule configuration is working correctly.",
                anthropic: "Hi there! I'm your AI assistant powered by Anthropic. I'm responding to your simple 'hi' message to validate that your rule configuration is functioning properly.",
                default: `Hello! This is a mock response from the probe API, confirming that your rule configuration with provider '${currentRule.provider}' and model '${currentRule.model}' is working correctly.`
            }

            const mockResponse = mockResponses[currentRule.provider as keyof typeof mockResponses] || mockResponses.default

            return HttpResponse.json({
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
            })
        } else {
            const errorTypes = [
                "Authentication failed",
                "Rate limit exceeded",
                "Model not available",
                "Connection timeout",
                "Invalid API key"
            ]

            const randomError = errorTypes[Math.floor(Math.random() * errorTypes.length)]

            return HttpResponse.json({
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
            }, { status: 500 })
        }
    }),

    http.get('/api/status', () => {
        return HttpResponse.json({
            success: true,
            data: {
                status: "mock",
                message: "Running with mock data"
            }
        })
    }),
]
