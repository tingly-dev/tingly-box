import { http, HttpResponse } from 'msw'

// ============================================
// Mock Providers (v2 API with uuid)
// ============================================
const mockV2Providers = [
    {
        uuid: 'mock-provider-anthropic',
        name: 'Anthropic',
        api_base: 'https://api.anthropic.com',
        api_style: 'anthropic',
        auth_type: 'api_key',
        token: 'sk-ant-****abcd',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
    },
    {
        uuid: 'mock-provider-openai',
        name: 'OpenAI',
        api_base: 'https://api.openai.com/v1',
        api_style: 'openai',
        auth_type: 'api_key',
        token: 'sk-****efgh',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
    },
    {
        uuid: 'mock-provider-openrouter',
        name: 'OpenRouter',
        api_base: 'https://openrouter.ai/api/v1',
        api_style: 'openai',
        auth_type: 'api_key',
        token: 'sk-or-****ijkl',
        enabled: false,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
    },
    {
        uuid: 'mock-provider-glm',
        name: 'GLM',
        api_base: 'https://open.bigmodel.cn/api/paas/v4',
        api_style: 'anthropic',
        auth_type: 'api_key',
        token: 'glm-****mnop',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
    },
    {
        uuid: 'mock-provider-deepseek',
        name: 'Deepseek',
        api_base: 'https://api.deepseek.com/v1',
        api_style: 'openai',
        auth_type: 'api_key',
        token: 'sk-ds-****qrst',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
    },
    {
        uuid: 'mock-provider-gemini',
        name: 'Gemini',
        api_base: 'https://generativelanguage.googleapis.com/v1beta/openai',
        api_style: 'openai',
        auth_type: 'api_key',
        token: 'AIza****uvwx',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
    },
]

// ============================================
// Mock Rules per scenario
// ============================================
const mockV1Rules: Record<string, any[]> = {
    openai: [
        {
            uuid: 'mock-rule-openai-1',
            scenario: 'openai',
            request_model: 'gpt-4o',
            response_model: '',
            active: true,
            description: 'Route gpt-4o to Anthropic claude-opus-4-7',
            services: [{ uuid: 'svc-o1', provider: 'mock-provider-anthropic', model: 'claude-opus-4-7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-openai-2',
            scenario: 'openai',
            request_model: 'gpt-4o-mini',
            response_model: '',
            active: true,
            description: 'Route gpt-4o-mini to Deepseek',
            services: [{ uuid: 'svc-o2', provider: 'mock-provider-deepseek', model: 'deepseek-chat', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-openai-3',
            scenario: 'openai',
            request_model: 'gpt-3.5-turbo',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-o3a', provider: 'mock-provider-glm', model: 'glm-4-flash', weight: 1, active: true },
                { uuid: 'svc-o3b', provider: 'mock-provider-deepseek', model: 'deepseek-chat', weight: 1, active: true },
            ],
        },
    ],
    anthropic: [
        {
            uuid: 'mock-rule-ant-1',
            scenario: 'anthropic',
            request_model: 'claude-opus-4-7',
            response_model: '',
            active: true,
            description: 'Opus 4.7 → GLM',
            services: [{ uuid: 'svc-a1', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-ant-2',
            scenario: 'anthropic',
            request_model: 'claude-sonnet-4-6',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-a2', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-ant-3',
            scenario: 'anthropic',
            request_model: 'claude-haiku-4-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-a3', provider: 'mock-provider-deepseek', model: 'deepseek-chat', weight: 1, active: true }],
        },
    ],
    claude_code: [
        {
            uuid: 'mock-rule-cc-1',
            scenario: 'claude_code',
            request_model: 'claude-opus-4-7',
            response_model: '',
            active: true,
            description: 'Claude Code: Opus 4.7',
            services: [{ uuid: 'svc-cc1', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cc-2',
            scenario: 'claude_code',
            request_model: 'claude-sonnet-4-6',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-cc2a', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true },
                { uuid: 'svc-cc2b', provider: 'mock-provider-deepseek', model: 'deepseek-chat', weight: 1, active: true },
            ],
            smart_enabled: true,
            smart_routing: [
                {
                    uuid: 'smart-cc-1',
                    description: 'Use Deepseek for large context',
                    ops: [{ uuid: 'op-cc-1', position: 'token', operation: 'gt', value: '8000' }],
                    services: [{ uuid: 'svc-sm-cc1', provider: 'mock-provider-deepseek', model: 'deepseek-chat', weight: 1, active: true }],
                },
            ],
        },
        {
            uuid: 'mock-rule-cc-3',
            scenario: 'claude_code',
            request_model: 'claude-haiku-4-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-cc3', provider: 'mock-provider-deepseek', model: 'deepseek-chat', weight: 1, active: true }],
        },
    ],
    claude_desktop: [
        {
            uuid: 'mock-rule-cd-1',
            scenario: 'claude_desktop',
            request_model: 'claude-sonnet-4-6',
            response_model: '',
            active: true,
            description: 'Claude Desktop - Sonnet 4.6 model for balanced performance',
            services: [{ uuid: 'svc-cd1', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cd-2',
            scenario: 'claude_desktop',
            request_model: 'claude-opus-4-6',
            response_model: '',
            active: true,
            description: 'Claude Desktop - Opus 4.6 model for complex tasks',
            services: [{ uuid: 'svc-cd2', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cd-3',
            scenario: 'claude_desktop',
            request_model: 'claude-opus-4-7',
            response_model: '',
            active: true,
            description: 'Claude Desktop - Opus 4.7 model for advanced reasoning',
            services: [{ uuid: 'svc-cd3', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cd-4',
            scenario: 'claude_desktop',
            request_model: 'claude-haiku-4-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-cd4', provider: 'mock-provider-deepseek', model: 'deepseek-chat', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cd-5',
            scenario: 'claude_desktop',
            request_model: 'anthropic/tb-ds-4-3',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-cd5', provider: 'mock-provider-deepseek', model: 'deepseek-r1', weight: 1, active: true }],
        },
    ],
    codex: [
        {
            uuid: 'mock-rule-codex-1',
            scenario: 'codex',
            request_model: 'codex-mini-latest',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-cx1', provider: 'mock-provider-openai', model: 'gpt-4o-mini', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-codex-2',
            scenario: 'codex',
            request_model: 'o4-mini',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-cx2a', provider: 'mock-provider-anthropic', model: 'claude-opus-4-7', weight: 1, active: true },
                { uuid: 'svc-cx2b', provider: 'mock-provider-gemini', model: 'gemini-2.5-pro', weight: 1, active: true },
            ],
        },
    ],
    agent: [
        {
            uuid: 'mock-rule-agent-1',
            scenario: 'agent',
            request_model: 'claude-opus-4-7',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-ag1', provider: 'mock-provider-anthropic', model: 'claude-opus-4-7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-agent-2',
            scenario: 'agent',
            request_model: 'gpt-4o',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-ag2a', provider: 'mock-provider-openai', model: 'gpt-4o', weight: 1, active: true },
                { uuid: 'svc-ag2b', provider: 'mock-provider-gemini', model: 'gemini-2.5-pro', weight: 1, active: true },
            ],
        },
    ],
    vscode: [
        {
            uuid: 'mock-rule-vsc-1',
            scenario: 'vscode',
            request_model: 'claude-sonnet-4-6',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-vs1', provider: 'mock-provider-anthropic', model: 'claude-sonnet-4-6', weight: 1, active: true }],
        },
    ],
    xcode: [
        {
            uuid: 'mock-rule-xc-1',
            scenario: 'xcode',
            request_model: 'claude-sonnet-4-6',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-xc1', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
        },
    ],
    opencode: [
        {
            uuid: 'mock-rule-oc-1',
            scenario: 'opencode',
            request_model: 'claude-opus-4-7',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-oc1', provider: 'mock-provider-anthropic', model: 'claude-opus-4-7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-oc-2',
            scenario: 'opencode',
            request_model: 'gpt-4o',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-oc2', provider: 'mock-provider-openai', model: 'gpt-4o', weight: 1, active: true }],
        },
    ],
}

// ============================================
// Mock Quota Data
// ============================================
const now = new Date()
const inOneHour = new Date(now.getTime() + 60 * 60 * 1000).toISOString()
const inTwoDays = new Date(now.getTime() + 2 * 24 * 60 * 60 * 1000).toISOString()
const inSixDays = new Date(now.getTime() + 6 * 24 * 60 * 60 * 1000).toISOString()
const inThirtyDays = new Date(now.getTime() + 30 * 24 * 60 * 60 * 1000).toISOString()

const mockQuotas: Record<string, any> = {
    'mock-provider-anthropic': {
        provider_uuid: 'mock-provider-anthropic',
        provider_name: 'Anthropic',
        provider_type: 'anthropic',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        primary: {
            type: 'session',
            used: 42350,
            limit: 80000,
            used_percent: 52.9,
            resets_at: inOneHour,
            unit: 'tokens',
            label: 'Session',
            description: 'Current session token usage',
        },
        secondary: {
            type: 'weekly',
            used: 1230000,
            limit: 3000000,
            used_percent: 41.0,
            resets_at: inSixDays,
            unit: 'tokens',
            label: 'Weekly',
            description: 'Weekly token usage',
        },
        tertiary: {
            type: 'monthly',
            used: 4200000,
            limit: 10000000,
            used_percent: 42.0,
            resets_at: inThirtyDays,
            unit: 'tokens',
            label: 'Monthly',
            description: 'Monthly token usage',
        },
        cost: {
            used: 12.50,
            limit: 50.00,
            currency_code: '$',
            resets_at: inThirtyDays,
            label: 'Monthly Cost',
        },
        account: {
            id: 'acc-mock-001',
            name: 'Mock Account',
            email: 'mock@example.com',
            tier: 'pro',
        },
    },
    'mock-provider-openai': {
        provider_uuid: 'mock-provider-openai',
        provider_name: 'OpenAI',
        provider_type: 'openai',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        primary: {
            type: 'daily',
            used: 850,
            limit: 1000,
            used_percent: 85.0,
            resets_at: inTwoDays,
            unit: 'requests',
            label: 'Daily',
            description: 'Daily request limit',
        },
        secondary: {
            type: 'monthly',
            used: 18500,
            limit: 30000,
            used_percent: 61.7,
            resets_at: inThirtyDays,
            unit: 'requests',
            label: 'Monthly',
            description: 'Monthly request limit',
        },
        cost: {
            used: 38.20,
            limit: 100.00,
            currency_code: '$',
            resets_at: inThirtyDays,
            label: 'Monthly Cost',
        },
    },
    'mock-provider-openrouter': {
        provider_uuid: 'mock-provider-openrouter',
        provider_name: 'OpenRouter',
        provider_type: 'openrouter',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        primary: {
            type: 'balance',
            used: 7.30,
            limit: 20.00,
            used_percent: 36.5,
            resets_at: null,
            unit: 'currency',
            label: 'Balance',
            description: 'Remaining credit balance',
        },
    },
}

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

    http.get('/api/v1/status', () => {
        return HttpResponse.json({
            success: true,
            data: {
                server_running: true,
                uptime: 'Mock mode',
            }
        })
    }),

    // ============================================
    // v2 Providers API
    // ============================================
    http.get('/api/v2/providers', () => {
        return HttpResponse.json({
            success: true,
            data: mockV2Providers,
        })
    }),

    http.get('/api/v2/providers/:uuid', ({ params }) => {
        const { uuid } = params
        const provider = mockV2Providers.find(p => p.uuid === uuid)
        if (provider) {
            return HttpResponse.json({ success: true, data: provider })
        }
        return HttpResponse.json({ success: false, error: 'Not found' }, { status: 404 })
    }),

    http.post('/api/v2/providers', async ({ request }) => {
        const body = await request.json() as any
        const newProvider = {
            uuid: `mock-provider-${Date.now()}`,
            ...body,
            token: body.token || '',
            enabled: true,
        }
        mockV2Providers.push(newProvider)
        return HttpResponse.json({ success: true, data: newProvider })
    }),

    // ============================================
    // Provider Quota API (v1)
    // ============================================
    http.post('/api/v1/provider-quota/batch', async ({ request }) => {
        const body = await request.json() as any
        const uuids: string[] = body?.provider_uuids || []
        const result: Record<string, any> = {}
        for (const uuid of uuids) {
            if (mockQuotas[uuid]) {
                result[uuid] = { ...mockQuotas[uuid], fetched_at: new Date().toISOString() }
            }
        }
        return HttpResponse.json({ success: true, data: result })
    }),

    http.get('/api/v1/provider-quota/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        if (mockQuotas[uuid]) {
            return HttpResponse.json({ success: true, data: { ...mockQuotas[uuid], fetched_at: new Date().toISOString() } })
        }
        return HttpResponse.json({ success: false, error: 'No quota data' }, { status: 404 })
    }),

    http.post('/api/v1/provider-quota/:uuid/refresh', async ({ params }) => {
        const { uuid } = params as { uuid: string }
        // Simulate a short delay
        await new Promise(r => setTimeout(r, 800))
        if (mockQuotas[uuid]) {
            const refreshed = { ...mockQuotas[uuid], fetched_at: new Date().toISOString() }
            return HttpResponse.json({ success: true, data: refreshed })
        }
        return HttpResponse.json({ success: false, error: 'No quota data' }, { status: 404 })
    }),

    http.post('/api/v1/provider-quota/refresh', async () => {
        await new Promise(r => setTimeout(r, 1000))
        return HttpResponse.json({ success: true })
    }),

    // --- Playground (imagegen) mocks ---

    http.get('/api/v1/auth/validate', () => {
        return HttpResponse.json({ valid: true, user: { username: 'admin', role: 'admin' } })
    }),

    http.get('/api/v1/token', () => {
        return HttpResponse.json({ token: 'mock-model-token', type: 'Bearer' })
    }),

    http.get('/api/v1/rules', ({ request }) => {
        const url = new URL(request.url)
        const scenario = url.searchParams.get('scenario')
        if (scenario === 'imagegen') {
            return HttpResponse.json({
                success: true,
                data: [
                    {
                        uuid: 'mock-imagegen-1',
                        request_model: 'gpt-image-1',
                        scenario: 'imagegen',
                        disabled: false,
                        services: [{ provider: 'mock', model: 'gpt-image-1' }],
                    },
                    {
                        uuid: 'mock-imagegen-2',
                        request_model: 'dall-e-3',
                        scenario: 'imagegen',
                        disabled: false,
                        services: [{ provider: 'mock', model: 'dall-e-3' }],
                    },
                ],
            })
        }
        const rules = scenario ? (mockV1Rules[scenario] ?? []) : []
        return HttpResponse.json({ success: true, data: rules })
    }),

    http.post('*/tingly/imagegen/v1/images/generations', async ({ request }) => {
        const body = (await request.json()) as any
        const n = Math.max(1, Math.min(10, Number(body?.n) || 1))
        const size = typeof body?.size === 'string' ? body.size : '1024x1024'
        const [w, h] = size.split('x').map(Number)
        const palette = ['#7c3aed', '#06b6d4', '#10b981', '#f59e0b', '#ef4444', '#3b82f6']
        const promptText = String(body?.prompt ?? '').slice(0, 80).replace(/[<>&"]/g, '')

        const makeSvgDataUrl = (idx: number): string => {
            const bg = palette[idx % palette.length]
            const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">`
                + `<rect width="100%" height="100%" fill="${bg}"/>`
                + `<text x="50%" y="45%" font-family="sans-serif" font-size="${Math.round(w / 18)}" fill="white" text-anchor="middle" font-weight="700">MOCK #${idx + 1}</text>`
                + `<text x="50%" y="56%" font-family="sans-serif" font-size="${Math.round(w / 36)}" fill="white" fill-opacity="0.85" text-anchor="middle">${promptText}</text>`
                + `<text x="50%" y="95%" font-family="monospace" font-size="${Math.round(w / 50)}" fill="white" fill-opacity="0.7" text-anchor="middle">${body?.model ?? ''} · ${size} · q=${body?.quality ?? 'auto'}</text>`
                + `</svg>`
            const b64 = btoa(unescape(encodeURIComponent(svg)))
            return `data:image/svg+xml;base64,${b64}`
        }

        // Simulate a small latency so the loading state is visible
        await new Promise((r) => setTimeout(r, 600))

        return HttpResponse.json({
            created: Math.floor(Date.now() / 1000),
            data: Array.from({ length: n }, (_, i) => ({ url: makeSvgDataUrl(i) })),
        })
    }),
]
