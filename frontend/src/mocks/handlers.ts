import { http, HttpResponse } from 'msw'

// ============================================
// Mock Model Requests (correlated per-request traces)
// ============================================
const mockModelRequests = [
    {
        request_id: 'req-anthropic-ok',
        time: '',
        scenario: 'anthropic',
        request_model: 'claude-sonnet-4',
        routed_model: 'claude-sonnet-4-20250514',
        provider: 'Anthropic',
        method: 'POST',
        path: '/anthropic/v1/messages',
        status: 200,
        latency_ms: 1840,
        has_error: false,
        max_level: 'info',
        event_count: 4,
    },
    {
        request_id: 'req-openai-routed',
        time: '',
        scenario: 'openai',
        request_model: 'gpt-4o',
        routed_model: 'claude-sonnet-4-20250514',
        provider: 'Anthropic',
        method: 'POST',
        path: '/openai/v1/chat/completions',
        status: 200,
        latency_ms: 2210,
        has_error: false,
        max_level: 'warning',
        event_count: 5,
    },
    {
        request_id: 'req-openai-fail',
        time: '',
        scenario: 'openai',
        request_model: 'gpt-4o',
        routed_model: 'gpt-4o',
        provider: 'OpenAI',
        method: 'POST',
        path: '/openai/v1/chat/completions',
        status: 502,
        latency_ms: 740,
        has_error: true,
        max_level: 'error',
        event_count: 3,
    },
]

const mockRequestEvents: Record<string, Array<{ source: string; level: string; stage?: string; message: string; fields?: Record<string, any> }>> = {
    'req-anthropic-ok': [
        { source: 'smart_routing', level: 'info', stage: 'routing', message: 'rule matched', fields: { outcome: 'selected', matched_rule_index: 0, selected_provider: 'Anthropic', selected_model: 'claude-sonnet-4-20250514', trace: [{ rule_index: 0, description: 'route sonnet', matched: true, ops: [{ position: 'model', operation: 'equals', matched: true, reason: 'model == claude-sonnet-4' }] }] } },
        { source: 'model_request', level: 'info', stage: 'transform', message: 'anthropic passthrough (no conversion)' },
        { source: 'model_request', level: 'info', stage: 'upstream', message: 'upstream responded', fields: { status: 200, provider: 'Anthropic' } },
        { source: 'http', level: 'info', message: 'POST /anthropic/v1/messages 200', fields: { status: 200, latency: 1840000000, method: 'POST', path: '/anthropic/v1/messages' } },
    ],
    'req-openai-routed': [
        { source: 'smart_routing', level: 'info', stage: 'routing', message: 'rule matched', fields: { outcome: 'selected', matched_rule_index: 1, selected_provider: 'Anthropic', selected_model: 'claude-sonnet-4-20250514', trace: [{ rule_index: 0, description: 'keep gpt', matched: false, ops: [{ position: 'model', operation: 'equals', matched: false, reason: 'model != gpt-3.5' }] }, { rule_index: 1, description: 'upgrade to sonnet', matched: true, ops: [{ position: 'model', operation: 'prefix', matched: true, reason: 'model startswith gpt-4' }] }] } },
        { source: 'model_request', level: 'warning', stage: 'transform', message: 'dropped unsupported field: logprobs' },
        { source: 'model_request', level: 'info', stage: 'transform', message: 'openai chat -> anthropic messages' },
        { source: 'model_request', level: 'info', stage: 'upstream', message: 'upstream responded', fields: { status: 200, provider: 'Anthropic' } },
        { source: 'http', level: 'info', message: 'POST /openai/v1/chat/completions 200', fields: { status: 200, latency: 2210000000, method: 'POST', path: '/openai/v1/chat/completions' } },
    ],
    'req-openai-fail': [
        { source: 'smart_routing', level: 'info', stage: 'routing', message: 'rule matched', fields: { outcome: 'selected', matched_rule_index: 0, selected_provider: 'OpenAI', selected_model: 'gpt-4o' } },
        { source: 'model_request', level: 'error', stage: 'upstream', message: 'upstream call failed: 502 Bad Gateway', fields: { status: 502, provider: 'OpenAI', error: 'bad gateway' } },
        { source: 'http', level: 'error', message: 'POST /openai/v1/chat/completions 502', fields: { status: 502, latency: 740000000, method: 'POST', path: '/openai/v1/chat/completions', error: 'upstream error' } },
    ],
}

// ============================================
// Mock System Logs (genuine app/system logs, not request-scoped)
// ============================================
const mockSystemLogs = [
    { time: '', level: 'info', message: 'tingly-box server started on :8080', fields: { component: 'server' } },
    { time: '', level: 'info', message: 'loaded 4 providers from config', fields: { component: 'config' } },
    { time: '', level: 'info', message: 'StoreManager: all stores initialized', fields: { component: 'db' } },
    { time: '', level: 'warning', message: 'config file changed, reloading rules', fields: { component: 'config' } },
    { time: '', level: 'info', message: 'transport pool cleanup: 2 idle transports removed', fields: { component: 'client' } },
    { time: '', level: 'error', message: 'background quota refresh failed: context deadline exceeded', fields: { component: 'quota' } },
]

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
        api_base_openai: 'https://api.deepseek.com/v1',
        api_base_anthropic: 'https://api.deepseek.com/anthropic',
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
    // ── Virtual-model (vmodel) providers ─────────────────────────────────
    {
        uuid: 'mock-vmodel-tingly',
        name: 'tingly',
        api_base: '',
        api_style: 'anthropic',
        auth_type: 'vmodel',
        token: '',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
        vmodel_detail: {
            models: ['claude-sonnet-4-5', 'claude-opus-4-5', 'gpt-4o', 'gpt-4o-mini', 'deepseek-v4-flash'],
        },
    },
    {
        uuid: 'mock-vmodel-tingly-openai',
        name: 'tingly-openai',
        api_base: '',
        api_style: 'openai',
        auth_type: 'vmodel',
        token: '',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
        vmodel_detail: {
            models: ['tingly', 'tingly-sonnet', 'tingly-opus', 'tingly-fast'],
        },
    },
    {
        uuid: 'mock-vmodel-tingly-anthropic',
        name: 'tingly-anthropic',
        api_base: '',
        api_style: 'anthropic',
        auth_type: 'vmodel',
        token: '',
        enabled: true,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
        vmodel_detail: {
            models: ['claude-3-7-sonnet-latest', 'claude-opus-4-5', 'claude-haiku-4-5'],
        },
    },
    {
        uuid: 'mock-vmodel-tingly-gemini',
        name: 'tingly-gemini',
        api_base: '',
        api_style: 'openai',
        auth_type: 'vmodel',
        token: '',
        enabled: false,
        proxy_url: '',
        api_base_openai: null,
        api_base_anthropic: null,
        vmodel_detail: {
            models: ['gemini-2.5-pro', 'gemini-2.5-flash'],
        },
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
            flags: { cursor_compat: true, thinking_effort: 'high' },
            services: [{ uuid: 'svc-o1', provider: 'mock-provider-anthropic', model: 'claude-opus-4-7', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-openai-2',
            scenario: 'openai',
            request_model: 'gpt-4o-mini[1m]',
            response_model: '',
            active: true,
            description: 'Route gpt-4o-mini to Deepseek',
            flags: { context_1m: true },
            services: [{ uuid: 'svc-o2', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true }],
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
                { uuid: 'svc-o3b', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true },
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
            request_model: 'claude-haiku-4-5[1m]',
            response_model: '',
            active: true,
            description: '',
            flags: { context_1m: true },
            services: [{ uuid: 'svc-a3', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true }],
        },
    ],
    claude_code: [
        {
            // Unified-mode rule fetched by GET /api/v1/rule/builtin:claude_code:cc.
            // Demonstrates smart routing with fabric conditions + tiered default fallback.
            uuid: 'builtin:claude_code:cc',
            scenario: 'claude_code',
            request_model: 'claude-sonnet-4-6',
            response_model: '',
            active: true,
            description: 'Smart routing + tiered fallback',
            flags: { claude_code_compat: true, clean_header: true, skip_usage: true, session_affinity: 3600 },
            // Default providers in 3 tiers: T0 primary, T1 secondary, T2 budget
            services: [
                { uuid: 'svc-cc-t0-a', provider: 'mock-provider-anthropic', model: 'claude-sonnet-4-6', weight: 1, active: true, tier: 0 },
                { uuid: 'svc-cc-t0-b', provider: 'mock-provider-openai', model: 'gpt-4o', weight: 1, active: true, tier: 0 },
                { uuid: 'svc-cc-t1-a', provider: 'mock-provider-gemini', model: 'gemini-2.0-flash', weight: 1, active: true, tier: 1 },
                { uuid: 'svc-cc-t2-a', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true, tier: 2 },
                { uuid: 'svc-cc-t2-b', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true, tier: 2 },
            ],
            smart_enabled: true,
            smart_routing: [
                {
                    uuid: 'smart-cc-bi-sub',
                    description: 'Subagent → Deepseek R1',
                    ops: [{ uuid: 'op-cc-bi-sub', position: 'agent.claude_code', operation: 'equals', value: 'subagent' }],
                    services: [{ uuid: 'svc-sm-cc-bi-sub', provider: 'mock-provider-deepseek', model: 'deepseek-v4-pro', weight: 1, active: true }],
                },
                {
                    uuid: 'smart-cc-bi-cmp',
                    description: 'Compact → GLM cheap summarisation',
                    ops: [{ uuid: 'op-cc-bi-cmp', position: 'agent.claude_code', operation: 'equals', value: 'compact' }],
                    services: [{ uuid: 'svc-sm-cc-bi-cmp', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
                },
                {
                    uuid: 'smart-cc-bi-tok',
                    description: 'Large context ≥ 60k tokens → Gemini',
                    ops: [{ uuid: 'op-cc-bi-tok', position: 'token', operation: 'ge', value: '60000' }],
                    services: [{ uuid: 'svc-sm-cc-bi-tok', provider: 'mock-provider-gemini', model: 'gemini-2.0-flash', weight: 1, active: true }],
                },
                {
                    uuid: 'smart-cc-bi-default',
                    description: 'Default (unconditional fallback)',
                    ops: [],
                    services: [
                        { uuid: 'svc-sm-cc-bi-def-a', provider: 'mock-provider-anthropic', model: 'claude-sonnet-4-6', weight: 1, active: true },
                        { uuid: 'svc-sm-cc-bi-def-b', provider: 'mock-provider-openai', model: 'gpt-4o', weight: 1, active: true },
                    ],
                },
            ],
        },
        {
            uuid: 'mock-rule-cc-smart',
            scenario: 'claude_code',
            request_model: 'claude-sonnet-4-6',
            response_model: '',
            active: true,
            description: 'Smart routing by agent kind',
            services: [
                { uuid: 'svc-cc-default-a', provider: 'mock-provider-anthropic', model: 'claude-sonnet-4-6', weight: 1, active: true },
            ],
            smart_enabled: true,
            smart_routing: [
                {
                    uuid: 'smart-cc-subagent',
                    description: 'Subagent → Deepseek',
                    ops: [{ uuid: 'op-cc-sub', position: 'agent.claude_code', operation: 'equals', value: 'subagent' }],
                    services: [{ uuid: 'svc-sm-cc-sub', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true }],
                },
                {
                    uuid: 'smart-cc-compact',
                    description: 'Compact → GLM (cheap summarisation)',
                    ops: [{ uuid: 'op-cc-cmp', position: 'agent.claude_code', operation: 'equals', value: 'compact' }],
                    services: [{ uuid: 'svc-sm-cc-cmp', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
                },
                {
                    uuid: 'smart-cc-large-ctx',
                    description: 'Large context → Deepseek R1',
                    ops: [{ uuid: 'op-cc-tok', position: 'token', operation: 'ge', value: '60000' }],
                    services: [{ uuid: 'svc-sm-cc-tok', provider: 'mock-provider-deepseek', model: 'deepseek-v4-pro', weight: 1, active: true }],
                },
            ],
        },
        {
            uuid: 'mock-rule-cc-direct',
            scenario: 'claude_code',
            request_model: 'claude-opus-4-7',
            response_model: '',
            active: true,
            description: 'Direct load-balance across Anthropic + GLM',
            services: [
                { uuid: 'svc-cc-dir-a', provider: 'mock-provider-anthropic', model: 'claude-opus-4-7', weight: 1, active: true },
                { uuid: 'svc-cc-dir-b', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true },
            ],
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
            flags: { context_1m: true },
            services: [{ uuid: 'svc-cd4', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true }],
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
            services: [{ uuid: 'svc-cd1', provider: 'mock-provider-glm', model: 'glm-4.7', weight: 1, active: true }],
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

// ============================================
// Mock Claude Code Profiles
// ============================================
const mockClaudeCodeProfiles = [
    {
        id: 'p1',
        name: 'ds',
        description: 'DeepSeek profile for cost-effective development',
        unified: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
    },
    {
        id: 'p2',
        name: 'sonnet',
        description: 'Claude Sonnet profile for balanced performance',
        unified: false,
        created_at: '2024-01-15T00:00:00Z',
        updated_at: '2024-01-15T00:00:00Z',
    },
    {
        id: 'p3',
        name: 'codex',
        description: 'Codex profile for code generation tasks',
        unified: true,
        created_at: '2024-02-01T00:00:00Z',
        updated_at: '2024-02-01T00:00:00Z',
    },
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

    // ── v2 provider models by UUID (used by ModelSelectDialog) ───────────────
    http.get('/api/v2/provider-models/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        const modelMap: Record<string, string[]> = {
            'mock-provider-anthropic': [
                'claude-opus-4-7',
                'claude-opus-4-6',
                'claude-sonnet-4-6',
                'claude-sonnet-4-5',
                'claude-haiku-4-5',
                'claude-opus-4-5',
                'claude-3-7-sonnet-latest',
                'claude-3-5-haiku-latest',
                'claude-3-5-sonnet-latest',
                'claude-fable-5',
            ],
            'mock-provider-openai': [
                'gpt-4o',
                'gpt-4o-mini',
                'gpt-4-turbo',
                'gpt-4',
                'gpt-3.5-turbo',
                'o1',
                'o1-mini',
                'o3',
                'o3-mini',
                'gpt-5-codex',
                'gpt-5.1-codex',
                'gpt-5.1-codex-max',
                'gpt-5.1-codex-mini',
                'gpt-5.2-codex',
                'gpt-5.3-codex',
                'gpt-5.4',
                'gpt-5.4-mini',
                'gpt-5.5',
            ],
            'mock-provider-codex': [
                'gpt-5-codex',
                'gpt-5.1-codex',
                'gpt-5.1-codex-max',
                'gpt-5.1-codex-mini',
                'gpt-5.2-codex',
                'gpt-5.3-codex',
                'gpt-5.4',
                'gpt-5.4-mini',
                'gpt-5.5',
            ],
            'mock-provider-openrouter': [
                'deepseek/deepseek-v4-flash',
                'deepseek/deepseek-v4-pro',
                'google/gemini-2.5-pro',
                'google/gemini-2.5-flash',
                'meta-llama/llama-4-maverick',
                'qwen/qwen3-235b-a22b',
            ],
            'mock-provider-deepseek': [
                'deepseek-v4-flash',
                'deepseek-v4-pro',
            ],
            'mock-provider-gemini': [
                'gemini-2.5-pro',
                'gemini-2.5-flash',
                'gemini-2.5-flash-lite',
                'gemini-2.0-flash',
                'gemini-1.5-pro',
                'gemini-1.5-flash',
            ],
            'mock-provider-glm': [
                'glm-4.7',
                'glm-4.6',
                'glm-4.5',
                'glm-4.5-air',
            ],
        }
        const models = modelMap[uuid] ?? []
        return HttpResponse.json({ success: true, data: { models } })
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

    // Health + version — polled by HealthContext / the sidebar; without these
    // the mock UI keeps popping the "Connection Lost" dialog.
    http.get('/api/v1/info/health', () => HttpResponse.json({ health: true })),

    http.get('/api/v1/info/version', () => HttpResponse.json({
        success: true,
        data: { version: 'mock-dev' },
    })),

    // Probe V2 (E2E) — used by the quick test button and the Probe dialog.
    // Alternates success/failure so both pill states are previewable.
    http.post('/api/v2/probe', async ({ request }) => {
        const body = await request.json() as any
        probeRequestCount++
        const isSuccess = probeRequestCount % 3 !== 0
        await new Promise((r) => setTimeout(r, 800))

        if (!isSuccess) {
            return HttpResponse.json({
                success: false,
                error: {
                    message: 'Simulated upstream error: connection timeout',
                    type: 'probe_error',
                },
            })
        }

        return HttpResponse.json({
            success: true,
            data: {
                content: JSON.stringify([{ choices: [{ delta: { content: 'Hello! Mock streaming probe response.' } }] }]),
                latency_ms: Math.floor(Math.random() * 2200) + 400,
                request_url: 'http://localhost:12222/tingly/openai/chat/completions',
                stream: body?.test_mode !== 'simple',
                prompt_tokens: 21,
                completion_tokens: 14,
                total_tokens: 35,
                selected_provider: 'Mock Provider',
                selected_model: 'mock-model-mini',
                routing_source: 'load_balancer',
                upstream_api: 'openai_chat',
                upstream_url: 'https://api.mock-provider.dev/v1/chat/completions',
                matched_rule_desc: 'Mock Rule',
                applied_flags: '',
            },
        })
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

    http.get('/api/v1/auth/token', () => {
        return HttpResponse.json({
            success: true,
            data: { token: 'mock-user-token-a1b2c3d4e5f6', is_default: false },
        })
    }),

    http.post('/api/v1/auth/token/reset', () => {
        return HttpResponse.json({
            success: true,
            data: { token: `mock-user-token-${Math.random().toString(36).slice(2, 14)}` },
        })
    }),

    http.get('/api/v1/token', () => {
        return HttpResponse.json({ token: 'mock-model-token', type: 'Bearer' })
    }),

    http.post('/api/v1/auth/model-token/reset', () => {
        return HttpResponse.json({
            success: true,
            data: { token: `mock-model-token-${Math.random().toString(36).slice(2, 14)}` },
        })
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

        // Handle profile-specific rules (e.g., claude_code:p1)
        if (scenario?.startsWith('claude_code:')) {
            const profileId = scenario.split(':')[1];
            // Return mock rules for the specific profile
            return HttpResponse.json({
                success: true,
                data: [
                    {
                        uuid: `mock-rule-cc-${profileId}-1`,
                        scenario: `claude_code:${profileId}`,
                        request_model: 'claude-sonnet-4-6',
                        response_model: '',
                        active: true,
                        description: `Profile ${profileId} - Smart routing rule`,
                        flags: { claude_code_compat: true, clean_header: true },
                        services: [
                            { uuid: `svc-cc-${profileId}-1`, provider: 'mock-provider-anthropic', model: 'claude-sonnet-4-6', weight: 1, active: true },
                        ],
                    },
                ],
            })
        }

        const rules = scenario ? (mockV1Rules[scenario] ?? []) : []
        return HttpResponse.json({ success: true, data: rules })
    }),

    // Scenario config (per-scenario UI prefs incl. unified vs. separate mode)
    http.get('/api/v1/scenario/:scenario', ({ params }) => {
        const { scenario } = params as { scenario: string }
        return HttpResponse.json({
            success: true,
            data: { scenario, config_mode: 'unified' },
        })
    }),

    http.post('/api/v1/scenario/:scenario', async ({ params, request }) => {
        const { scenario } = params as { scenario: string }
        const body = await request.json() as any
        return HttpResponse.json({ success: true, data: { scenario, ...body } })
    }),

    http.get('/api/v1/rule/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        for (const rules of Object.values(mockV1Rules)) {
            const rule = rules.find((r) => r.uuid === uuid)
            if (rule) return HttpResponse.json({ success: true, data: rule })
        }
        return HttpResponse.json({ success: false, error: 'Rule not found' }, { status: 404 })
    }),

    http.post('/api/v1/rule/:uuid', async ({ params, request }) => {
        const { uuid } = params as { uuid: string }
        const body = await request.json() as any
        for (const rules of Object.values(mockV1Rules)) {
            const idx = rules.findIndex((r) => r.uuid === uuid)
            if (idx >= 0) {
                rules[idx] = { ...rules[idx], ...body }
                return HttpResponse.json({ success: true, data: rules[idx] })
            }
        }
        return HttpResponse.json({ success: false, error: 'Rule not found' }, { status: 404 })
    }),

    http.delete('/api/v1/rule/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        for (const rules of Object.values(mockV1Rules)) {
            const idx = rules.findIndex((r) => r.uuid === uuid)
            if (idx >= 0) {
                rules.splice(idx, 1)
                return HttpResponse.json({ success: true })
            }
        }
        return HttpResponse.json({ success: false, error: 'Rule not found' }, { status: 404 })
    }),

    // ============================================
    // Rule Flag Registry
    // ============================================
    http.get('/api/v1/rule/flags/registry', () => {
        return HttpResponse.json({
            success: true,
            data: [
                { key: 'custom_user_agent', label: 'Custom User-Agent', description: 'Override the outbound User-Agent header.', type: 'string', category: 'request_openai', placeholder: 'e.g. MyApp/1.0', suggestions: [{ value: 'claude-cli/2.1.86 (external, cli)', label: 'Claude Code (CLI)' }], shared: true, inheritance_mode: 'override' },
                { key: 'openai_endpoint_override', label: 'OpenAI endpoint override', description: 'Force Chat or Responses endpoint.', type: 'enum', category: 'request_openai', options: [{ value: 'auto', label: 'Auto' }, { value: 'chat', label: 'Force Chat' }, { value: 'responses', label: 'Force Responses' }] },
                { key: 'use_max_completion_tokens', label: 'OpenAI: Use max_completion_tokens', description: 'Rewrite max_tokens to max_completion_tokens.', type: 'bool', category: 'request_openai' },
                { key: 'use_max_tokens', label: 'OpenAI: Use max_tokens (legacy)', description: 'Rewrite max_completion_tokens to max_tokens.', type: 'bool', category: 'request_openai' },
                { key: 'block_tools', label: 'Block tools', description: 'Comma-separated tool names to strip.', type: 'string', category: 'request_openai', placeholder: 'e.g. web_search,run_terminal_cmd' },
                { key: 'skip_usage', label: 'Skip usage in response', description: 'Strip usage from responses.', type: 'bool', category: 'response', shared: true, inheritance_mode: 'or' },
                { key: 'thinking_effort', label: 'Thinking', description: 'Extended thinking control.', type: 'enum', category: 'reasoning', options: [{ value: '', label: 'By Client' }, { value: 'off', label: 'Off' }, { value: 'low', label: 'Low' }, { value: 'medium', label: 'Medium' }, { value: 'high', label: 'High' }, { value: 'max', label: 'Max' }], shared: true, inheritance_mode: 'override' },
                { key: 'vision_proxy_service', label: 'Vision Proxy', description: 'Describe images via a vision-capable model.', type: 'service_ref', category: 'vision' },
                { key: 'session_affinity', label: 'Session affinity', description: 'TTL in seconds for session pinning.', type: 'int', category: 'routing', placeholder: 'e.g. 3600', shared: true, inheritance_mode: 'override' },
                { key: 'cursor_compat', label: 'Cursor compatibility', description: 'Normalize rich content for Cursor clients.', type: 'bool', category: 'app' },
                { key: 'cursor_compat_auto', label: 'Auto-detect Cursor', description: 'Auto-apply cursor compat from request headers.', type: 'bool', category: 'app' },
                { key: 'claude_code_compat', label: 'Claude Code compatibility', description: 'Rewrite system role to user.', type: 'bool', category: 'app', shared: true, inheritance_mode: 'or' },
                { key: 'clean_header', label: 'Clean Header', description: 'Strip billing header from system messages.', type: 'bool', category: 'app', shared: true, inheritance_mode: 'or' },
            ],
        })
    }),

    // ============================================
    // Guardrails API (v1)
    // ============================================
    http.get('/api/v1/guardrails/config', () => {
        return HttpResponse.json({
            success: true,
            config: {
                policies: [
                    {
                        id: 'policy-001',
                        name: 'Block Sensitive Data',
                        enabled: true,
                        actions: ['block'],
                        patterns: ['credit_card', 'ssn', 'password'],
                        description: 'Prevents sensitive data from being transmitted',
                    },
                    {
                        id: 'policy-002',
                        name: 'Rate Limit Guard',
                        enabled: true,
                        actions: ['throttle'],
                        patterns: [],
                        description: 'Limits request rate per user session',
                    },
                    {
                        id: 'policy-003',
                        name: 'Prompt Injection Shield',
                        enabled: true,
                        actions: ['block', 'log'],
                        patterns: ['ignore previous', 'system prompt'],
                        description: 'Detects and blocks prompt injection attempts',
                    },
                    {
                        id: 'policy-004',
                        name: 'Content Safety',
                        enabled: false,
                        actions: ['warn'],
                        patterns: [],
                        description: 'Flags potentially unsafe content for review',
                    },
                ],
            },
            imports: [
                { path: '/etc/tingly/guardrails/default.yml', name: 'Default Policy', policy_count: 3 },
                { path: '/etc/tingly/guardrails/enterprise.yml', name: 'Enterprise Rules', policy_count: 8 },
            ],
        })
    }),

    http.get('/api/v1/guardrails/history', () => {
        const now = Date.now()
        return HttpResponse.json({
            success: true,
            data: [
                { time: new Date(now - 2 * 60 * 1000).toISOString(), verdict: 'blocked', phase: 'request', scenario: 'claude_code', alias_hits: ['credit_card'], credential_names: [] },
                { time: new Date(now - 8 * 60 * 1000).toISOString(), verdict: 'allowed', phase: 'response', scenario: 'openai', alias_hits: [], credential_names: [] },
                { time: new Date(now - 15 * 60 * 1000).toISOString(), verdict: 'blocked', phase: 'request', scenario: 'openai', alias_hits: ['prompt_injection'], credential_names: ['OpenAI'] },
                { time: new Date(now - 32 * 60 * 1000).toISOString(), verdict: 'allowed', phase: 'request', scenario: 'claude_code', alias_hits: [], credential_names: [] },
                { time: new Date(now - 45 * 60 * 1000).toISOString(), verdict: 'blocked', phase: 'response', scenario: 'anthropic', alias_hits: ['ssn'], credential_names: [] },
                { time: new Date(now - 60 * 60 * 1000).toISOString(), verdict: 'allowed', phase: 'request', scenario: 'agent', alias_hits: [], credential_names: [] },
            ],
        })
    }),

    http.get('/api/v1/guardrails/builtins', () => {
        return HttpResponse.json({ success: true, data: [] })
    }),

    http.get('/api/v1/guardrails/credentials', () => {
        return HttpResponse.json({ success: true, data: [] })
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

    // ============================================
    // Usage Stats API (v1)
    // ============================================
    http.get('/api/v1/usage/stats', () => {
        return HttpResponse.json({
            success: true,
            data: [
                {
                    key: 'anthropic/claude-sonnet-4-5',
                    provider_uuid: 'mock-provider-anthropic',
                    provider_name: 'Anthropic',
                    model: 'claude-sonnet-4-5',
                    scenario: 'claude_code',
                    request_count: 1842,
                    total_tokens: 9820400,
                    total_input_tokens: 5210000,
                    total_output_tokens: 3850000,
                    cache_input_tokens: 760400,
                    avg_latency_ms: 1240,
                    error_count: 12,
                    error_rate: 0.65,
                    streamed_count: 1800,
                },
                {
                    key: 'anthropic/claude-opus-4-5',
                    provider_uuid: 'mock-provider-anthropic',
                    provider_name: 'Anthropic',
                    model: 'claude-opus-4-5',
                    scenario: 'claude_code',
                    request_count: 420,
                    total_tokens: 3240000,
                    total_input_tokens: 1980000,
                    total_output_tokens: 1100000,
                    cache_input_tokens: 160000,
                    avg_latency_ms: 2100,
                    error_count: 3,
                    error_rate: 0.71,
                    streamed_count: 415,
                },
                {
                    key: 'openai/gpt-4o',
                    provider_uuid: 'mock-provider-openai',
                    provider_name: 'OpenAI',
                    model: 'gpt-4o',
                    scenario: 'openai',
                    request_count: 938,
                    total_tokens: 4120000,
                    total_input_tokens: 2600000,
                    total_output_tokens: 1520000,
                    cache_input_tokens: 0,
                    avg_latency_ms: 980,
                    error_count: 8,
                    error_rate: 0.85,
                    streamed_count: 920,
                },
                {
                    key: 'openai/gpt-4o-mini',
                    provider_uuid: 'mock-provider-openai',
                    provider_name: 'OpenAI',
                    model: 'gpt-4o-mini',
                    scenario: 'openai',
                    request_count: 2150,
                    total_tokens: 3280000,
                    total_input_tokens: 1840000,
                    total_output_tokens: 1440000,
                    cache_input_tokens: 0,
                    avg_latency_ms: 420,
                    error_count: 5,
                    error_rate: 0.23,
                    streamed_count: 2100,
                },
                {
                    key: 'openrouter/deepseek-v4-pro',
                    provider_uuid: 'mock-provider-openrouter',
                    provider_name: 'OpenRouter',
                    model: 'deepseek/deepseek-v4-pro',
                    scenario: 'agent',
                    request_count: 312,
                    total_tokens: 2180000,
                    total_input_tokens: 1200000,
                    total_output_tokens: 980000,
                    cache_input_tokens: 0,
                    avg_latency_ms: 3200,
                    error_count: 2,
                    error_rate: 0.64,
                    streamed_count: 308,
                },
            ],
        })
    }),

    http.get('/api/v1/usage/timeseries', ({ request }) => {
        const url = new URL(request.url)
        const interval = url.searchParams.get('interval') || 'day'
        const startTimeStr = url.searchParams.get('start_time')
        const endTimeStr = url.searchParams.get('end_time')
        const now = new Date()

        // Determine count from start/end when present
        const inferCount = (ms: number): number => {
            if (startTimeStr && endTimeStr) {
                const diff = new Date(endTimeStr).getTime() - new Date(startTimeStr).getTime()
                return Math.max(1, Math.round(diff / ms))
            }
            return undefined as unknown as number
        }

        const generatePoints = (count: number, intervalMs: number, baseTokens = 800000, workdayBias = false) => {
            const origin = startTimeStr ? new Date(startTimeStr) : new Date(now.getTime() - (count - 1) * intervalMs)
            return Array.from({ length: count }, (_, i) => {
                const ts = new Date(origin.getTime() + i * intervalMs)
                // Weekend / off-hours reduction for realistic shape
                const dow = ts.getDay()
                const hour = ts.getHours()
                const isWeekend = dow === 0 || dow === 6
                const isOffHours = hour < 7 || hour > 22
                const activityFactor = workdayBias
                    ? (isWeekend ? 0.1 + Math.random() * 0.15 : isOffHours ? 0.2 + Math.random() * 0.2 : 1)
                    : 1
                const trend = 1 + (i / count) * 0.4  // gradual growth over time
                const wave = Math.sin(i * 0.4) * 0.35
                const base = baseTokens * (0.65 + wave + Math.random() * 0.3) * trend * activityFactor
                const input = Math.round(base * 0.55)
                const output = Math.round(base * 0.38)
                const cache = Math.round(base * 0.07)
                return {
                    timestamp: ts.toISOString(),
                    request_count: Math.round((180 + Math.sin(i * 0.4) * 70 + Math.random() * 50) * activityFactor * trend),
                    input_tokens: input,
                    output_tokens: output,
                    cache_input_tokens: cache,
                    total_tokens: input + output + cache,
                    error_count: Math.round(Math.random() * 3),
                    avg_latency_ms: Math.round(900 + Math.random() * 600),
                }
            })
        }

        let data
        if (interval === 'minute') {
            // today / yesterday: one point per 15 minutes → 96 points
            const count = inferCount(15 * 60 * 1000) || 96
            data = generatePoints(Math.min(count, 96), 15 * 60 * 1000, 12000, true)
        } else if (interval === 'hour') {
            const count = inferCount(60 * 60 * 1000) || 24
            data = generatePoints(Math.min(count, 48), 60 * 60 * 1000, 80000, true)
        } else {
            // day — may be 7d or 180d/365d for the heatmap
            const count = inferCount(24 * 60 * 60 * 1000) || 180
            data = generatePoints(Math.min(count, 366), 24 * 60 * 60 * 1000, 900000, true)
        }

        return HttpResponse.json({ success: true, data })
    }),

    http.get('/api/v1/usage/records', ({ request }) => {
        const url = new URL(request.url)
        const limit = parseInt(url.searchParams.get('limit') || '500')
        const offset = parseInt(url.searchParams.get('offset') || '0')
        const statusFilter = url.searchParams.get('status') || ''

        const models = [
            { provider_name: 'Anthropic', model: 'claude-sonnet-4-5', scenario: 'claude_code', streamed: true },
            { provider_name: 'Anthropic', model: 'claude-opus-4-5',   scenario: 'claude_code', streamed: true },
            { provider_name: 'OpenAI',    model: 'gpt-4o',            scenario: 'openai',      streamed: true },
            { provider_name: 'OpenAI',    model: 'gpt-4o-mini',       scenario: 'openai',      streamed: false },
            { provider_name: 'OpenRouter', model: 'deepseek/deepseek-v4-pro', scenario: 'agent',   streamed: true },
        ]

        const now = Date.now()
        const dayStart = new Date()
        dayStart.setHours(0, 0, 0, 0)

        const total = 120
        const all = Array.from({ length: total }, (_, i) => {
            const m = models[i % models.length]
            const isError = i % 20 === 0
            const input = Math.round(800 + Math.random() * 4000)
            const output = Math.round(200 + Math.random() * 1500)
            const cache = m.provider_name === 'Anthropic' ? Math.round(Math.random() * 800) : 0
            const latency = Math.round(
                m.model.includes('opus') ? 1500 + Math.random() * 3000
                : m.model.includes('mini') ? 200 + Math.random() * 600
                : 600 + Math.random() * 1800
            )
            const ts = new Date(dayStart.getTime() + Math.random() * (now - dayStart.getTime()))
            return {
                id: i + 1,
                provider_uuid: `mock-provider-${m.provider_name.toLowerCase()}`,
                provider_name: m.provider_name,
                model: m.model,
                scenario: m.scenario,
                timestamp: ts.toISOString(),
                input_tokens: input,
                output_tokens: output,
                total_tokens: input + output + cache,
                cache_input_tokens: cache,
                status: isError ? 'error' : 'success',
                error_code: isError ? 'rate_limit_exceeded' : '',
                latency_ms: latency,
                ttft_ms: (m.streamed && !isError) ? Math.round(latency * (0.05 + Math.random() * 0.15)) : 0,
                streamed: m.streamed && !isError,
            }
        }).sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())

        const filtered = statusFilter ? all.filter(r => r.status === statusFilter) : all
        const page = filtered.slice(offset, offset + limit)

        return HttpResponse.json({
            success: true,
            meta: { total: filtered.length, limit, offset },
            data: page,
        })
    }),

    // ============================================
    // ImBot Settings API (v1)
    // ============================================
    http.get('/api/v1/imbot-platforms', () => {
        return HttpResponse.json({
            success: true,
            platforms: [
                { name: 'telegram', label: 'Telegram', auth_type: 'token', category: 'im' },
                { name: 'slack', label: 'Slack', auth_type: 'oauth', category: 'im' },
                { name: 'discord', label: 'Discord', auth_type: 'token', category: 'im' },
                { name: 'feishu', label: 'Feishu', auth_type: 'qr', category: 'enterprise' },
                { name: 'wecom', label: 'WeCom', auth_type: 'token', category: 'enterprise' },
            ],
        })
    }),

    http.get('/api/v1/imbot-settings', () => {
        return HttpResponse.json({
            success: true,
            settings: [
                {
                    uuid: 'mock-bot-001',
                    name: 'My Claude Code Bot',
                    platform: 'telegram',
                    enabled: true,
                    auth_type: 'token',
                    default_cwd: '/home/user/projects',
                    default_agent: 'claude_code',
                    smartguide_provider: 'mock-provider-anthropic',
                    smartguide_model: 'claude-sonnet-4-5',
                    chat_id: '123456789',
                    created_at: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
                    auth: { token: 'mock-bot-token-****' },
                },
                {
                    uuid: 'mock-bot-002',
                    name: 'Team Slack Bot',
                    platform: 'slack',
                    enabled: true,
                    auth_type: 'oauth',
                    default_cwd: '/home/user/workspace',
                    default_agent: 'claude_code',
                    smartguide_provider: 'mock-provider-openai',
                    smartguide_model: 'gpt-4o',
                    chat_id: 'C0123456789',
                    created_at: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
                    auth: {},
                },
                {
                    uuid: 'mock-bot-003',
                    name: 'Dev Discord Bot',
                    platform: 'discord',
                    enabled: false,
                    auth_type: 'token',
                    default_cwd: '/home/user/dev',
                    default_agent: 'claude_code',
                    smartguide_provider: 'mock-provider-anthropic',
                    smartguide_model: 'claude-opus-4-5',
                    chat_id: '',
                    created_at: new Date(Date.now() - 14 * 24 * 60 * 60 * 1000).toISOString(),
                    auth: { token: 'mock-discord-token-****' },
                },
            ],
        })
    }),

    http.get('/api/v1/imbot-settings/:uuid', ({ params }) => {
        return HttpResponse.json({
            success: true,
            setting: {
                uuid: params.uuid,
                name: 'Mock Bot',
                platform: 'telegram',
                enabled: true,
                auth_type: 'token',
                default_cwd: '/home/user/projects',
                default_agent: 'claude_code',
                smartguide_provider: 'mock-provider-anthropic',
                smartguide_model: 'claude-sonnet-4-5',
                created_at: new Date().toISOString(),
                auth: {},
            },
        })
    }),

    http.post('/api/v1/imbot-settings/:uuid/toggle', ({ params }) => {
        return HttpResponse.json({ success: true, uuid: params.uuid })
    }),

    http.get('/api/v1/imbot-settings/:uuid/pairing-code', () => {
        return HttpResponse.json({
            success: true,
            active: true,
            code: 'TNGLY-1001-XK72',
            expires_at: new Date(Date.now() + 9 * 60 * 1000).toISOString(),
        })
    }),

    http.post('/api/v1/imbot-settings/:uuid/pairing-code/rotate', () => {
        return HttpResponse.json({
            success: true,
            active: true,
            code: 'TNGLY-' + Math.random().toString(36).slice(2, 6).toUpperCase() + '-' + Math.floor(Math.random() * 9000 + 1000),
            expires_at: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
        })
    }),

    http.put('/api/v1/imbot-settings/:uuid', async ({ params, request }) => {
        const body = await request.json() as any
        return HttpResponse.json({ success: true, uuid: params.uuid, ...body })
    }),

    http.get('/api/v1/system/logs', ({ request }) => {
        const url = new URL(request.url)
        const limit = Number(url.searchParams.get('limit')) || 100
        const now = Date.now()
        const logs = mockSystemLogs.slice(0, limit).map((l, i) => ({
            ...l,
            time: new Date(now - i * 1500).toISOString(),
        }))
        return HttpResponse.json({ total: logs.length, logs })
    }),

    // ============================================
    // Scenario Descriptors API
    // ============================================
    http.get('/api/v1/scenario-descriptors', () => {
        return HttpResponse.json({
            success: true,
            data: [
                { id: 'claude_code', supports_profiles: true },
                { id: 'claude_desktop', supports_profiles: false },
                { id: 'openai', supports_profiles: false },
                { id: 'anthropic', supports_profiles: false },
                { id: 'codex', supports_profiles: false },
            ],
        })
    }),

    http.get('/api/v1/requests', ({ request }) => {
        const url = new URL(request.url)
        const scenario = url.searchParams.get('scenario')
        const provider = url.searchParams.get('provider')
        const status = url.searchParams.get('status')
        const now = Date.now()
        const filtered = mockModelRequests.filter((r) =>
            (!scenario || r.scenario === scenario) &&
            (!provider || r.provider === provider) &&
            (!status || String(r.status) === status),
        )
        return HttpResponse.json({
            total: filtered.length,
            requests: filtered.map((r, i) => ({
                ...r,
                time: new Date(now - (filtered.length - i) * 4000).toISOString(),
            })),
        })
    }),

    http.get('/api/v1/requests/:id', ({ params }) => {
        const id = String(params.id)
        const summary = mockModelRequests.find((r) => r.request_id === id)
        if (!summary) {
            return HttpResponse.json({ error: 'request not found' }, { status: 404 })
        }
        const base = Date.now() - 4000
        return HttpResponse.json({
            ...summary,
            time: new Date(base).toISOString(),
            events: (mockRequestEvents[id] || []).map((e, i) => ({
                ...e,
                time: new Date(base + i * 120).toISOString(),
            })),
        })
    }),

    // ============================================
    // Claude Code Profiles API (v1)
    // ============================================
    http.get('/api/v1/scenario/:scenario/profiles', ({ params }) => {
        const { scenario } = params as { scenario: string }
        if (scenario === 'claude_code') {
            return HttpResponse.json({
                success: true,
                data: mockClaudeCodeProfiles,
            })
        }
        return HttpResponse.json({
            success: true,
            data: [],
        })
    }),

    http.post('/api/v1/scenario/:scenario/profiles', async ({ params, request }) => {
        const { scenario } = params as { scenario: string }
        const body = await request.json() as any
        const newProfile = {
            id: `profile-${Date.now()}`,
            name: body.name,
            description: body.description || '',
            unified: body.unified ?? true,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
        }
        mockClaudeCodeProfiles.push(newProfile)
        return HttpResponse.json({
            success: true,
            data: newProfile,
        })
    }),

    http.put('/api/v1/scenario/:scenario/profiles/:profileId', async ({ params, request }) => {
        const { scenario, profileId } = params as { scenario: string; profileId: string }
        const body = await request.json() as any

        if (scenario !== 'claude_code') {
            return HttpResponse.json({
                success: false,
                error: 'Scenario not supported',
            }, { status: 400 })
        }

        const profile = mockClaudeCodeProfiles.find(p => p.id === profileId)
        if (!profile) {
            return HttpResponse.json({
                success: false,
                error: 'Profile not found',
            }, { status: 404 })
        }

        if (body.name !== undefined) profile.name = body.name
        if (body.description !== undefined) profile.description = body.description
        if (body.unified !== undefined) profile.unified = body.unified
        profile.updated_at = new Date().toISOString()

        return HttpResponse.json({
            success: true,
            data: profile,
        })
    }),

    http.delete('/api/v1/scenario/:scenario/profiles/:profileId', ({ params }) => {
        const { scenario, profileId } = params as { scenario: string; profileId: string }

        if (scenario !== 'claude_code') {
            return HttpResponse.json({
                success: false,
                error: 'Scenario not supported',
            }, { status: 400 })
        }

        const index = mockClaudeCodeProfiles.findIndex(p => p.id === profileId)
        if (index >= 0) {
            mockClaudeCodeProfiles.splice(index, 1)
            return HttpResponse.json({
                success: true,
                message: 'Profile deleted successfully',
            })
        }

        return HttpResponse.json({
            success: false,
            error: 'Profile not found',
        }, { status: 404 })
    }),

    // ============================================
    // Codex config preview / apply
    // Mirrors internal/server/module/configapply so the modal renders a real
    // assembled config.toml (instead of "# Loading...") and Auto Config succeeds.
    // ============================================
    http.post('/api/v1/config/preview/codex', async ({ request }) => {
        const body = await request.json().catch(() => ({})) as any
        const authMode: string = body?.authMode || 'apikey'
        const writeCatalog: boolean = body?.writeCatalog !== false
        const prefs = (body?.preferences || {}) as Record<string, string>
        const models = ['gpt-5.1-codex', 'codex-mini-latest']
        const baseUrl = 'http://localhost:3000/tingly/codex'
        const token = 'tb-mock-model-token'

        const lines: string[] = []
        lines.push(`model = "${models[0]}"`)
        lines.push('model_provider = "tingly-box"')
        if (writeCatalog) lines.push('model_catalog_json = "~/.codex/tingly-model-catalog.json"')
        // Managed reasoning/verbosity prefs — only emit the ones the user set.
        for (const [k, v] of Object.entries(prefs)) {
            if (v) lines.push(`${k} = "${v}"`)
        }
        lines.push('')
        lines.push('[model_providers.tingly-box]')
        lines.push('name = "OpenAI using Tingly Box"')
        lines.push(`base_url = "${baseUrl}"`)
        lines.push('wire_api = "responses"')
        if (authMode === 'hybrid') {
            // Hybrid keeps the gateway token in config.toml so auth.json can hold
            // a native ChatGPT login.
            lines.push(`experimental_bearer_token = "${token}"`)
            lines.push('requires_openai_auth = false')
        } else {
            // Gateway: Codex sources the key from auth.json's OPENAI_API_KEY.
            lines.push('requires_openai_auth = true')
        }
        for (const m of models) {
            lines.push('')
            lines.push(`[profiles."${m}"]`)
            lines.push(`model = "${m}"`)
            lines.push('model_provider = "tingly-box"')
        }
        const configToml = lines.join('\n') + '\n'

        // Hybrid leaves auth.json untouched → nothing to preview there.
        const authJson = authMode === 'hybrid' ? '' : JSON.stringify({ OPENAI_API_KEY: token }, null, 2)
        const catalogJson = writeCatalog
            ? JSON.stringify({ models: models.map((id) => ({ id, context_window: 272000 })) }, null, 2)
            : ''

        return HttpResponse.json({ success: true, configToml, authJson, catalogJson, models })
    }),

    http.post('/api/v1/config/apply/codex', async ({ request }) => {
        const body = await request.json().catch(() => ({})) as any
        const authMode: string = body?.authMode || 'apikey'
        const writeCatalog: boolean = body?.writeCatalog !== false
        return HttpResponse.json({
            success: true,
            configResult: {
                success: true,
                updated: true,
                message: authMode === 'chatgpt'
                    ? 'Cleared tingly gateway keys from ~/.codex/config.toml'
                    : 'Updated ~/.codex/config.toml',
            },
            authResult: {
                success: true,
                updated: authMode !== 'hybrid',
                message: authMode === 'hybrid'
                    ? 'Left ~/.codex/auth.json untouched (kept existing ChatGPT login)'
                    : 'Updated ~/.codex/auth.json',
            },
            catalogWritten: writeCatalog && authMode !== 'chatgpt',
            models: ['gpt-5.1-codex', 'codex-mini-latest'],
            message: 'Codex configuration applied',
        })
    }),
]
