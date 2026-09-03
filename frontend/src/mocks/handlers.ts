import { http, HttpResponse } from 'msw'

// ============================================
// Mock Model Requests (correlated per-request traces)
// ============================================
const mockModelRequests = [
    {
        request_id: 'req-anthropic-ok',
        time: '',
        scenario: 'anthropic',
        request_model: 'claude-sonnet-5',
        routed_model: 'claude-sonnet-5',
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
        request_model: 'gpt-5.6-sol',
        routed_model: 'claude-sonnet-5',
        provider: 'Anthropic',
        method: 'POST',
        path: '/openai/v1/chat/completions',
        status: 200,
        latency_ms: 2210,
        has_error: false,
        max_level: 'warning',
        event_count: 5,
        trace_id: 'a1b2c3d4e5f60718293a4b5c6d7e8f90',
    },
    {
        request_id: 'req-openai-fail',
        time: '',
        scenario: 'openai',
        request_model: 'gpt-5.6-sol',
        routed_model: 'gpt-5.6-sol',
        provider: 'OpenAI',
        method: 'POST',
        path: '/openai/v1/chat/completions',
        status: 502,
        latency_ms: 740,
        has_error: true,
        max_level: 'error',
        event_count: 3,
        trace_id: 'ffee1122334455667788990011223344',
    },
]

// Shared time anchor for the request-detail and trace handlers. It is
// captured once rather than per call: the two endpoints are fetched a few
// hundred ms apart, and a moving anchor would shift the spans relative to
// the events, which the journey view attaches to stages by time containment.
const MOCK_TRACE_BASE = Date.now() - 6000

// Span trees for the in-memory trace viewer, keyed by trace_id. Times are
// offsets in ms from the trace start, converted at request time.
const mockTraceSpans: Record<string, Array<{ span_id: string; parent_span_id?: string; name: string; kind: string; start: number; end: number; status_code: string; status_message?: string; attributes?: Record<string, string> }>> = {
    a1b2c3d4e5f60718293a4b5c6d7e8f90: [
        { span_id: 'r000000000000001', name: 'chat gpt-5.6-sol', kind: 'client', start: 0, end: 2210, status_code: 'Unset', attributes: { 'gen_ai.operation.name': 'chat', 'gen_ai.request.model': 'gpt-5.6-sol', 'gen_ai.response.model': 'claude-sonnet-5', 'gen_ai.provider.name': 'Anthropic', 'gen_ai.usage.input_tokens': '1204', 'gen_ai.usage.output_tokens': '388', 'tingly.scenario': 'openai', 'http.response.status_code': '200' } },
        { span_id: 's000000000000001', parent_span_id: 'r000000000000001', name: 'routing', kind: 'internal', start: 2, end: 14, status_code: 'Unset', attributes: { 'tingly.lb.service_id': 'openai-uuid/gpt-5.6-sol', 'tingly.lb.tactic': 'tier' } },
        { span_id: 'a000000000000001', parent_span_id: 'r000000000000001', name: 'failover.attempt', kind: 'internal', start: 40, end: 620, status_code: 'Error', status_message: 'upstream status 529', attributes: { 'tingly.failover.attempt': '1', 'tingly.lb.service_id': 'openai-uuid/gpt-5.6-sol', 'http.response.status_code': '529' } },
        { span_id: 'a000000000000002', parent_span_id: 'r000000000000001', name: 'failover.attempt', kind: 'internal', start: 640, end: 2180, status_code: 'Ok', attributes: { 'tingly.failover.attempt': '2', 'tingly.lb.service_id': 'anthropic-uuid/claude-sonnet-5' } },
    ],
    ffee1122334455667788990011223344: [
        { span_id: 'r000000000000002', name: 'chat gpt-5.6-sol', kind: 'client', start: 0, end: 740, status_code: 'Error', status_message: 'Bad Gateway', attributes: { 'gen_ai.operation.name': 'chat', 'gen_ai.request.model': 'gpt-5.6-sol', 'gen_ai.provider.name': 'OpenAI', 'error.type': '502', 'http.response.status_code': '502', 'tingly.scenario': 'openai' } },
        { span_id: 's000000000000002', parent_span_id: 'r000000000000002', name: 'routing', kind: 'internal', start: 1, end: 9, status_code: 'Unset', attributes: { 'tingly.lb.service_id': 'openai-uuid/gpt-5.6-sol', 'tingly.lb.tactic': 'random' } },
    ],
}

const mockRequestEvents: Record<string, Array<{ offset: number; source: string; level: string; stage?: string; message: string; fields?: Record<string, any> }>> = {
    'req-anthropic-ok': [
        { offset: 8, source: 'smart_routing', level: 'info', stage: 'routing', message: 'rule matched', fields: { outcome: 'selected', matched_rule_index: 0, selected_provider: 'Anthropic', selected_model: 'claude-sonnet-5', trace: [{ rule_index: 0, description: 'route sonnet', matched: true, ops: [{ position: 'model', operation: 'equals', matched: true, reason: 'model == claude-sonnet-5' }] }] } },
        { offset: 20, source: 'model_request', level: 'info', stage: 'transform', message: 'anthropic passthrough (no conversion)' },
        { offset: 1800, source: 'model_request', level: 'info', stage: 'upstream', message: 'upstream responded', fields: { status: 200, provider: 'Anthropic' } },
        { offset: 1840, source: 'http', level: 'info', message: 'POST /anthropic/v1/messages 200', fields: { status: 200, latency: 1840000000, method: 'POST', path: '/anthropic/v1/messages' } },
    ],
    'req-openai-routed': [
        { offset: 10, source: 'smart_routing', level: 'info', stage: 'routing', message: 'rule matched', fields: { outcome: 'selected', matched_rule_index: 1, selected_provider: 'Anthropic', selected_model: 'claude-sonnet-5', trace: [{ rule_index: 0, description: 'keep gpt', matched: false, ops: [{ position: 'model', operation: 'equals', matched: false, reason: 'model != gpt-5.6-terra' }] }, { rule_index: 1, description: 'upgrade to sonnet', matched: true, ops: [{ position: 'model', operation: 'prefix', matched: true, reason: 'model startswith gpt-5.6' }] }] } },
        { offset: 25, source: 'model_request', level: 'warning', stage: 'transform', message: 'dropped unsupported field: logprobs' },
        { offset: 30, source: 'model_request', level: 'info', stage: 'transform', message: 'openai chat -> anthropic messages' },
        { offset: 2170, source: 'model_request', level: 'info', stage: 'upstream', message: 'upstream responded', fields: { status: 200, provider: 'Anthropic' } },
        { offset: 2210, source: 'http', level: 'info', message: 'POST /openai/v1/chat/completions 200', fields: { status: 200, latency: 2210000000, method: 'POST', path: '/openai/v1/chat/completions' } },
    ],
    'req-openai-fail': [
        { offset: 5, source: 'smart_routing', level: 'info', stage: 'routing', message: 'rule matched', fields: { outcome: 'selected', matched_rule_index: 0, selected_provider: 'OpenAI', selected_model: 'gpt-5.6-sol' } },
        { offset: 700, source: 'model_request', level: 'error', stage: 'upstream', message: 'upstream call failed: 502 Bad Gateway', fields: { status: 502, provider: 'OpenAI', error: 'bad gateway' } },
        { offset: 740, source: 'http', level: 'error', message: 'POST /openai/v1/chat/completions 502', fields: { status: 502, latency: 740000000, method: 'POST', path: '/openai/v1/chat/completions', error: 'upstream error' } },
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

const mockGuardrailsGroups: Array<{
    id: string
    name: string
    enabled: boolean
    severity: string
}> = [
    { id: 'default', name: 'Default', enabled: true, severity: 'high' },
]

// ============================================
// Mock Providers (v2 API with uuid)
// ============================================
interface MockProviderCatalogEntry {
    provider: {
        uuid: string
        name: string
        api_base: string
        api_style: string
        auth_type: string
        token: string
        enabled: boolean
        proxy_url: string
        api_base_openai: string | null
        api_base_anthropic: string | null
        oauth_detail?: {
            access_token: string
            refresh_token: string
            expires_at: string
            issuer: string
            provider_type: string
            user_id: string
            device_id?: string
            extra_fields?: Record<string, string>
        }
    }
    models: string[]
}

// One source of truth for standard provider metadata and its available models.
// Rules and provider-model endpoints are validated against this catalog below.
const mockStandardProviderCatalog: MockProviderCatalogEntry[] = [
    {
        provider: {
            uuid: 'mock-provider-anthropic', name: 'Anthropic',
            api_base: 'https://api.anthropic.com', api_style: 'anthropic', auth_type: 'api_key',
            token: 'sk-ant-****abcd', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: 'https://api.anthropic.com',
        },
        models: [
            'claude-sonnet-5', 'claude-opus-4-8', 'claude-fable-5',
            'claude-haiku-4-5',
        ],
    },
    {
        provider: {
            uuid: 'mock-provider-openai', name: 'OpenAI',
            api_base: 'https://api.openai.com/v1', api_style: 'openai', auth_type: 'api_key',
            token: 'sk-****efgh', enabled: true, proxy_url: '',
            api_base_openai: 'https://api.openai.com/v1', api_base_anthropic: null,
        },
        models: [
            'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna',
            'gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini',
            'o3', 'o3-mini', 'gpt-image-1',
        ],
    },
    {
        provider: {
            uuid: 'mock-provider-gemini', name: 'Google Gemini',
            api_base: 'https://generativelanguage.googleapis.com/v1beta/openai', api_style: 'openai', auth_type: 'api_key',
            token: 'AIza****uvwx', enabled: true, proxy_url: '',
            api_base_openai: 'https://generativelanguage.googleapis.com/v1beta/openai', api_base_anthropic: null,
        },
        models: [
            'gemini-3.1-pro-preview', 'gemini-3.5-flash',
            'gemini-2.5-flash-lite',
        ],
    },
    {
        provider: {
            uuid: 'mock-provider-qwen', name: 'Alibaba Dashscope',
            api_base: 'https://dashscope-intl.aliyuncs.com/compatible-mode/v1', api_style: 'openai', auth_type: 'api_key',
            token: 'sk-qwen-****uvwx', enabled: true, proxy_url: '',
            api_base_openai: 'https://dashscope-intl.aliyuncs.com/compatible-mode/v1', api_base_anthropic: null,
        },
        models: ['qwen3.5-plus', 'qwen3-max', 'qwen3-coder-next', 'qwen3-coder-plus'],
    },
    {
        provider: {
            uuid: 'mock-provider-deepseek', name: 'DeepSeek',
            api_base: 'https://api.deepseek.com/v1', api_style: 'openai', auth_type: 'api_key',
            token: 'sk-ds-****qrst', enabled: true, proxy_url: '',
            api_base_openai: 'https://api.deepseek.com/v1', api_base_anthropic: 'https://api.deepseek.com/anthropic',
        },
        models: ['deepseek-v4-pro', 'deepseek-v4-flash'],
    },
    {
        provider: {
            uuid: 'mock-provider-zai', name: 'Z.ai',
            api_base: 'https://api.z.ai/api/paas/v4', api_style: 'openai', auth_type: 'api_key',
            token: 'zai-****yzab', enabled: true, proxy_url: '',
            api_base_openai: 'https://api.z.ai/api/paas/v4', api_base_anthropic: 'https://api.z.ai/api/anthropic',
        },
        models: ['glm-5.1', 'glm-5-turbo', 'glm-5'],
    },
    {
        provider: {
            uuid: 'mock-provider-minimax', name: 'MiniMax',
            api_base: 'https://api.minimax.io/v1', api_style: 'openai', auth_type: 'api_key',
            token: 'mm-****wxyz', enabled: true, proxy_url: '',
            api_base_openai: 'https://api.minimax.io/v1', api_base_anthropic: null,
        },
        models: ['MiniMax-M2', 'MiniMax-Text-01'],
    },
    {
        provider: {
            uuid: 'mock-provider-openrouter', name: 'OpenRouter',
            api_base: 'https://openrouter.ai/api/v1', api_style: 'openai', auth_type: 'api_key',
            token: 'sk-or-****ijkl', enabled: false, proxy_url: '',
            api_base_openai: 'https://openrouter.ai/api/v1', api_base_anthropic: null,
        },
        models: [
            'anthropic/claude-sonnet-5', 'openai/gpt-5.6-sol',
            'deepseek/deepseek-v4-pro', 'qwen/qwen3.5-plus',
            'google/gemini-3.1-pro-preview',
        ],
    },
]

const mockOAuthExpiresAt = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString()

const mockOAuthProviderCatalog: MockProviderCatalogEntry[] = [
    {
        provider: {
            uuid: 'mock-oauth-claude-code', name: 'Claude Code OAuth',
            api_base: 'https://api.anthropic.com', api_style: 'anthropic', auth_type: 'oauth',
            token: '', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: null,
            oauth_detail: {
                access_token: 'mock-claude-code-access-token',
                refresh_token: 'mock-claude-code-refresh-token',
                expires_at: mockOAuthExpiresAt,
                issuer: 'claude_code',
                provider_type: 'claude_code',
                user_id: 'mock-claude-user',
            },
        },
        models: ['claude-opus-4-8', 'claude-sonnet-5', 'claude-haiku-4-5'],
    },
    {
        provider: {
            uuid: 'mock-oauth-gemini', name: 'Gemini CLI OAuth',
            api_base: 'https://cloudcode-pa.googleapis.com', api_style: 'openai', auth_type: 'oauth',
            token: '', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: null,
            oauth_detail: {
                access_token: 'mock-gemini-access-token',
                refresh_token: 'mock-gemini-refresh-token',
                expires_at: mockOAuthExpiresAt,
                issuer: 'gemini',
                provider_type: 'gemini',
                user_id: 'gemini.mock@example.com',
            },
        },
        models: ['gemini-3.1-pro-preview', 'gemini-3.5-flash', 'gemini-2.5-flash-lite'],
    },
    {
        provider: {
            uuid: 'mock-oauth-antigravity', name: 'Antigravity OAuth',
            api_base: 'https://cloudcode-pa.googleapis.com', api_style: 'openai', auth_type: 'oauth',
            token: '', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: null,
            oauth_detail: {
                access_token: 'mock-antigravity-access-token',
                refresh_token: 'mock-antigravity-refresh-token',
                expires_at: mockOAuthExpiresAt,
                issuer: 'antigravity',
                provider_type: 'antigravity',
                user_id: 'antigravity.mock@example.com',
            },
        },
        models: ['gemini-3.1-pro-preview', 'gemini-3.5-flash'],
    },
    {
        provider: {
            uuid: 'mock-oauth-qwen-code', name: 'Qwen Code OAuth',
            api_base: 'https://portal.qwen.ai/v1', api_style: 'openai', auth_type: 'oauth',
            token: '', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: null,
            oauth_detail: {
                access_token: 'mock-qwen-code-access-token',
                refresh_token: 'mock-qwen-code-refresh-token',
                expires_at: mockOAuthExpiresAt,
                issuer: 'qwen_code',
                provider_type: 'qwen_code',
                user_id: 'mock-qwen-user',
            },
        },
        models: ['qwen3-coder-next', 'qwen3-coder-plus'],
    },
    {
        provider: {
            uuid: 'mock-oauth-codex', name: 'Codex OAuth',
            api_base: 'https://chatgpt.com/backend-api', api_style: 'openai', auth_type: 'oauth',
            token: '', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: null,
            oauth_detail: {
                access_token: 'mock-codex-access-token',
                refresh_token: 'mock-codex-refresh-token',
                expires_at: mockOAuthExpiresAt,
                issuer: 'codex',
                provider_type: 'codex',
                user_id: 'mock-codex-user',
                extra_fields: { account_id: 'acct_mock_codex' },
            },
        },
        models: ['gpt-5.6-sol', 'gpt-5.5', 'gpt-5.4-mini'],
    },
    {
        provider: {
            uuid: 'mock-oauth-kimi-code', name: 'Kimi Code OAuth',
            api_base: 'https://api.kimi.com/coding/v1', api_style: 'openai', auth_type: 'oauth',
            token: '', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: null,
            oauth_detail: {
                access_token: 'mock-kimi-code-access-token',
                refresh_token: 'mock-kimi-code-refresh-token',
                expires_at: mockOAuthExpiresAt,
                issuer: 'kimi_code',
                provider_type: 'kimi_code',
                user_id: 'com8n51luj0',
                device_id: 'mock-kimi-device',
            },
        },
        models: ['kimi-for-coding', 'kimi-k2.5'],
    },
    {
        provider: {
            uuid: 'mock-oauth-testing', name: 'Mock OAuth',
            api_base: 'https://oauth-mock.mock.beeceptor.com', api_style: 'openai', auth_type: 'oauth',
            token: '', enabled: true, proxy_url: '',
            api_base_openai: null, api_base_anthropic: null,
            oauth_detail: {
                access_token: 'mock-oauth-access-token',
                refresh_token: 'mock-oauth-refresh-token',
                expires_at: mockOAuthExpiresAt,
                issuer: 'mock',
                provider_type: 'mock',
                user_id: 'mock-oauth-user',
            },
        },
        models: ['mock-chat-model', 'mock-reasoning-model'],
    },
]

const mockVirtualProviders = [
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
            models: ['claude-sonnet-5', 'claude-opus-4-8', 'gpt-5.6-sol', 'qwen3.5-plus', 'deepseek-v4-pro', 'glm-5.1'],
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
            models: ['claude-sonnet-5', 'claude-opus-4-8', 'claude-haiku-4-5'],
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
            models: ['gemini-3.1-pro-preview', 'gemini-3.5-flash'],
        },
    },
]

const getMockProviders = () => [
    ...mockStandardProviderCatalog.map(({ provider }) => provider),
    ...mockOAuthProviderCatalog.map(({ provider }) => provider),
    ...mockVirtualProviders,
]

const getMockProviderModels = (uuid: string): string[] => {
    const standardProvider = mockStandardProviderCatalog.find(({ provider }) => provider.uuid === uuid)
    if (standardProvider) return standardProvider.models

    const oauthProvider = mockOAuthProviderCatalog.find(({ provider }) => provider.uuid === uuid)
    if (oauthProvider) return oauthProvider.models

    return mockVirtualProviders.find((provider) => provider.uuid === uuid)?.vmodel_detail.models ?? []
}

// ============================================
// Mock Rules per scenario
// ============================================
const mockV1Rules: Record<string, any[]> = {
    openai: [
        {
            uuid: 'mock-rule-openai-1',
            scenario: 'openai',
            request_model: 'gpt-5.6-sol',
            response_model: '',
            active: true,
            description: 'Route gpt-5.6-sol to Anthropic claude-opus-4-8',
            flags: { cursor_compat: true, thinking_effort: 'high' },
            services: [{ uuid: 'svc-o1', provider: 'mock-provider-anthropic', model: 'claude-opus-4-8', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-openai-2',
            scenario: 'openai',
            request_model: 'gpt-5.6-luna',
            response_model: '',
            active: true,
            description: 'Route gpt-5.6-luna to Deepseek',
            services: [{ uuid: 'svc-o2', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-openai-3',
            scenario: 'openai',
            request_model: 'gpt-5.6-terra',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-o3a', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true },
                { uuid: 'svc-o3b', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true },
            ],
        },
    ],
    anthropic: [
        {
            uuid: 'mock-rule-ant-1',
            scenario: 'anthropic',
            request_model: 'claude-opus-4-8',
            response_model: '',
            active: true,
            description: 'Opus 4.8 → GLM 5.1',
            services: [{ uuid: 'svc-a1', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-ant-2',
            scenario: 'anthropic',
            request_model: 'claude-sonnet-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-a2', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-ant-3',
            scenario: 'anthropic',
            request_model: 'claude-haiku-4-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-a3', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true }],
        },
    ],
    claude_code: [
        {
            // Unified-mode rule fetched by GET /api/v1/rule/builtin:claude_code:cc.
            // Demonstrates smart routing with fabric conditions + tiered default fallback.
            uuid: 'builtin:claude_code:cc',
            scenario: 'claude_code',
            request_model: 'claude-sonnet-5',
            response_model: '',
            active: true,
            description: 'Smart routing + tiered fallback',
            flags: { claude_code_compat: true, clean_header: true, skip_usage: true, session_affinity: 3600 },
            // Default providers in 3 tiers: T0 primary, T1 secondary, T2 budget
            services: [
                { uuid: 'svc-cc-t0-a', provider: 'mock-provider-anthropic', model: 'claude-sonnet-5', weight: 1, active: true, tier: 0 },
                { uuid: 'svc-cc-t0-b', provider: 'mock-provider-openai', model: 'gpt-5.6-sol', weight: 1, active: true, tier: 0 },
                { uuid: 'svc-cc-t1-a', provider: 'mock-provider-gemini', model: 'gemini-3.5-flash', weight: 1, active: true, tier: 1 },
                { uuid: 'svc-cc-t2-a', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true, tier: 2 },
                { uuid: 'svc-cc-t2-b', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true, tier: 2 },
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
                    services: [{ uuid: 'svc-sm-cc-bi-cmp', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
                },
                {
                    uuid: 'smart-cc-bi-tok',
                    description: 'Large context ≥ 60k tokens → Gemini',
                    ops: [{ uuid: 'op-cc-bi-tok', position: 'token', operation: 'ge', value: '60000' }],
                    services: [{ uuid: 'svc-sm-cc-bi-tok', provider: 'mock-provider-gemini', model: 'gemini-3.5-flash', weight: 1, active: true }],
                },
                {
                    uuid: 'smart-cc-bi-default',
                    description: 'Default (unconditional fallback)',
                    ops: [],
                    services: [
                        { uuid: 'svc-sm-cc-bi-def-a', provider: 'mock-provider-anthropic', model: 'claude-sonnet-5', weight: 1, active: true },
                        { uuid: 'svc-sm-cc-bi-def-b', provider: 'mock-provider-openai', model: 'gpt-5.6-sol', weight: 1, active: true },
                    ],
                },
            ],
        },
        {
            uuid: 'mock-rule-cc-smart',
            scenario: 'claude_code',
            request_model: 'claude-sonnet-5',
            response_model: '',
            active: true,
            description: 'Smart routing by agent kind',
            services: [
                { uuid: 'svc-cc-default-a', provider: 'mock-provider-anthropic', model: 'claude-sonnet-5', weight: 1, active: true },
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
                    services: [{ uuid: 'svc-sm-cc-cmp', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
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
            request_model: 'claude-opus-4-8',
            response_model: '',
            active: true,
            description: 'Direct load-balance across Anthropic + GLM',
            services: [
                { uuid: 'svc-cc-dir-a', provider: 'mock-provider-anthropic', model: 'claude-opus-4-8', weight: 1, active: true },
                { uuid: 'svc-cc-dir-b', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true },
            ],
        },
    ],
    claude_desktop: [
        {
            uuid: 'mock-rule-cd-1',
            scenario: 'claude_desktop',
            request_model: 'claude-sonnet-5',
            response_model: '',
            active: true,
            description: 'Claude Desktop - Sonnet 5 for balanced performance',
            flags: { context_1m: true },
            services: [{ uuid: 'svc-cd4', provider: 'mock-provider-deepseek', model: 'deepseek-v4-flash', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cd-2',
            scenario: 'claude_desktop',
            request_model: 'claude-opus-4-8',
            response_model: '',
            active: true,
            description: 'Claude Desktop - Opus 4.8 for complex tasks',
            services: [{ uuid: 'svc-cd2', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cd-3',
            scenario: 'claude_desktop',
            request_model: 'claude-opus-4-8',
            response_model: '',
            active: true,
            description: 'Claude Desktop - Opus 4.8 for advanced reasoning',
            services: [{ uuid: 'svc-cd3', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-cd-4',
            scenario: 'claude_desktop',
            request_model: 'claude-haiku-4-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-cd1', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
        },
    ],
    codex: [
        {
            uuid: 'mock-rule-codex-1',
            scenario: 'codex',
            request_model: 'gpt-5.6-luna',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-cx1', provider: 'mock-provider-openai', model: 'gpt-5.6-luna', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-codex-2',
            scenario: 'codex',
            request_model: 'gpt-5.6-terra',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-cx2a', provider: 'mock-provider-anthropic', model: 'claude-opus-4-8', weight: 1, active: true },
                { uuid: 'svc-cx2b', provider: 'mock-provider-gemini', model: 'gemini-3.1-pro-preview', weight: 1, active: true },
            ],
        },
    ],
    custom: [
        {
            uuid: 'mock-rule-custom-1',
            scenario: 'custom',
            request_model: 'claude-opus-4-8',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-ag1', provider: 'mock-provider-anthropic', model: 'claude-opus-4-8', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-custom-2',
            scenario: 'custom',
            request_model: 'gpt-5.6-sol',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-ag2a', provider: 'mock-provider-openai', model: 'gpt-5.6-sol', weight: 1, active: true },
                { uuid: 'svc-ag2b', provider: 'mock-provider-gemini', model: 'gemini-3.1-pro-preview', weight: 1, active: true },
            ],
        },
    ],
    team: [
        {
            uuid: 'mock-rule-team-claude-opus',
            scenario: 'team',
            request_model: 'claude-opus-4-8',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-team-claude-opus', provider: 'mock-provider-anthropic', model: 'claude-opus-4-8', weight: 1, active: true },
            ],
        },
        {
            uuid: 'mock-rule-team-claude-sonnet',
            scenario: 'team',
            request_model: 'claude-sonnet-5',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-team-claude-sonnet', provider: 'mock-provider-anthropic', model: 'claude-sonnet-5', weight: 1, active: true },
            ],
        },
        {
            uuid: 'mock-rule-team-gpt',
            scenario: 'team',
            request_model: 'gpt-5.6-sol',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-team-gpt', provider: 'mock-provider-openai', model: 'gpt-5.6-sol', weight: 1, active: true },
            ],
        },
        {
            uuid: 'mock-rule-team-qwen',
            scenario: 'team',
            request_model: 'qwen3.5-plus',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-team-qwen', provider: 'mock-provider-qwen', model: 'qwen3.5-plus', weight: 1, active: true },
            ],
        },
        {
            uuid: 'mock-rule-team-deepseek',
            scenario: 'team',
            request_model: 'deepseek-v4-pro',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-team-deepseek', provider: 'mock-provider-deepseek', model: 'deepseek-v4-pro', weight: 1, active: true },
            ],
        },
        {
            uuid: 'mock-rule-team-glm',
            scenario: 'team',
            request_model: 'glm-5.1',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-team-glm', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true },
            ],
        },
    ],
    imagegen: [
        {
            uuid: 'mock-rule-imagegen-openai',
            scenario: 'imagegen',
            request_model: 'gpt-image-1',
            response_model: '',
            active: true,
            description: '',
            services: [
                { uuid: 'svc-imagegen-openai', provider: 'mock-provider-openai', model: 'gpt-image-1', weight: 1, active: true },
            ],
        },
    ],
    vscode: [
        {
            uuid: 'mock-rule-vsc-1',
            scenario: 'vscode',
            request_model: 'claude-sonnet-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-vs1', provider: 'mock-provider-anthropic', model: 'claude-sonnet-5', weight: 1, active: true }],
        },
    ],
    xcode: [
        {
            uuid: 'mock-rule-xc-1',
            scenario: 'xcode',
            request_model: 'claude-sonnet-5',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-xc1', provider: 'mock-provider-zai', model: 'glm-5.1', weight: 1, active: true }],
        },
    ],
    opencode: [
        {
            uuid: 'mock-rule-oc-1',
            scenario: 'opencode',
            request_model: 'claude-opus-4-8',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-oc1', provider: 'mock-provider-anthropic', model: 'claude-opus-4-8', weight: 1, active: true }],
        },
        {
            uuid: 'mock-rule-oc-2',
            scenario: 'opencode',
            request_model: 'gpt-5.6-sol',
            response_model: '',
            active: true,
            description: '',
            services: [{ uuid: 'svc-oc2', provider: 'mock-provider-openai', model: 'gpt-5.6-sol', weight: 1, active: true }],
        },
    ],
}

const getMockRuleServiceErrors = (scenario: string, rule: any): string[] => {
    const errors: string[] = []
    const serviceGroups = [
        rule.services ?? [],
        ...(rule.smart_routing ?? []).map((smartRule: any) => smartRule.services ?? []),
    ]

    serviceGroups.flat().forEach((service: any) => {
        const provider = mockStandardProviderCatalog.find(({ provider }) => provider.uuid === service.provider)
        if (!provider) {
            errors.push(`${scenario}/${rule.uuid}: unknown provider ${service.provider}`)
        } else if (!provider.models.includes(service.model)) {
            errors.push(`${scenario}/${rule.uuid}: ${service.model} is not offered by ${service.provider}`)
        }
    })

    return errors
}

const getMockRuleErrors = (scenario: string, rule: any, validateRequestModel = false): string[] => {
    const errors = getMockRuleServiceErrors(scenario, rule)
    const catalogModels = new Set(mockStandardProviderCatalog.flatMap(({ models }) => models))
    if (validateRequestModel && rule.request_model && rule.request_model !== '*' && !catalogModels.has(rule.request_model)) {
        errors.push(`${scenario}/${rule.uuid}: request model ${rule.request_model} is not in the catalog`)
    }
    return errors
}

const validateMockRuleCatalog = () => {
    const errors: string[] = []
    const providerUuids = new Set<string>()

    mockStandardProviderCatalog.forEach(({ provider, models }) => {
        if (providerUuids.has(provider.uuid)) {
            errors.push(`duplicate provider UUID ${provider.uuid}`)
        }
        providerUuids.add(provider.uuid)

        const uniqueModels = new Set(models)
        if (uniqueModels.size !== models.length) {
            errors.push(`${provider.uuid}: duplicate model entries`)
        }
    })

    Object.entries(mockV1Rules).forEach(([scenario, rules]) => {
        rules.forEach((rule) => {
            errors.push(...getMockRuleErrors(scenario, rule, true))
        })
    })

    if (errors.length > 0) {
        throw new Error(`Invalid mock provider catalog:\n${errors.join('\n')}`)
    }
}

validateMockRuleCatalog()

const getMockRulesForScenario = (scenario: string): any[] => {
    if (!mockV1Rules[scenario] && scenario.startsWith('claude_code:')) {
        const profileId = scenario.slice('claude_code:'.length)
        mockV1Rules[scenario] = [
            {
                uuid: `mock-rule-cc-${profileId}-1`,
                scenario,
                request_model: 'claude-sonnet-5',
                response_model: '',
                active: true,
                description: `Profile ${profileId} - Smart routing rule`,
                flags: { claude_code_compat: true, clean_header: true },
                services: [
                    { uuid: `svc-cc-${profileId}-1`, provider: 'mock-provider-anthropic', model: 'claude-sonnet-5', weight: 1, active: true },
                ],
            },
        ]
    }

    return mockV1Rules[scenario] ?? []
}

// ============================================
// Mock Quota Data
// ============================================
const now = new Date()
const inOneHour = new Date(now.getTime() + 60 * 60 * 1000).toISOString()
const inTwoDays = new Date(now.getTime() + 2 * 24 * 60 * 60 * 1000).toISOString()
const inSixDays = new Date(now.getTime() + 6 * 24 * 60 * 60 * 1000).toISOString()
const inThirtyDays = new Date(now.getTime() + 30 * 24 * 60 * 60 * 1000).toISOString()

// Payloads mirror build/Taskfile.quota.yml, and the normalized windows mirror
// what the fetchers actually produce from them — see the pinned expectations in
// ai/quota/fetcher/taskfile_samples_test.go. Between them these cover every
// rendering state: an allowance, a balance, an uncapped window, a scoped limit
// that must not answer for the account, and a quota we cannot read at all.
const mockQuotas: Record<string, any> = {
    // Nothing readable: the reason is recorded and no windows are invented.
    // The credential row still renders — quota area empty, refresh still
    // reachable — and the reason stays available via Details (raw_response).
    // This mirrors the "upstream answered, but the answer is a refusal" case
    // (e.g. user has no coding plan): the fetcher keeps the payload.
    'mock-provider-anthropic': {
        provider_uuid: 'mock-provider-anthropic',
        provider_name: 'Anthropic',
        provider_type: 'anthropic',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        last_error: 'user does not have an active coding plan',
        last_error_at: now.toISOString(),
        raw_response: {
            type: 'error',
            error: {
                type: 'permission_error',
                message: 'user does not have an active coding plan',
            },
        },
    },

    // Fetch failed outright: no payload came back, so no raw_response either —
    // the row renders with an empty quota area and no Details button.

    'mock-provider-openai': {
        provider_uuid: 'mock-provider-openai',
        provider_name: 'OpenAI',
        provider_type: 'openai',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        last_error: 'quota API not available - see the OpenAI dashboard',
        last_error_at: now.toISOString(),
    },

    // Free key: monthly spend has no cap, so there is no percentage to report.
    // The key limit caps lifetime usage, not the month.
    'mock-provider-openrouter': {
        provider_uuid: 'mock-provider-openrouter',
        provider_name: 'OpenRouter',
        provider_type: 'openrouter',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        windows: [
            {
                key: 'monthly',
                type: 'monthly',
                unlimited: true,
                used: 8.1,
                limit: 0,
                used_percent: 0,
                window_minutes: 43200,
                unit: 'currency',
                label: 'Monthly Usage',
                description: 'This month: $8.1000 (no limit set)',
            },
        ],
        account: { id: 'user_xxx', tier: 'free' },
        raw_response: {
            data: {
                label: 'sk-or-v1-c1a...5a2',
                limit: null,
                limit_remaining: null,
                usage: 8.1,
                usage_daily: 0.4,
                usage_weekly: 2.2,
                usage_monthly: 8.1,
                is_free_tier: true,
                creator_user_id: 'user_xxx',
            },
        },
    },

    // GLM's captured form: a spent weekly allowance, plus a monthly MCP limit
    // scoped to that feature so it neither gates the account nor softens it.
    'mock-provider-zai': {
        provider_uuid: 'mock-provider-zai',
        provider_name: 'Z.ai',
        provider_type: 'glm',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        windows: [
            {
                key: 'TOKENS_LIMIT_3_5',
                type: 'session',
                used: 0,
                limit: 100,
                used_percent: 0,
                window_minutes: 300,
                unit: 'tokens',
                label: '5-hour Tokens',
                description: '0% utilization',
            },
            {
                key: 'TOKENS_LIMIT_6_1',
                type: 'weekly',
                used: 100,
                limit: 100,
                used_percent: 100,
                resets_at: inSixDays,
                window_minutes: 10080,
                unit: 'tokens',
                label: '1-week Tokens',
                description: '100% utilization',
            },
        ],
        breakdowns: [
            {
                key: 'mcp',
                label: '1-month MCP',
                group: 'feature',
                windows: [{
                    type: 'monthly',
                    used: 63,
                    limit: 4000,
                    used_percent: 1.575,
                    resets_at: inThirtyDays,
                    window_minutes: 43200,
                    unit: 'requests',
                    label: '1-month MCP',
                    description: '63 / 4000',
                }],
            },
            {
                key: 'search-prime',
                label: 'search-prime',
                group: 'resource',
                windows: [{
                    type: 'monthly',
                    used: 71,
                    limit: 4000,
                    used_percent: 1.775,
                    resets_at: inThirtyDays,
                    window_minutes: 43200,
                    unit: 'requests',
                    label: '1-month MCP',
                    description: '71 / 4000 · 113% of usage',
                }],
            },
            {
                key: 'web-reader',
                label: 'web-reader',
                group: 'resource',
                windows: [{
                    type: 'monthly',
                    used: 5,
                    limit: 4000,
                    used_percent: 0.125,
                    resets_at: inThirtyDays,
                    window_minutes: 43200,
                    unit: 'requests',
                    label: '1-month MCP',
                    description: '5 / 4000 · 8% of usage',
                }],
            },
        ],
        account: { tier: 'max' },
        raw_response: {
            code: 200,
            msg: 'ok',
            success: true,
            data: {
                level: 'max',
                limits: [
                    { type: 'TOKENS_LIMIT', unit: 3, number: 5, percentage: 0 },
                    { type: 'TOKENS_LIMIT', unit: 6, number: 1, percentage: 100 },
                    {
                        type: 'TIME_LIMIT', unit: 5, number: 1, usage: 4000, currentValue: 63,
                        remaining: 3937, percentage: 1,
                        usageDetails: [
                            { modelCode: 'search-prime', usage: 71 },
                            { modelCode: 'web-reader', usage: 5 },
                            { modelCode: 'zread', usage: 0 },
                        ],
                    },
                ],
            },
        },
    },

    // The add-on is enabled but upstream will not say how much of it is used.
    // It must not report 0% — that reads as untouched and safe to spend against.
    'mock-oauth-claude-code': {
        provider_uuid: 'mock-oauth-claude-code',
        provider_name: 'Claude Code OAuth',
        provider_type: 'anthropic',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        windows: [
            {
                key: 'five_hour',
                type: 'session',
                used: 62,
                limit: 100,
                used_percent: 62,
                resets_at: inOneHour,
                window_minutes: 300,
                unit: 'percent',
                label: '5-Hour Window',
                description: '62% utilization',
            },
            {
                key: 'seven_day',
                type: 'weekly',
                used: 88,
                limit: 100,
                used_percent: 88,
                resets_at: inSixDays,
                window_minutes: 10080,
                unit: 'percent',
                label: '7-Day Window',
                description: '88% utilization',
            },
            {
                key: 'extra_usage',
                type: 'monthly',
                unknown: true,
                used: 0,
                limit: 0,
                used_percent: 0,
                window_minutes: 43200,
                unit: 'percent',
                label: 'Extra Usage',
                description: 'utilization unavailable',
            },
        ],
        cost: {
            used: 0,
            limit: 75,
            currency_code: 'USD',
            label: 'Extra Usage',
        },
        account: {
            id: 'acc-mock-001',
            name: 'Mock Account',
            email: 'mock@example.com',
            tier: 'max',
        },
        raw_response: {
            five_hour: { utilization: 62, resets_at: inOneHour },
            seven_day: { utilization: 88, resets_at: inSixDays },
            extra_usage: { is_enabled: true, monthly_limit: 7500, used_credits: 0, utilization: null },
        },
    },

    // The account figure names the most-used model. An average would have
    // reported 17.5% here and hidden which model is actually running down.
    'mock-oauth-gemini': {
        provider_uuid: 'mock-oauth-gemini',
        provider_name: 'Gemini CLI OAuth',
        provider_type: 'gemini',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        windows: [
            {
                key: 'tightest',
                type: 'daily',
                used: 25,
                limit: 100,
                used_percent: 25,
                resets_at: inOneHour,
                window_minutes: 1440,
                unit: 'percent',
                label: 'gemini-2.5-pro',
                description: '25% used — most-used of 2 models',
            },
        ],
        breakdowns: [
            {
                key: 'gemini-2.5-pro',
                label: 'gemini-2.5-pro',
                group: 'resource',
                windows: [{
                    type: 'daily', used: 25, limit: 100, used_percent: 25,
                    resets_at: inOneHour, window_minutes: 1440, unit: 'percent',
                    label: 'Daily', description: '25% used',
                }],
            },
            {
                key: 'gemini-2.5-flash',
                label: 'gemini-2.5-flash',
                group: 'resource',
                windows: [{
                    type: 'daily', used: 10, limit: 100, used_percent: 10,
                    resets_at: inOneHour, window_minutes: 1440, unit: 'percent',
                    label: 'Daily', description: '10% used',
                }],
            },
        ],
        raw_response: {
            buckets: [
                { modelId: 'gemini-2.5-pro', remainingFraction: 0.75, resetTime: inOneHour },
                { modelId: 'gemini-2.5-flash', remainingFraction: 0.9, resetTime: inOneHour },
            ],
        },
    },

    // The rate-limited free-plan sample. Upstream calls the spent window the
    // primary one but reports 604800s for it, so its type comes from the
    // length. Spark is untouched but scoped to its own model, so it neither
    // rescues nor gates the account.
    'mock-oauth-codex': {
        provider_uuid: 'mock-oauth-codex',
        provider_name: 'Codex OAuth',
        provider_type: 'codex',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        windows: [
            {
                key: 'current',
                type: 'weekly',
                used: 100,
                limit: 100,
                used_percent: 100,
                resets_at: inOneHour,
                window_minutes: 10080,
                unit: 'percent',
                label: 'Current Window',
                description: '168h window, 100% used',
                allowed: false,
                limit_reached: true,
            },
            {
                key: 'weekly',
                type: 'weekly',
                used: 87,
                limit: 100,
                used_percent: 87,
                resets_at: inTwoDays,
                window_minutes: 10080,
                unit: 'percent',
                label: 'Weekly',
                description: '7d window, 87% used',
            },
        ],
        breakdowns: [
            {
                key: 'codex_bengalfox',
                label: 'GPT-5.3-Codex-Spark',
                group: 'model',
                windows: [{
                    type: 'session', used: 0, limit: 100, used_percent: 0,
                    resets_at: inOneHour, window_minutes: 300, unit: 'percent',
                    label: 'GPT-5.3-Codex-Spark',
                    description: 'GPT-5.3-Codex-Spark: 0% used',
                    allowed: true, limit_reached: false,
                }],
            },
        ],
        account: {
            id: 'acct_mock_codex',
            email: 'codex.mock@example.com',
            tier: 'free',
        },
        raw_response: {
            plan_type: 'free',
            rate_limit: {
                allowed: false,
                limit_reached: true,
                primary_window: {
                    used_percent: 100,
                    limit_window_seconds: 604800,
                    reset_after_seconds: 943,
                    reset_at: Math.floor(new Date(inOneHour).getTime() / 1000),
                },
                secondary_window: {
                    used_percent: 87,
                    limit_window_seconds: 604800,
                    reset_after_seconds: 244631,
                    reset_at: Math.floor(new Date(inTwoDays).getTime() / 1000),
                },
            },
            code_review_rate_limit: null,
            additional_rate_limits: [{
                limit_name: 'GPT-5.3-Codex-Spark',
                metered_feature: 'codex_bengalfox',
                rate_limit: {
                    allowed: true,
                    limit_reached: false,
                    primary_window: {
                        used_percent: 0,
                        limit_window_seconds: 18000,
                        reset_at: Math.floor(new Date(inOneHour).getTime() / 1000),
                    },
                },
            }],
            credits: { has_credits: false, unlimited: false, balance: '0' },
            rate_limit_reset_credits: { available_count: 4 },
        },
    },

    // Real MiniMax capture. The counts come back 0/0 and the remaining
    // percentages carry the whole story. general answers for the account —
    // video is bundled media the gateway never sends a request to, so a spent
    // video quota must not make MiniMax look exhausted.
    'mock-provider-minimax': {
        provider_uuid: 'mock-provider-minimax',
        provider_name: 'MiniMax',
        provider_type: 'minimax',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        windows: [
            {
                key: 'interval',
                type: 'session',
                used: 60,
                limit: 100,
                used_percent: 60,
                resets_at: inOneHour,
                window_minutes: 300,
                unit: 'percent',
                label: 'Interval Quota',
                description: '60% used · general',
            },
            {
                key: 'weekly',
                type: 'weekly',
                used: 12,
                limit: 100,
                used_percent: 12,
                resets_at: inSixDays,
                window_minutes: 10080,
                unit: 'percent',
                label: 'Weekly Quota',
                description: '12% used · general',
            },
        ],
        breakdowns: [
            {
                key: 'video',
                label: 'video',
                group: 'feature',
                windows: [
                    {
                        type: 'daily', used: 100, limit: 100, used_percent: 100,
                        resets_at: inTwoDays, window_minutes: 1440, unit: 'percent',
                        label: 'Interval', description: '100% used',
                    },
                    {
                        type: 'weekly', used: 100, limit: 100, used_percent: 100,
                        resets_at: inSixDays, window_minutes: 10080, unit: 'percent',
                        label: 'Weekly', description: '100% used',
                    },
                ],
            },
        ],
        raw_response: {
            model_remains: [
                {
                    start_time: 1785376800000, end_time: 1785394800000, remains_time: 14240499,
                    current_interval_total_count: 0, current_interval_usage_count: 0,
                    model_name: 'general',
                    current_weekly_total_count: 0, current_weekly_usage_count: 0,
                    weekly_start_time: 1785081600000, weekly_end_time: 1785686400000,
                    weekly_remains_time: 305840499,
                    current_interval_status: 1, current_interval_remaining_percent: 40,
                    current_weekly_status: 3, current_weekly_remaining_percent: 88,
                },
                {
                    start_time: 1785340800000, end_time: 1785427200000, remains_time: 46640499,
                    current_interval_total_count: 0, current_interval_usage_count: 0,
                    model_name: 'video',
                    current_weekly_total_count: 0, current_weekly_usage_count: 0,
                    weekly_start_time: 1785081600000, weekly_end_time: 1785686400000,
                    weekly_remains_time: 305840499,
                    current_interval_status: 3, current_interval_remaining_percent: 0,
                    current_weekly_status: 3, current_weekly_remaining_percent: 0,
                },
            ],
            base_resp: { status_code: 0, status_msg: 'success' },
        },
    },

    // An allowance and a balance side by side: the booster wallet comes back by
    // topping up, so it reports no recovery time and sorts after the allowances.
    'mock-oauth-kimi-code': {
        provider_uuid: 'mock-oauth-kimi-code',
        provider_name: 'Kimi Code OAuth',
        provider_type: 'kimi_code',
        fetched_at: now.toISOString(),
        expires_at: inOneHour,
        windows: [
            {
                key: 'limit_1',
                type: 'session',
                used: 0,
                limit: 100,
                used_percent: 0,
                resets_at: inOneHour,
                window_minutes: 300,
                unit: 'credits',
                label: '5h limit',
                description: '0 of 100 used',
            },
            {
                key: 'weekly',
                type: 'weekly',
                used: 6,
                limit: 100,
                used_percent: 6,
                resets_at: inSixDays,
                window_minutes: 10080,
                unit: 'credits',
                label: 'Weekly limit',
                description: '6 of 100 used',
            },
            {
                key: 'booster',
                type: 'balance',
                kind: 'resource',
                used: 12.5,
                limit: 50,
                used_percent: 25,
                unit: 'currency',
                label: 'Booster balance',
                description: '37.50 USD remaining',
            },
        ],
        cost: {
            used: 12.5,
            limit: 50,
            currency_code: 'USD',
            label: 'Monthly booster usage',
        },
        account: {
            id: 'com8n51luj0',
            name: 'Kimi Code Mock',
            tier: 'LEVEL_ADVANCED',
        },
        raw_response: {
            user: { userId: 'com8n51luj0', region: 'REGION', membership: { level: 'LEVEL_ADVANCED' }, businessId: '' },
            usage: { limit: '100', used: '6', remaining: '94', resetTime: inSixDays },
            limits: [{
                window: { duration: 300, timeUnit: 'TIME_UNIT_MINUTE' },
                detail: { limit: '100', remaining: '100', resetTime: inOneHour },
            }],
            boosterWallet: {
                balance: { type: 'BOOSTER', amount: '5000000000', amountLeft: '3750000000' },
                monthlyChargeLimitEnabled: true,
                monthlyChargeLimit: { currency: 'USD', priceInCents: '5000' },
                monthlyUsed: { currency: 'USD', priceInCents: 1250 },
            },
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

const mockMainClaudeCodePreferences: Record<string, string> = {
    ANTHROPIC_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_HAIKU_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_SONNET_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_OPUS_MODEL: 'tingly/cc',
    CLAUDE_CODE_SUBAGENT_MODEL: 'tingly/cc',
    CLAUDE_CODE_MAX_OUTPUT_TOKENS: '32000',
    API_TIMEOUT_MS: '3000000',
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: '200000',
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '80',
    DISABLE_TELEMETRY: '1',
    DISABLE_ERROR_REPORTING: '1',
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
}

const mockProfileClaudeConfigs: Record<string, { preferences: Record<string, string>; defaultMode: string; hasOverrides: boolean }> = {}

const mockProfileClaudeConfigData = (profileId: string) => {
    const profile = mockClaudeCodeProfiles.find(item => item.id === profileId)
    const model = profile?.unified ? 'cc' : 'default'
    const basePreferences = {
        ...mockMainClaudeCodePreferences,
        ANTHROPIC_MODEL: model,
        ANTHROPIC_DEFAULT_HAIKU_MODEL: profile?.unified ? model : 'haiku',
        ANTHROPIC_DEFAULT_SONNET_MODEL: profile?.unified ? model : 'sonnet',
        ANTHROPIC_DEFAULT_OPUS_MODEL: profile?.unified ? model : 'opus',
        CLAUDE_CODE_SUBAGENT_MODEL: profile?.unified ? model : 'subagent',
    }
    const stored = mockProfileClaudeConfigs[profileId]
    return {
        basePreferences,
        preferences: stored?.preferences || basePreferences,
        inheritedDefaultMode: 'acceptEdits',
        defaultMode: stored?.defaultMode || 'acceptEdits',
        hasOverrides: stored?.hasOverrides || false,
        // Mirrors the backend contract: derive from current profile metadata;
        // never remember the path in the stored override object.
        settingsPath: `/Users/demo/.tingly-box/claude/${profileId}--${profile?.name}/settings.json`,
        settingsExists: true,
    }
}

// Counter for alternating probe responses
let probeRequestCount = 0

const defaultMockTeamID = '00000000-0000-0000-0000-000000000001'
let mockTeams = [
    { id: defaultMockTeamID, name: 'Default', slug: 'default', enabled: true, is_default: true, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
    { id: '00000000-0000-0000-0000-000000000002', name: 'Platform', slug: 't1', enabled: true, is_default: false, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z' },
]
let mockSharingKeys = [
    { token_id: 'tb-share-design0000000000000000000000000001', user_id: 'user-design', team_id: defaultMockTeamID, display_name: 'Design team', enabled: true, created_at: '2026-06-08T00:00:00Z' },
    { token_id: 'tb-share-platform00000000000000000000000001', user_id: 'user-platform', team_id: '00000000-0000-0000-0000-000000000002', display_name: 'Platform CI', enabled: true, created_at: '2026-07-12T00:00:00Z' },
]

// Shared by the imagegen fixtures below: SVG source -> data URL.
const svgDataUrl = (svg: string): string =>
    `data:image/svg+xml;base64,${btoa(unescape(encodeURIComponent(svg)))}`

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

        const isStream = body?.stream ?? true
        const content = isStream
            ? JSON.stringify([{ choices: [{ delta: { content: 'Hello! Mock streaming probe response.' } }] }])
            : JSON.stringify({ choices: [{ message: { content: 'Hello! Mock nonstream probe response.' } }] })

        return HttpResponse.json({
            success: true,
            data: {
                content,
                latency_ms: Math.floor(Math.random() * 2200) + 400,
                request_url: 'http://localhost:12222/tingly/openai/chat/completions',
                stream: isStream,
                usage: {
                    input_tokens: 21,
                    output_tokens: 14,
                    cache_read_tokens: 0,
                },
                selected_provider: 'Anthropic',
                selected_model: 'claude-opus-4-8',
                routing_source: 'load_balancer',
                upstream_api: 'anthropic_message',
                upstream_url: 'https://api.anthropic.com/v1/messages',
                matched_rule_desc: 'Route gpt-5.6-sol to Anthropic claude-opus-4-8',
                applied_flags: '',
            },
        })
    }),

    // Probe cURL — mirrors the request config so the cURL section previews.
    http.post('/api/v2/probe/curl', async ({ request }) => {
        const body = await request.json() as any
        const isStream = body?.stream ?? false
        const protocol = body?.protocol || 'openai_chat'
        const throughTB = !body?.direct
        const url =
            protocol === 'anthropic_v1'
                ? 'http://localhost:12222/tingly/anthropic/v1/messages'
                : protocol === 'openai_responses'
                  ? 'http://localhost:12222/tingly/openai/responses'
                  : 'http://localhost:12222/tingly/openai/chat/completions'
        const headers: Record<string, string> =
            protocol === 'anthropic_v1'
                ? { 'Content-Type': 'application/json', 'x-api-key': '$TB_API_KEY', 'anthropic-version': '2023-06-01' }
                : { 'Content-Type': 'application/json', Authorization: 'Bearer $TB_API_KEY' }
        const headersOut = throughTB
            ? headers
            : Object.fromEntries(Object.entries(headers).map(([k, v]) => [k, v.replace('$TB_API_KEY', '$UPSTREAM_API_KEY')]))
        const visionDataURL = 'data:image/png;base64,' + 'iVBORw0KGgoAAAANSUhEUgAAAQAAAAEACAIAAADTED8xAAAB+0lEQVR42u3TQQkAAAjAwPUvrX8reHAJBmsK3pIAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgAJMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAyAASTAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADADHAllMDvLkz2XNAAAAAElFTkSuQmCC'
        const messages =
            body?.vision === 'user'
                ? [{ role: 'user', content: [{ type: 'text', text: 'What color is this image? Answer with one word.' }, { type: 'image_url', image_url: { url: visionDataURL } }] }]
                : body?.vision === 'tool'
                  ? [
                        { role: 'user', content: 'Analyze the image using the capture tool.' },
                        { role: 'assistant', content: '', tool_calls: [{ id: 'call_vision_1', type: 'function', function: { name: 'vision_capture', arguments: '{}' } }] },
                        { role: 'tool', tool_call_id: 'call_vision_1', content: [{ type: 'text', text: 'Image captured.' }, { type: 'image_url', image_url: { url: visionDataURL } }] },
                    ]
                  : [{ role: 'user', content: body?.message || 'Hello, this is a test message.' }]
        const mockBody = JSON.stringify({ model: body?.model || 'mock-model', stream: isStream, messages }, null, 2)
        const headerArgs = Object.entries(headersOut).map(([k, v]) => `  -H '${k}: ${v}'`).join(' \\\n')
        const command = `curl${isStream ? ' -N' : ''} \\\n${headerArgs} \\\n  -d '${mockBody}' \\\n  ${url}`
        return HttpResponse.json({
            success: true,
            data: {
                command,
                method: 'POST',
                url,
                headers: headersOut,
                body: mockBody,
                key_env_var: throughTB ? '$TB_API_KEY' : '$UPSTREAM_API_KEY',
            },
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
            data: getMockProviders(),
        })
    }),

    http.get('/api/v2/providers/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        const provider = getMockProviders().find((provider) => provider.uuid === uuid)
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
        mockStandardProviderCatalog.push({ provider: newProvider, models: body.models ?? [] })
        return HttpResponse.json({ success: true, data: newProvider })
    }),

    http.put('/api/v2/providers/:uuid', async ({ params, request }) => {
        const { uuid } = params as { uuid: string }
        const body = await request.json() as any
        const catalogEntry = [...mockStandardProviderCatalog, ...mockOAuthProviderCatalog]
            .find(({ provider }) => provider.uuid === uuid)
        const virtualProvider = mockVirtualProviders.find((provider) => provider.uuid === uuid)
        const provider = catalogEntry?.provider ?? virtualProvider
        if (!provider) return HttpResponse.json({ success: false, error: 'Not found' }, { status: 404 })

        Object.assign(provider, body, { uuid })
        return HttpResponse.json({ success: true, data: provider })
    }),

    http.post('/api/v2/providers/:uuid/toggle', ({ params }) => {
        const { uuid } = params as { uuid: string }
        const provider = getMockProviders().find((provider) => provider.uuid === uuid)
        if (!provider) return HttpResponse.json({ success: false, error: 'Not found' }, { status: 404 })

        provider.enabled = !provider.enabled
        return HttpResponse.json({ success: true, data: provider })
    }),

    http.delete('/api/v2/providers/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        const standardIndex = mockStandardProviderCatalog.findIndex(({ provider }) => provider.uuid === uuid)
        if (standardIndex >= 0) {
            mockStandardProviderCatalog.splice(standardIndex, 1)
            return HttpResponse.json({ success: true })
        }

        const oauthIndex = mockOAuthProviderCatalog.findIndex(({ provider }) => provider.uuid === uuid)
        if (oauthIndex >= 0) {
            mockOAuthProviderCatalog.splice(oauthIndex, 1)
            return HttpResponse.json({ success: true })
        }

        const virtualIndex = mockVirtualProviders.findIndex((provider) => provider.uuid === uuid)
        if (virtualIndex >= 0) {
            mockVirtualProviders.splice(virtualIndex, 1)
            return HttpResponse.json({ success: true })
        }

        return HttpResponse.json({ success: false, error: 'Not found' }, { status: 404 })
    }),

    http.get('/api/v2/provider-models/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        if (!getMockProviders().some((provider) => provider.uuid === uuid)) {
            return HttpResponse.json({ success: false, error: 'Provider not found' }, { status: 404 })
        }
        return HttpResponse.json({ success: true, data: { models: getMockProviderModels(uuid) } })
    }),

    http.post('/api/v2/provider-models/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        if (!getMockProviders().some((provider) => provider.uuid === uuid)) {
            return HttpResponse.json({ success: false, error: 'Provider not found' }, { status: 404 })
        }
        return HttpResponse.json({ success: true, data: { models: getMockProviderModels(uuid) } })
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

    // Shape matches the Go handler: GetQuota / RefreshProvider answer with the
    // usage record itself, not a {success, data} envelope (only /batch wraps).
    // While these wrapped it, the hook read no provider_uuid and treated every
    // single-provider read as empty, so failure states never rendered in mock.
    http.get('/api/v1/provider-quota/:uuid', ({ params }) => {
        const { uuid } = params as { uuid: string }
        if (mockQuotas[uuid]) {
            return HttpResponse.json({ ...mockQuotas[uuid], fetched_at: new Date().toISOString() })
        }
        return HttpResponse.json({ error: 'quota not found for provider' }, { status: 404 })
    }),

    http.post('/api/v1/provider-quota/:uuid/refresh', async ({ params }) => {
        const { uuid } = params as { uuid: string }
        // Simulate a short delay
        await new Promise(r => setTimeout(r, 800))
        if (mockQuotas[uuid]) {
            return HttpResponse.json({ ...mockQuotas[uuid], fetched_at: new Date().toISOString() })
        }
        return HttpResponse.json({ error: 'quota not found for provider' }, { status: 404 })
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

        const rules = scenario ? getMockRulesForScenario(scenario) : []
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

    // Experimental feature flags (guardrails, mcp, skill_user, skill_ide) —
    // all off by default in mock mode, mirroring production.
    http.get('/api/v1/scenario/:scenario/flag/:flag', ({ params }) => {
        const { scenario, flag } = params as { scenario: string; flag: string }
        return HttpResponse.json({ success: true, data: { scenario, flag, value: false } })
    }),

    http.put('/api/v1/scenario/:scenario/flag/:flag', async ({ params, request }) => {
        const { scenario, flag } = params as { scenario: string; flag: string }
        const body = await request.json() as { value?: boolean }
        return HttpResponse.json({ success: true, data: { scenario, flag, value: body.value ?? true } })
    }),

    http.post('/api/v1/rule', async ({ request }) => {
        const body = await request.json() as any
        const scenario = body.scenario?.trim()
        if (!scenario) {
            return HttpResponse.json({ success: false, error: 'Scenario is required' }, { status: 400 })
        }

        const rule = {
            ...body,
            uuid: body.uuid || `mock-rule-${Date.now()}`,
            scenario,
        }
        const errors = getMockRuleErrors(scenario, rule)
        if (errors.length > 0) {
            return HttpResponse.json({ success: false, error: errors.join('; ') }, { status: 400 })
        }

        if (!mockV1Rules[scenario]) mockV1Rules[scenario] = []
        mockV1Rules[scenario].push(rule)
        return HttpResponse.json({ success: true, data: rule })
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
        for (const [scenario, rules] of Object.entries(mockV1Rules)) {
            const idx = rules.findIndex((r) => r.uuid === uuid)
            if (idx >= 0) {
                const rule = { ...rules[idx], ...body, uuid, scenario }
                const errors = getMockRuleErrors(scenario, rule)
                if (errors.length > 0) {
                    return HttpResponse.json({ success: false, error: errors.join('; ') }, { status: 400 })
                }
                rules[idx] = rule
                return HttpResponse.json({ success: true, data: rule })
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
                { key: 'extra_headers', label: 'Custom Headers', description: 'Append custom HTTP headers to the outbound upstream request for this rule. API-key providers only.', type: 'headers', category: 'request' },
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
                groups: mockGuardrailsGroups,
                policies: [
                    {
                        id: 'policy-001',
                        name: 'Block Sensitive Data',
                        kind: 'content',
                        enabled: true,
                        groups: ['default'],
                        actions: ['block'],
                        patterns: ['credit_card', 'ssn', 'password'],
                        description: 'Prevents sensitive data from being transmitted',
                    },
                    {
                        id: 'policy-002',
                        name: 'Restrict File Access',
                        kind: 'resource_access',
                        enabled: true,
                        groups: ['default'],
                        actions: ['block'],
                        patterns: ['/etc/*', '~/.ssh/*'],
                        description: 'Blocks access to sensitive filesystem paths',
                    },
                    {
                        id: 'policy-003',
                        name: 'Prompt Injection Shield',
                        kind: 'content',
                        enabled: true,
                        groups: ['default'],
                        actions: ['block', 'log'],
                        patterns: ['ignore previous', 'system prompt'],
                        description: 'Detects and blocks prompt injection attempts',
                    },
                    {
                        id: 'policy-004',
                        name: 'Guard Destructive Commands',
                        kind: 'command_execution',
                        enabled: false,
                        groups: ['default'],
                        actions: ['warn'],
                        patterns: ['rm -rf', 'sudo'],
                        description: 'Flags potentially destructive shell commands for review',
                    },
                ],
            },
            imports: [
                { path: '/etc/tingly/guardrails/default.yml', name: 'Default Policy', policy_count: 3 },
                { path: '/etc/tingly/guardrails/enterprise.yml', name: 'Enterprise Rules', policy_count: 8 },
            ],
        })
    }),

    http.post('/api/v1/guardrails/group', async ({ request }) => {
        const group = await request.json() as { id?: string; name?: string; enabled?: boolean; severity?: string }
        if (!group.id) {
            return HttpResponse.json({ success: false, error: 'Group ID is required' }, { status: 400 })
        }
        const nextGroup = {
            id: group.id,
            name: group.name || group.id,
            enabled: group.enabled ?? true,
            severity: group.severity || 'medium',
        }
        const existingIndex = mockGuardrailsGroups.findIndex((item) => item.id === group.id)
        if (existingIndex >= 0) {
            mockGuardrailsGroups[existingIndex] = nextGroup
        } else {
            mockGuardrailsGroups.push(nextGroup)
        }
        return HttpResponse.json({ success: true, group: nextGroup })
    }),

    http.put('/api/v1/guardrails/group/:id', async ({ params, request }) => {
        const group = await request.json() as { id?: string; name?: string; enabled?: boolean; severity?: string }
        const groupID = String(params.id)
        const existingIndex = mockGuardrailsGroups.findIndex((item) => item.id === groupID)
        if (existingIndex < 0) {
            return HttpResponse.json({ success: false, error: 'Group not found' }, { status: 404 })
        }
        const nextGroup = {
            ...mockGuardrailsGroups[existingIndex],
            ...group,
            id: group.id || groupID,
        }
        mockGuardrailsGroups[existingIndex] = nextGroup
        return HttpResponse.json({ success: true, group: nextGroup })
    }),

    http.delete('/api/v1/guardrails/group/:id', ({ params }) => {
        const groupID = String(params.id)
        const existingIndex = mockGuardrailsGroups.findIndex((item) => item.id === groupID)
        if (existingIndex < 0) {
            return HttpResponse.json({ success: false, error: 'Group not found' }, { status: 404 })
        }
        mockGuardrailsGroups.splice(existingIndex, 1)
        return HttpResponse.json({ success: true })
    }),

    http.get('/api/v1/guardrails/history', () => {
        const now = Date.now()
        return HttpResponse.json({
            success: true,
            data: [
                { time: new Date(now - 2 * 60 * 1000).toISOString(), verdict: 'block', phase: 'request', scenario: 'claude_code', alias_hits: ['credit_card'], credential_names: [] },
                { time: new Date(now - 8 * 60 * 1000).toISOString(), verdict: 'allow', phase: 'response', scenario: 'openai', alias_hits: [], credential_names: [] },
                { time: new Date(now - 15 * 60 * 1000).toISOString(), verdict: 'block', phase: 'request', scenario: 'openai', alias_hits: ['prompt_injection'], credential_names: ['OpenAI'] },
                { time: new Date(now - 32 * 60 * 1000).toISOString(), verdict: 'mask', phase: 'request', scenario: 'claude_code', alias_hits: ['ssn'], credential_names: [] },
                { time: new Date(now - 45 * 60 * 1000).toISOString(), verdict: 'review', phase: 'response', scenario: 'anthropic', alias_hits: ['ssn'], credential_names: [] },
                { time: new Date(now - 60 * 60 * 1000).toISOString(), verdict: 'allow', phase: 'request', scenario: 'custom', alias_hits: [], credential_names: [] },
            ],
        })
    }),

    http.get('/api/v1/guardrails/builtins', () => {
        return HttpResponse.json({ success: true, data: [] })
    }),

    http.get('/api/v1/guardrails/credentials', () => {
        return HttpResponse.json({ success: true, data: [] })
    }),

    // ============================================
    // MCP (Model Context Protocol) runtime config
    // ============================================
    http.get('/api/v1/mcp/config', () => {
        return HttpResponse.json({
            success: true,
            config: {
                sources: [
                    { id: 'webtools', enabled: true, transport: 'advisor', visibility: 'client' },
                    {
                        id: 'filesystem', enabled: true, transport: 'stdio', visibility: 'server',
                        command: 'npx', args: ['-y', '@modelcontextprotocol/server-filesystem', '/home/user/projects'],
                        tools: ['read_file', 'write_file', 'list_directory'],
                    },
                    {
                        id: 'github', enabled: true, transport: 'http', visibility: 'client',
                        endpoint: 'https://api.githubcopilot.com/mcp',
                        tools: ['search_repositories', 'create_issue', 'get_pull_request'],
                    },
                    {
                        id: 'postgres', enabled: false, transport: 'stdio', visibility: 'server',
                        command: 'npx', args: ['-y', '@modelcontextprotocol/server-postgres'],
                        tools: ['query'],
                    },
                ],
                request_timeout: 30000,
                strip_disabled_mcp_tools: true,
            },
        })
    }),

    http.put('/api/v1/mcp/config', async ({ request }) => {
        const body = await request.json() as any
        return HttpResponse.json({ success: true, config: body })
    }),

    http.get('/api/v1/mcp/clients', () => {
        return HttpResponse.json({ success: true, data: [] })
    }),

    http.post('*/tingly/imagegen/v1/images/generations', async ({ request }) => {
        const body = (await request.json()) as any
        const n = Math.max(1, Math.min(10, Number(body?.n) || 1))
        const size = typeof body?.size === 'string' ? body.size : '1024x1024'
        const [w, h] = size.split('x').map(Number)
        const palette = ['#7c3aed', '#06b6d4', '#10b981', '#f59e0b', '#ef4444', '#3b82f6']
        const promptText = String(body?.prompt ?? '').slice(0, 80).replace(/[<>&"]/g, '')

        // A prompt that asks for an NxM grid gets a real grid back, so the
        // playground's slicer is exercisable against the mock backend.
        const sheet = /(\d+)\s*[x×]\s*(\d+)\s+grid/i.exec(promptText)

        const makeSheetSvgDataUrl = (cols: number, rows: number, offset: number): string => {
            const cellW = w / cols
            const cellH = h / rows
            const radius = Math.min(cellW, cellH) * 0.36
            let body = ''
            for (let row = 0; row < rows; row += 1) {
                for (let col = 0; col < cols; col += 1) {
                    const idx = row * cols + col
                    const cx = cellW * (col + 0.5)
                    const cy = cellH * (row + 0.5)
                    body += `<circle cx="${cx}" cy="${cy}" r="${radius}" fill="${palette[(idx + offset) % palette.length]}" stroke="white" stroke-width="${radius * 0.16}"/>`
                        + `<text x="${cx}" y="${cy + radius * 0.2}" font-family="sans-serif" font-size="${radius * 0.7}" fill="white" text-anchor="middle" font-weight="700">${idx + 1}</text>`
                }
            }
            return svgDataUrl(`<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">${body}</svg>`)
        }

        const makeSvgDataUrl = (idx: number): string => {
            if (sheet) return makeSheetSvgDataUrl(Number(sheet[1]), Number(sheet[2]), idx)
            const bg = palette[idx % palette.length]
            const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">`
                + `<rect width="100%" height="100%" fill="${bg}"/>`
                + `<text x="50%" y="45%" font-family="sans-serif" font-size="${Math.round(w / 18)}" fill="white" text-anchor="middle" font-weight="700">MOCK #${idx + 1}</text>`
                + `<text x="50%" y="56%" font-family="sans-serif" font-size="${Math.round(w / 36)}" fill="white" fill-opacity="0.85" text-anchor="middle">${promptText}</text>`
                + `<text x="50%" y="95%" font-family="monospace" font-size="${Math.round(w / 50)}" fill="white" fill-opacity="0.7" text-anchor="middle">${body?.model ?? ''} · ${size} · q=${body?.quality ?? 'auto'}</text>`
                + `</svg>`
            return svgDataUrl(svg)
        }

        // Simulate a small latency so the loading state is visible
        await new Promise((r) => setTimeout(r, 600))

        return HttpResponse.json({
            created: Math.floor(Date.now() / 1000),
            data: Array.from({ length: n }, (_, i) => ({ url: makeSvgDataUrl(i) })),
        })
    }),

    // Mirrors the generations mock above but reads the standard OpenAI
    // multipart /images/edits wire format (the openai SDK's images.edit()
    // always multipart-encodes, regardless of upstream vendor).
    http.post('*/tingly/imagegen/v1/images/edits', async ({ request }) => {
        const form = await request.formData()
        const n = Math.max(1, Math.min(10, Number(form.get('n')) || 1))
        const sizeValue = form.get('size')
        const size = typeof sizeValue === 'string' && sizeValue ? sizeValue : '1024x1024'
        const [w, h] = size.split('x').map(Number)
        const palette = ['#7c3aed', '#06b6d4', '#10b981', '#f59e0b', '#ef4444', '#3b82f6']
        const promptText = String(form.get('prompt') ?? '').slice(0, 80).replace(/[<>&"]/g, '')
        const model = form.get('model') ?? ''
        const quality = form.get('quality') || 'auto'

        const makeSvgDataUrl = (idx: number): string => {
            const bg = palette[(idx + 2) % palette.length]
            const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">`
                + `<rect width="100%" height="100%" fill="${bg}"/>`
                + `<text x="50%" y="45%" font-family="sans-serif" font-size="${Math.round(w / 18)}" fill="white" text-anchor="middle" font-weight="700">EDITED #${idx + 1}</text>`
                + `<text x="50%" y="56%" font-family="sans-serif" font-size="${Math.round(w / 36)}" fill="white" fill-opacity="0.85" text-anchor="middle">${promptText}</text>`
                + `<text x="50%" y="95%" font-family="monospace" font-size="${Math.round(w / 50)}" fill="white" fill-opacity="0.7" text-anchor="middle">${model} · ${size} · q=${quality}</text>`
                + `</svg>`
            return svgDataUrl(svg)
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
    http.get('/api/v1/teams', () => HttpResponse.json({teams: mockTeams})),

    http.post('/api/v1/teams', async ({request}) => {
        const body = await request.json() as any
        const used = new Set(mockTeams.map((team) => /^t(\d+)$/.exec(team.slug)?.[1]).filter(Boolean).map(Number))
        let number = 1
        while (used.has(number)) number++
        const team = {id: crypto.randomUUID(), name: body.name, slug: `t${number}`, enabled: true, is_default: false, created_at: new Date().toISOString(), updated_at: new Date().toISOString()}
        mockTeams = [...mockTeams, team]
        return HttpResponse.json(team, {status: 201})
    }),

    http.put('/api/v1/teams/:teamId', async ({params, request}) => {
        const body = await request.json() as any
        mockTeams = mockTeams.map((team) => team.id === params.teamId ? {...team, ...body, updated_at: new Date().toISOString()} : team)
        return HttpResponse.json(mockTeams.find((team) => team.id === params.teamId))
    }),

    http.put('/api/v1/teams/:teamId/:action', ({params}) => {
        const enabled = params.action === 'enable'
        mockTeams = mockTeams.map((team) => team.id === params.teamId ? {...team, enabled} : team)
        return HttpResponse.json(mockTeams.find((team) => team.id === params.teamId))
    }),

    http.delete('/api/v1/teams/:teamId', ({params}) => {
        mockTeams = mockTeams.filter((team) => team.id !== params.teamId)
        return new HttpResponse(null, {status: 204})
    }),

    http.get('/api/v1/tokens', ({request}) => {
        const teamID = new URL(request.url).searchParams.get('team_id')
        const tokens = teamID ? mockSharingKeys.filter((token) => token.team_id === teamID) : mockSharingKeys
        return HttpResponse.json({
            tokens,
            total: tokens.length,
        })
    }),

    http.post('/api/v1/tokens', async ({request}) => {
        const body = await request.json() as any
        const token = {token_id: `tb-share-${crypto.randomUUID().replaceAll('-', '')}`, user_id: crypto.randomUUID(), team_id: body.team_id || defaultMockTeamID, display_name: body.display_name, enabled: true, created_at: new Date().toISOString()}
        mockSharingKeys = [token, ...mockSharingKeys]
        return HttpResponse.json({...token, token: token.token_id}, {status: 201})
    }),

    http.put('/api/v1/tokens/:tokenId/team', async ({params, request}) => {
        const body = await request.json() as any
        mockSharingKeys = mockSharingKeys.map((token) => token.token_id === params.tokenId ? {...token, team_id: body.team_id} : token)
        return HttpResponse.json(mockSharingKeys.find((token) => token.token_id === params.tokenId))
    }),

    http.put('/api/v1/tokens/:tokenId/:action', ({params}) => {
        const enabled = params.action === 'enable'
        mockSharingKeys = mockSharingKeys.map((token) => token.token_id === params.tokenId ? {...token, enabled} : token)
        return new HttpResponse(null, {status: 204})
    }),

    http.delete('/api/v1/tokens/:tokenId', ({params}) => {
        mockSharingKeys = mockSharingKeys.filter((token) => token.token_id !== params.tokenId)
        return new HttpResponse(null, {status: 204})
    }),

    http.get('/api/v1/usage/stats', ({ request }) => {
        const url = new URL(request.url)
        const groupBy = url.searchParams.get('group_by')
        const userID = url.searchParams.get('user_id')
        const modelParam = url.searchParams.get('model')
        const providerParam = url.searchParams.get('provider')
        const userUsage = [
            { key: 'admin', user_id: 'admin', request_count: 2310, total_tokens: 11200000, total_input_tokens: 6180000, total_output_tokens: 4220000, cache_read_tokens: 800000, cache_write_tokens: 96000, error_count: 9, error_rate: 0.0039 },
            { key: 'user-platform', user_id: 'user-platform', request_count: 1884, total_tokens: 8460000, total_input_tokens: 4620000, total_output_tokens: 3140000, cache_read_tokens: 700000, cache_write_tokens: 84000, error_count: 13, error_rate: 0.0069 },
            { key: 'user-design', user_id: 'user-design', request_count: 1256, total_tokens: 5790000, total_input_tokens: 3110000, total_output_tokens: 2260000, cache_read_tokens: 420000, cache_write_tokens: 50400, error_count: 4, error_rate: 0.0032 },
            { key: 'user-mobile', user_id: 'user-mobile', request_count: 934, total_tokens: 3680000, total_input_tokens: 2180000, total_output_tokens: 1420000, cache_read_tokens: 80000, cache_write_tokens: 9600, error_count: 19, error_rate: 0.0203 },
            { key: 'user-support', user_id: 'user-support', request_count: 226, total_tokens: 930000, total_input_tokens: 540000, total_output_tokens: 370000, cache_read_tokens: 20000, cache_write_tokens: 2400, error_count: 1, error_rate: 0.0044 },
        ]
        if (groupBy === 'user') {
            const scopeKey = modelParam ? `model:${modelParam}` : providerParam ? `provider:${providerParam}` : ''
            if (!scopeKey) {
                return HttpResponse.json({ success: true, data: userUsage })
            }
            // "Which accounts used this model/provider" — deterministic per
            // (scope, user) weight so different models/providers return
            // different, sortable account mixes instead of always the same 5
            // rows scaled uniformly. Accounts below the weight floor are
            // dropped, so not every model/provider is used by every account.
            const hash = (input: string) => {
                let h = 0
                for (let i = 0; i < input.length; i++) h = (h * 31 + input.charCodeAt(i)) >>> 0
                return h
            }
            const scaledByScope = userUsage
                .map((account) => {
                    const weight = (hash(`${scopeKey}:${account.user_id}`) % 100) / 100
                    const factor = weight > 0.15 ? weight : 0
                    const scale = (value: number) => Math.round(value * factor)
                    return {
                        ...account,
                        request_count: scale(account.request_count),
                        total_tokens: scale(account.total_tokens),
                        total_input_tokens: scale(account.total_input_tokens),
                        total_output_tokens: scale(account.total_output_tokens),
                        cache_read_tokens: scale(account.cache_read_tokens),
                        cache_write_tokens: scale(account.cache_write_tokens),
                        error_count: scale(account.error_count),
                    }
                })
                .filter((account) => account.request_count > 0)
            return HttpResponse.json({ success: true, data: scaledByScope })
        }

        // Unscaled per-model base figures. Both the model list below (scaled
        // per-user when userID is set) and the provider list derive from this
        // single array, so there's one source of truth to keep numbers
        // consistent rather than a hand-maintained sum living separately.
        const modelUsageBase = [
            {
                key: 'anthropic/claude-sonnet-5',
                provider_uuid: 'mock-provider-anthropic',
                provider_name: 'Anthropic',
                model: 'claude-sonnet-5',
                scenario: 'claude_code',
                request_count: 1842,
                total_tokens: 25920000,
                total_input_tokens: 1140000,
                total_output_tokens: 920000,
                cache_read_tokens: 23860000,
                cache_write_tokens: 2863200,
                avg_latency_ms: 1240,
                error_count: 12,
                error_rate: 0.65,
                streamed_count: 1800,
            },
            {
                key: 'anthropic/claude-opus-4-8',
                provider_uuid: 'mock-provider-anthropic',
                provider_name: 'Anthropic',
                model: 'claude-opus-4-8',
                scenario: 'claude_code',
                request_count: 420,
                total_tokens: 8180000,
                total_input_tokens: 620000,
                total_output_tokens: 380000,
                cache_read_tokens: 7180000,
                cache_write_tokens: 861600,
                avg_latency_ms: 2100,
                error_count: 3,
                error_rate: 0.71,
                streamed_count: 415,
            },
            {
                key: 'openai/gpt-5.6-sol',
                provider_uuid: 'mock-provider-openai',
                provider_name: 'OpenAI',
                model: 'gpt-5.6-sol',
                scenario: 'openai',
                request_count: 938,
                total_tokens: 6720000,
                total_input_tokens: 890000,
                total_output_tokens: 520000,
                cache_read_tokens: 5310000,
                cache_write_tokens: 637200,
                avg_latency_ms: 980,
                error_count: 8,
                error_rate: 0.85,
                streamed_count: 920,
            },
            {
                key: 'openai/gpt-5.6-luna',
                provider_uuid: 'mock-provider-openai',
                provider_name: 'OpenAI',
                model: 'gpt-5.6-luna',
                scenario: 'openai',
                request_count: 2150,
                total_tokens: 4310000,
                total_input_tokens: 390000,
                total_output_tokens: 410000,
                cache_read_tokens: 3510000,
                cache_write_tokens: 421200,
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
                scenario: 'custom',
                request_count: 312,
                total_tokens: 1350000,
                total_input_tokens: 1050000,
                total_output_tokens: 120000,
                cache_read_tokens: 180000,
                cache_write_tokens: 21600,
                avg_latency_ms: 3200,
                error_count: 2,
                error_rate: 0.64,
                streamed_count: 308,
            },
        ]

        if (groupBy === 'provider') {
            const providerNames = new Map(modelUsageBase.map((m) => [m.provider_uuid, m.provider_name]))
            const totalsByProvider = new Map<string, { request_count: number; total_tokens: number; total_input_tokens: number; total_output_tokens: number; cache_read_tokens: number; cache_write_tokens: number; error_count: number }>()
            for (const m of modelUsageBase) {
                const totals = totalsByProvider.get(m.provider_uuid) || { request_count: 0, total_tokens: 0, total_input_tokens: 0, total_output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, error_count: 0 }
                totals.request_count += m.request_count
                totals.total_tokens += m.total_tokens
                totals.total_input_tokens += m.total_input_tokens
                totals.total_output_tokens += m.total_output_tokens
                totals.cache_read_tokens += m.cache_read_tokens
                totals.cache_write_tokens += m.cache_write_tokens
                totals.error_count += m.error_count
                totalsByProvider.set(m.provider_uuid, totals)
            }
            return HttpResponse.json({
                success: true,
                data: Array.from(totalsByProvider, ([providerUuid, totals]) => ({
                    key: providerUuid,
                    provider_uuid: providerUuid,
                    provider_name: providerNames.get(providerUuid) || providerUuid,
                    ...totals,
                    error_rate: totals.request_count > 0 ? totals.error_count / totals.request_count : 0,
                })),
            })
        }

        const userScale = userID
            ? (userUsage.find((item) => item.user_id === userID)?.total_tokens || 0) / 46480000
            : 1
        const scale = (value: number) => Math.round(value * userScale)
        return HttpResponse.json({
            success: true,
            data: modelUsageBase
                .map((model) => ({
                    ...model,
                    request_count: scale(model.request_count),
                    total_tokens: scale(model.total_tokens),
                    total_input_tokens: scale(model.total_input_tokens),
                    total_output_tokens: scale(model.total_output_tokens),
                    cache_read_tokens: scale(model.cache_read_tokens),
                    cache_write_tokens: scale(model.cache_write_tokens),
                    error_count: scale(model.error_count),
                }))
                .filter((stat) => !userID || stat.total_tokens > 0),
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
            let cacheNoise = 0
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
                const output = Math.round(base * (0.025 + Math.random() * 0.055))
                const prompt = Math.max(0, Math.round(base) - output)
                // A continuous session warms once, then stays in a narrow
                // high-hit band instead of repeatedly dropping to cold-cache.
                const warmupPoints = Math.max(2, Math.min(8, Math.round(count * 0.08)))
                const warmupProgress = Math.min(1, i / warmupPoints)
                const easedWarmup = 1 - Math.pow(1 - warmupProgress, 2)
                cacheNoise = cacheNoise * 0.82 + (Math.random() - 0.5) * 0.022
                const steadyCacheRatio = Math.min(0.985, Math.max(0.9, 0.945 + cacheNoise))
                const cacheRatio = 0.15 + (steadyCacheRatio - 0.15) * easedWarmup
                const cache = Math.round(prompt * cacheRatio)
                const input = prompt - cache
                return {
                    timestamp: ts.toISOString(),
                    request_count: Math.round((180 + Math.sin(i * 0.4) * 70 + Math.random() * 50) * activityFactor * trend),
                    input_tokens: input,
                    output_tokens: output,
                    cache_read_tokens: cache,
                    // Writes are a slice of the uncached input, never on top of it.
                    cache_write_tokens: Math.round(input * 0.12),
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
            { provider_name: 'Anthropic', model: 'claude-sonnet-5', scenario: 'claude_code', streamed: true, cacheHitRatio: 0.97 },
            { provider_name: 'Anthropic', model: 'claude-opus-4-8',   scenario: 'claude_code', streamed: true, cacheHitRatio: 0.94 },
            { provider_name: 'OpenAI',    model: 'gpt-5.6-sol',            scenario: 'openai',      streamed: true, cacheHitRatio: 0.91 },
            { provider_name: 'OpenAI',    model: 'gpt-5.6-luna',       scenario: 'openai',      streamed: false, cacheHitRatio: 0.90 },
            { provider_name: 'OpenRouter', model: 'deepseek/deepseek-v4-pro', scenario: 'custom', streamed: true, cacheHitRatio: 0.18 },
        ]

        const now = Date.now()
        const dayStart = new Date()
        dayStart.setHours(0, 0, 0, 0)

        const total = 120
        const all = Array.from({ length: total }, (_, i) => {
            const m = models[i % models.length]
            const isError = i % 20 === 0
            const prompt = Math.round(6000 + Math.random() * 42000)
            const isColdStart = i % 23 === 0
            const isPartialHit = !isColdStart && i % 13 === 4
            const cacheRatio = isColdStart
                ? Math.random() * 0.12
                : isPartialHit
                    ? 0.45 + Math.random() * 0.3
                    : Math.min(0.995, Math.max(0, m.cacheHitRatio + (Math.random() - 0.5) * 0.05))
            const cache = Math.round(prompt * cacheRatio)
            const input = prompt - cache
            const output = isError
                ? Math.round(Math.random() * 24)
                : Math.round(80 + Math.random() * Math.min(1200, prompt * 0.045))
            const latency = Math.round(
                m.model.includes('opus') ? 1500 + Math.random() * 3000
                : m.model.includes('mini') ? 200 + Math.random() * 600
                : 600 + Math.random() * 1800
            )
            const ttft = (m.streamed && !isError) ? Math.round(latency * (0.05 + Math.random() * 0.15)) : 0
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
                cache_read_tokens: cache,
                cache_write_tokens: Math.round(input * 0.12),
                status: isError ? 'error' : 'success',
                error_code: isError ? 'rate_limit_exceeded' : '',
                latency_ms: latency,
                ttft_ms: ttft,
                tokens_per_second: (m.streamed && !isError && output > 1)
                    ? (output - 1) * 1000 / (latency - ttft)
                    : 0,
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

    http.get('/api/v1/usage/performance', () => HttpResponse.json({
        ttft: { sample_count: 842, p10: 118, p50: 386, p90: 1120, p95: 1860, p99: 3480 },
        tps: { sample_count: 817, p10: 31.4, p50: 52.8, p90: 83.6, p95: 94.8, p99: 104.2 },
        completion: { sample_count: 910, p10: 2350, p50: 8400, p90: 22100, p95: 35400, p99: 61200 },
    })),

    // ============================================
    // ImBot Settings API (v1)
    // ============================================
    // Mirrors the platform registry in imbot/platform.go (PlatformConfigs) -
    // field shape (platform/display_name/auth_type/category/fields) must match
    // the real API or BotPlatformSelector's <Select> and BotAuthForm both
    // silently fail to render (key/value lookups use `.platform`).
    http.get('/api/v1/imbot-platforms', () => {
        return HttpResponse.json({
            success: true,
            platforms: [
                {
                    platform: 'telegram', display_name: 'Telegram', auth_type: 'token', category: 'im',
                    fields: [
                        { key: 'token', label: 'Bot Token', placeholder: '123456789:ABCdefGHIjklMNOpqrsTUVwxyz', required: true, secret: true, helperText: 'Get from @BotFather on Telegram' },
                    ],
                },
                {
                    platform: 'slack', display_name: 'Slack', auth_type: 'token', category: 'im',
                    fields: [
                        { key: 'token', label: 'Bot Token', placeholder: 'xoxb-your-token-here', required: true, secret: true, helperText: "Must start with 'xoxb-'. Get from Slack API" },
                    ],
                },
                {
                    platform: 'discord', display_name: 'Discord', auth_type: 'token', category: 'im',
                    fields: [
                        { key: 'token', label: 'Bot Token', placeholder: 'MTIzNDU2Nzg5OABCDEF123456789', required: true, secret: true, helperText: "Must start with 'Bot ' prefix. Get from Discord Developer Portal" },
                    ],
                },
                {
                    platform: 'dingtalk', display_name: 'DingTalk', auth_type: 'oauth', category: 'enterprise',
                    fields: [
                        { key: 'clientId', label: 'App Key', placeholder: 'ding-your-app-key', required: true, secret: true, helperText: 'Also known as AppKey or ClientId' },
                        { key: 'clientSecret', label: 'App Secret', placeholder: 'Your app secret', required: true, secret: true, helperText: 'Also known as AppSecret or ClientSecret' },
                    ],
                },
                {
                    platform: 'feishu', display_name: 'Feishu', auth_type: 'oauth', category: 'enterprise',
                    fields: [
                        { key: 'clientId', label: 'App ID', placeholder: 'cli-your-app-id', required: true, secret: true, helperText: 'Also known as AppID or ClientId' },
                        { key: 'clientSecret', label: 'App Secret', placeholder: 'Your app secret', required: true, secret: true, helperText: 'Also known as AppSecret or ClientSecret' },
                    ],
                },
                {
                    platform: 'lark', display_name: 'Lark', auth_type: 'oauth', category: 'enterprise',
                    fields: [
                        { key: 'clientId', label: 'App ID', placeholder: 'cli-your-app-id', required: true, secret: true, helperText: 'Also known as AppID or ClientId' },
                        { key: 'clientSecret', label: 'App Secret', placeholder: 'Your app secret', required: true, secret: true, helperText: 'Also known as AppSecret or ClientSecret' },
                    ],
                },
                {
                    platform: 'weixin', display_name: 'Weixin', auth_type: 'qr', category: 'enterprise',
                    fields: [],
                },
                {
                    platform: 'wecom', display_name: 'WeCom', auth_type: 'oauth', category: 'enterprise',
                    fields: [
                        { key: 'clientId', label: 'Bot ID', placeholder: 'Your WeCom AI Bot ID', required: true, secret: false, helperText: 'The AI Bot ID from WeCom developer console' },
                        { key: 'clientSecret', label: 'Bot Secret', placeholder: 'Your WeCom AI Bot secret', required: true, secret: true, helperText: 'The AI Bot secret from WeCom developer console' },
                    ],
                },
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
                    // @cc routes through the "ds" Claude Code profile (claude_code:p1)
                    default_agent: 'claude_code:p1',
                    smartguide_provider: 'mock-provider-anthropic',
                    smartguide_model: 'claude-sonnet-5',
                    chat_id_lock: '123456789',
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
                    smartguide_model: 'gpt-5.6-sol',
                    chat_id_lock: 'C0123456789',
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
                    smartguide_model: 'claude-opus-4-8',
                    chat_id_lock: '',
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
                smartguide_model: 'claude-sonnet-5',
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

    // Bot access + interaction APIs. The fixtures intentionally include an
    // authorized Direct Chat, an observed-but-denied Direct Chat, and a Group
    // so the Notify work surface can exercise every primary state.
    http.get('/api/v1/bots/:bot/capabilities', ({ params }) => {
        return HttpResponse.json({capabilities: [
            {bot_uuid: params.bot, capability: 'remote_control', enabled: true, config: {}},
            {bot_uuid: params.bot, capability: 'notify', enabled: true, config: {}},
        ], bot_running: true})
    }),

    http.put('/api/v1/bots/:bot/capabilities/:capability', async ({ params, request }) => {
        const body = await request.json().catch(() => ({})) as any
        return HttpResponse.json({
            capability: {bot_uuid: params.bot, capability: params.capability, enabled: Boolean(body.enabled), config: body.config || {}},
            bot_running: Boolean(body.enabled),
        })
    }),

    http.get('/api/v1/bots/:bot/chats', () => {
        return HttpResponse.json({
            chats: [
                {
                    chat: {id: 'direct-1', bot_uuid: 'mock-bot', platform: 'telegram', external_chat_id: 'telegram:123456789', peer_actor_id: 'actor-1', blocked: false},
                    permissions: [
                        {capability: 'notify', action: 'access', effect: 'allow'},
                        {capability: 'notify', action: 'notify.receive', effect: 'allow'},
                        {capability: 'notify', action: 'notify.reply', effect: 'allow'},
                    ],
                },
                {
                    chat: {id: 'direct-2', bot_uuid: 'mock-bot', platform: 'telegram', external_chat_id: 'telegram:987654321', peer_actor_id: 'actor-2', blocked: false},
                    permissions: [],
                },
            ],
        })
    }),

    http.put('/api/v1/bots/:bot/chats/:chat/permissions/:capability/:action', () => HttpResponse.json({ok: true})),
    http.put('/api/v1/bots/:bot/chats/:chat/blocked', () => HttpResponse.json({ok: true})),
    http.delete('/api/v1/bots/:bot/chats/:chat', () => HttpResponse.json({ok: true})),

    http.get('/api/v1/bots/:bot/groups', () => HttpResponse.json({groups: [
        {id: 'group-1', bot_uuid: 'mock-bot', platform: 'telegram', external_group_id: 'telegram:-10024681012', name: 'Release Crew', blocked: false},
    ]})),
    http.get('/api/v1/bots/:bot/groups/:group', () => HttpResponse.json({
        group: {id: 'group-1', bot_uuid: 'mock-bot', platform: 'telegram', external_group_id: 'telegram:-10024681012', name: 'Release Crew', blocked: false},
        capabilities: {notify: 'allow', remote_control: 'deny'},
        actors: [],
    })),
    http.put('/api/v1/bots/:bot/groups/:group/capabilities/:capability', () => HttpResponse.json({ok: true})),
    http.put('/api/v1/bots/:bot/groups/:group/blocked', () => HttpResponse.json({ok: true})),

    http.post('/api/v1/bots/:bot/notify', async ({ request }) => {
        const body = await request.json().catch(() => null) as any
        if (!body?.target?.kind || !body?.target?.id || !body.body) {
            return HttpResponse.json({ error: 'target and body are required' }, { status: 400 })
        }
        return HttpResponse.json({ ok: true })
    }),

    // Bot interact API — POST /bots/:bot/interact + GET .../interact/:id.
    // Mirrors internal/server/module/notify/bot_api.go. The wait handler
    // answers on the first poll so the confirm probe resolves immediately in
    // mock mode (real backend long-polls).
    http.post('/api/v1/bots/:bot/interact', async ({ params, request }) => {
        const body = await request.json().catch(() => null) as any
        if (!body?.target?.kind || !body?.target?.id || !body.kind || !body.title) {
            return HttpResponse.json({ error: 'target, kind, and title are required' }, { status: 400 })
        }
        const request_id = 'mock-req-' + Math.random().toString(36).slice(2, 10)
        return HttpResponse.json({
            request_id,
            wait_url: `/api/v1/bots/${params.bot}/interact/${request_id}`,
            expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
        }, { status: 202 })
    }),

    http.get('/api/v1/bots/:bot/interact/:request_id', () => {
        // Immediately answered with "allow" so the confirm probe succeeds.
        return HttpResponse.json({ status: 'answered', decision: { selected: 'allow' } })
    }),

    http.put('/api/v1/imbot-settings/:uuid', async ({ params, request }) => {
        const body = await request.json() as any
        return HttpResponse.json({ success: true, uuid: params.uuid, ...body })
    }),

    http.post('/api/v1/imbot-settings', async ({ request }) => {
        const body = await request.json() as any
        const uuid = 'mock-bot-' + Math.random().toString(36).slice(2, 10)
        return HttpResponse.json({ success: true, uuid, setting: { uuid, ...body } })
    }),

    http.delete('/api/v1/imbot-settings/:uuid', ({ params }) => {
        return HttpResponse.json({ success: true, uuid: params.uuid })
    }),

    http.post('/api/v1/imbot-admin/restart/:uuid', ({ params }) => {
        return HttpResponse.json({ success: true, uuid: params.uuid })
    }),

    // --- Weixin QR bind flow (WeixinQRAuth.tsx) ---
    // Poll counters are per-uuid so the mock progresses wait -> scaned ->
    // confirmed over a few polls instead of hanging forever.
    ...(() => {
        const weixinPolls = new Map<string, number>()
        return [
            http.post('/api/v1/imbot-settings/:uuid/weixin/qr-start', ({ params }) => {
                weixinPolls.set(params.uuid as string, 0)
                return HttpResponse.json({
                    success: true,
                    data: { qrcode_id: 'mock-qr-' + params.uuid, qrcode_data: 'weixin://mock-bind/' + params.uuid, expires_in: 120 },
                })
            }),
            http.get('/api/v1/imbot-settings/:uuid/weixin/qr-status', ({ params }) => {
                const uuid = params.uuid as string
                const count = (weixinPolls.get(uuid) ?? 0) + 1
                weixinPolls.set(uuid, count)
                if (count < 2) return HttpResponse.json({ success: true, data: { status: 'wait' } })
                if (count < 3) return HttpResponse.json({ success: true, data: { status: 'scaned' } })
                return HttpResponse.json({ success: true, data: { status: 'confirmed', bot_uuid: uuid.startsWith('temp-') ? 'mock-bot-' + uuid.slice(5, 13) : uuid } })
            }),
            http.post('/api/v1/imbot-settings/:uuid/weixin/qr-cancel', ({ params }) => {
                weixinPolls.delete(params.uuid as string)
                return HttpResponse.json({ status: 'cancelled' })
            }),
        ]
    })(),

    // --- Feishu/Lark one-click registration flow (FeishuQRAuth.tsx) ---
    ...(() => {
        const feishuPolls = new Map<string, number>()
        return [
            http.post('/api/v1/imbot-settings/:uuid/feishu/qr-start', ({ params }) => {
                feishuPolls.set(params.uuid as string, 0)
                return HttpResponse.json({
                    success: true,
                    data: { qr_url: 'https://open.feishu.cn/mock-launcher/' + params.uuid, expires_in: 120 },
                })
            }),
            http.get('/api/v1/imbot-settings/:uuid/feishu/qr-status', ({ params }) => {
                const uuid = params.uuid as string
                const count = (feishuPolls.get(uuid) ?? 0) + 1
                feishuPolls.set(uuid, count)
                if (count < 3) return HttpResponse.json({ success: true, data: { status: 'pending' } })
                return HttpResponse.json({ success: true, data: { status: 'confirmed', bot_uuid: uuid.startsWith('temp-') ? 'mock-bot-' + uuid.slice(5, 13) : uuid, tenant_brand: 'Feishu' } })
            }),
            http.post('/api/v1/imbot-settings/:uuid/feishu/qr-cancel', ({ params }) => {
                feishuPolls.delete(params.uuid as string)
                return HttpResponse.json({ status: 'cancelled' })
            }),
        ]
    })(),

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
        const base = MOCK_TRACE_BASE
        return HttpResponse.json({
            ...summary,
            time: new Date(base).toISOString(),
            events: (mockRequestEvents[id] || []).map((e) => ({
                ...e,
                time: new Date(base + e.offset).toISOString(),
            })),
        })
    }),

    http.get('/api/v1/traces/:traceId', ({ params }) => {
        const traceId = String(params.traceId)
        const spans = mockTraceSpans[traceId]
        if (!spans) {
            return HttpResponse.json({ error: 'trace not found (never sampled, or evicted from the in-memory buffer)' }, { status: 404 })
        }
        const base = MOCK_TRACE_BASE
        return HttpResponse.json({
            trace_id: traceId,
            spans: spans.map((s) => ({
                trace_id: traceId,
                span_id: s.span_id,
                parent_span_id: s.parent_span_id,
                name: s.name,
                kind: s.kind,
                start_time: new Date(base + s.start).toISOString(),
                end_time: new Date(base + s.end).toISOString(),
                status_code: s.status_code,
                status_message: s.status_message,
                attributes: s.attributes,
            })),
        })
    }),

    // ============================================
    // Claude Code Profiles API (v1)
    // ============================================
    http.get('/api/v1/config/claude', () => {
        return HttpResponse.json({
            success: true,
            exists: true,
            preferences: mockMainClaudeCodePreferences,
            defaultMode: 'acceptEdits',
            installStatusLine: true,
            showThinkingSummaries: true,
        })
    }),

    http.get('/api/v1/config/codex', () => {
        return HttpResponse.json({
            success: true,
            exists: true,
            preferences: { model_reasoning_effort: 'high' },
            writeCatalog: true,
        })
    }),

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

    http.get('/api/v1/scenario/:scenario/profiles/:profileId/claude-config', ({ params }) => {
        const { scenario, profileId } = params as { scenario: string; profileId: string }
        if (scenario !== 'claude_code' || !mockClaudeCodeProfiles.some(profile => profile.id === profileId)) {
            return HttpResponse.json({ success: false, error: 'Profile not found' }, { status: 404 })
        }
        return HttpResponse.json({ success: true, data: mockProfileClaudeConfigData(profileId) })
    }),

    http.put('/api/v1/scenario/:scenario/profiles/:profileId/claude-config', async ({ params, request }) => {
        const { profileId } = params as { profileId: string }
        const body = await request.json() as { preferences?: Record<string, string>; defaultMode?: string }
        mockProfileClaudeConfigs[profileId] = {
            preferences: body.preferences || {},
            defaultMode: body.defaultMode || 'acceptEdits',
            hasOverrides: true,
        }
        return HttpResponse.json({ success: true, data: mockProfileClaudeConfigData(profileId) })
    }),

    http.delete('/api/v1/scenario/:scenario/profiles/:profileId/claude-config', ({ params }) => {
        const { profileId } = params as { profileId: string }
        delete mockProfileClaudeConfigs[profileId]
        return HttpResponse.json({ success: true, data: mockProfileClaudeConfigData(profileId) })
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
        const models = ['gpt-5.6-sol', 'gpt-5.6-luna']
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
            lines.push('requires_openai_auth = true')
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
            models: ['gpt-5.6-sol', 'gpt-5.6-luna'],
            message: 'Codex configuration applied',
        })
    }),
]
