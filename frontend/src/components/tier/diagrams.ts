import type { ConfigRecord } from '@/components/RoutingGraphTypes';
import type { Provider } from '@/types/provider';

/**
 * Tier diagram types for different configuration scenarios
 */
export enum TierDiagramType {
    EMPTY = 'empty',
    SINGLE_PROVIDER = 'single-provider',
    TWO_PROVIDERS_SAME_TIER = 'two-providers-same-tier',
    TWO_PROVIDERS_DIFFERENT_TIERS = 'two-providers-different-tiers',
    THREE_TIERS = 'three-tiers',
    RUNTIME_FAILOVER = 'runtime-failover',
    // Direct routing diagrams
    DIRECT_SINGLE = 'direct-single',
    DIRECT_MULTIPLE_TIERS = 'direct-multiple-tiers',
    // Smart routing diagrams
    SMART_BASIC = 'smart-basic',
    SMART_CONDITIONS = 'smart-conditions',
    SMART_COMPLEX = 'smart-complex',
}

/**
 * Annotation for diagram elements
 */
export interface Annotation {
    target: string; // CSS selector or element identifier
    text: string; // i18n key
    position?: 'top' | 'bottom' | 'left' | 'right';
}

/**
 * Single step in the tier guide
 */
export interface GuideStep {
    diagram: TierDiagramType;
    title: string; // i18n key (will be looked up as `rule.tier.guide.steps.{stepNumber}.title`)
    content: string; // i18n key
    annotations?: Annotation[];
    // Which routing mode this step belongs to (routing guide only). Lets the
    // dialog filter steps without relying on array-index magic numbers.
    mode?: 'direct' | 'smart';
    // When set, the step renders a mock page-toolbar above the diagram with this
    // button highlighted, so users can recognise what to click (the real button
    // lives in the toolbar, off-graph).
    toolbarHighlight?: 'connectAI' | 'newRule';
}

/**
 * Mock provider data for diagrams
 */
const MOCK_PROVIDERS: Provider[] = [
    {
        uuid: 'provider-1',
        name: 'OpenAI',
        api_base: 'https://api.openai.com',
        api_key: 'sk-***',
        api_style: 'openai',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
    },
    {
        uuid: 'provider-2',
        name: 'Anthropic',
        api_base: 'https://api.anthropic.com',
        api_key: 'sk-ant-***',
        api_style: 'anthropic',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
    },
    {
        uuid: 'provider-3',
        name: 'Azure OpenAI',
        api_base: 'https://azure.openai.com',
        api_key: 'sk-azure-***',
        api_style: 'openai',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
    },
    {
        uuid: 'provider-4',
        name: 'DeepSeek',
        api_base: 'https://api.deepseek.com',
        api_key: 'sk-ds-***',
        api_style: 'openai',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
    },
];

/**
 * Diagram data for each scenario
 */
export const TIER_DIAGRAM_DATA: Record<string, {
    record: ConfigRecord;
    providers: Provider[];
    active: boolean;
}> = {
    [TierDiagramType.EMPTY]: {
        record: {
            uuid: 'rule-empty',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Example rule',
            providers: [],
            lbTactic: { type: 'random', params: {} },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.SINGLE_PROVIDER]: {
        record: {
            uuid: 'rule-single',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Single provider rule',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
            ],
            lbTactic: { type: 'random', params: {} },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.TWO_PROVIDERS_SAME_TIER]: {
        record: {
            uuid: 'rule-same-tier',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Load balancing example',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 0,
                    active: true,
                },
            ],
            lbTactic: { type: 'tier', params: { within_tier_tactic: 'random' } },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.TWO_PROVIDERS_DIFFERENT_TIERS]: {
        record: {
            uuid: 'rule-different-tiers',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Primary and fallback example',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 1,
                    active: true,
                },
            ],
            lbTactic: { type: 'tier', params: { within_tier_tactic: 'random' } },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.THREE_TIERS]: {
        record: {
            uuid: 'rule-three-tiers',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Multi-tier fallback example',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 1,
                    active: true,
                },
                {
                    uuid: 'svc-3',
                    provider: 'provider-3',
                    model: 'gpt-4',
                    tier: 2,
                    active: true,
                },
            ],
            lbTactic: { type: 'tier', params: { within_tier_tactic: 'random' } },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.RUNTIME_FAILOVER]: {
        record: {
            uuid: 'rule-failover',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Failover scenario',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 1,
                    active: true,
                },
            ],
            lbTactic: { type: 'tier', params: { within_tier_tactic: 'random' } },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    // Direct routing diagrams
    [TierDiagramType.DIRECT_SINGLE]: {
        record: {
            uuid: 'rule-direct-single',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Direct routing with single provider',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
            ],
            lbTactic: { type: 'random', params: {} },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.DIRECT_MULTIPLE_TIERS]: {
        record: {
            uuid: 'rule-direct-multi',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Direct routing with multiple tiers',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-3',
                    provider: 'provider-3',
                    model: 'gpt-4',
                    tier: 1,
                    active: true,
                },
            ],
            lbTactic: { type: 'tier', params: { within_tier_tactic: 'random' } },
            smartEnabled: false,
            smartRouting: [],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    // Smart routing diagrams
    [TierDiagramType.SMART_BASIC]: {
        record: {
            uuid: 'rule-smart-basic',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Smart routing with basic conditions',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 0,
                    active: true,
                },
            ],
            lbTactic: { type: 'smart', params: {} },
            smartEnabled: true,
            smartRouting: [
                {
                    uuid: 'smart-rule-1',
                    description: 'Route Claude requests to Anthropic',
                    ops: [
                        {
                            uuid: 'op-1',
                            position: 'model',
                            operation: 'contains',
                            value: 'claude',
                        },
                    ],
                    services: [
                        {
                            uuid: 'svc-2',
                            provider: 'provider-2',
                            model: 'claude-3-5-sonnet-20241022',
                            active: true,
                        },
                    ],
                },
            ],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.SMART_CONDITIONS]: {
        record: {
            uuid: 'rule-smart-conditions',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Smart routing with multiple conditions',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-3',
                    provider: 'provider-3',
                    model: 'gpt-4',
                    tier: 1,
                    active: true,
                },
            ],
            lbTactic: { type: 'smart', params: {} },
            smartEnabled: true,
            smartRouting: [
                {
                    uuid: 'smart-rule-1',
                    description: 'Route Claude requests to Anthropic',
                    ops: [
                        {
                            uuid: 'op-1',
                            position: 'model',
                            operation: 'contains',
                            value: 'claude',
                        },
                    ],
                    services: [
                        {
                            uuid: 'svc-2',
                            provider: 'provider-2',
                            model: 'claude-3-5-sonnet-20241022',
                            active: true,
                        },
                    ],
                },
                {
                    uuid: 'smart-rule-2',
                    description: 'Route large token requests to Azure',
                    ops: [
                        {
                            uuid: 'op-2',
                            position: 'token',
                            operation: 'gt',
                            value: '4000',
                        },
                    ],
                    services: [
                        {
                            uuid: 'svc-3',
                            provider: 'provider-3',
                            model: 'gpt-4',
                            active: true,
                        },
                    ],
                },
            ],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },

    [TierDiagramType.SMART_COMPLEX]: {
        record: {
            uuid: 'rule-smart-complex',
            requestModel: 'claude-3-5-sonnet-20241022',
            description: 'Smart routing with complex conditions',
            providers: [
                {
                    uuid: 'svc-1',
                    provider: 'provider-1',
                    model: 'gpt-4',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-2',
                    provider: 'provider-2',
                    model: 'claude-3-5-sonnet-20241022',
                    tier: 0,
                    active: true,
                },
                {
                    uuid: 'svc-3',
                    provider: 'provider-3',
                    model: 'gpt-4',
                    tier: 1,
                    active: true,
                },
                {
                    uuid: 'svc-4',
                    provider: 'provider-4',
                    model: 'deepseek-v4-flash',
                    tier: 1,
                    active: true,
                },
            ],
            lbTactic: { type: 'smart', params: {} },
            smartEnabled: true,
            smartRouting: [
                {
                    uuid: 'smart-rule-1',
                    description: 'Route Claude requests to Anthropic',
                    ops: [
                        {
                            uuid: 'op-1',
                            position: 'model',
                            operation: 'contains',
                            value: 'claude',
                        },
                    ],
                    services: [
                        {
                            uuid: 'svc-2',
                            provider: 'provider-2',
                            model: 'claude-3-5-sonnet-20241022',
                            active: true,
                        },
                    ],
                },
                {
                    uuid: 'smart-rule-2',
                    description: 'Route large token requests to Azure',
                    ops: [
                        {
                            uuid: 'op-2',
                            position: 'token',
                            operation: 'gt',
                            value: '4000',
                        },
                    ],
                    services: [
                        {
                            uuid: 'svc-3',
                            provider: 'provider-3',
                            model: 'gpt-4',
                            active: true,
                        },
                    ],
                },
                {
                    uuid: 'smart-rule-3',
                    description: 'Route @@@ds commands to DeepSeek',
                    ops: [
                        {
                            uuid: 'op-3',
                            position: 'latest_user',
                            operation: 'contains',
                            value: '@@@ds',
                        },
                    ],
                    services: [
                        {
                            uuid: 'svc-4',
                            provider: 'provider-4',
                            model: 'deepseek-v4-flash',
                            active: true,
                        },
                    ],
                },
            ],
        },
        providers: MOCK_PROVIDERS,
        active: true,
    },
};

/**
 * Tier guide steps configuration
 *
 * Each step includes:
 * - diagram: Which scenario to display
 * - title: i18n key for step title (format: `rule.tier.guide.steps.{stepNumber}.title`)
 * - content: i18n key for explanation text
 * - annotations: Optional callout annotations for key elements
 */
export const TIER_GUIDE_STEPS: GuideStep[] = [
    {
        diagram: TierDiagramType.SINGLE_PROVIDER,
        title: 'rule.tier.guide.steps.1.title',
        content: 'rule.tier.guide.steps.1.content',
        annotations: [
            { target: '.tier-node-0', text: 'rule.tier.guide.steps.1.annotation.tier' },
            { target: '.service-node-0', text: 'rule.tier.guide.steps.1.annotation.service' },
        ],
    },
    {
        diagram: TierDiagramType.TWO_PROVIDERS_SAME_TIER,
        title: 'rule.tier.guide.steps.2.title',
        content: 'rule.tier.guide.steps.2.content',
        annotations: [
            { target: '.tier-node-0', text: 'rule.tier.guide.steps.2.annotation.loadBalance' },
            { target: '.service-node-0', text: 'rule.tier.guide.steps.2.annotation.multiple' },
        ],
    },
    {
        diagram: TierDiagramType.TWO_PROVIDERS_DIFFERENT_TIERS,
        title: 'rule.tier.guide.steps.3.title',
        content: 'rule.tier.guide.steps.3.content',
        annotations: [
            { target: '.tier-node-0', text: 'rule.tier.guide.steps.3.annotation.primary' },
            { target: '.tier-node-1', text: 'rule.tier.guide.steps.3.annotation.fallback' },
            { target: '.service-node-0', text: 'rule.tier.guide.steps.3.annotation.actionButtons' },
        ],
    },
    {
        diagram: TierDiagramType.RUNTIME_FAILOVER,
        title: 'rule.tier.guide.steps.4.title',
        content: 'rule.tier.guide.steps.4.content',
        annotations: [
            { target: '.tier-node-0', text: 'rule.tier.guide.steps.4.annotation.circuitBreaker' },
            { target: '.tier-node-1', text: 'rule.tier.guide.steps.4.annotation.automaticFailover' },
        ],
    },
    {
        diagram: TierDiagramType.THREE_TIERS,
        title: 'rule.tier.guide.steps.5.title',
        content: 'rule.tier.guide.steps.5.content',
        annotations: [
            { target: '.tier-node-0', text: 'rule.tier.guide.steps.5.annotation.priority' },
            { target: '.tier-node-1', text: 'rule.tier.guide.steps.5.annotation.cascade' },
        ],
    },
];

/**
 * Routing guide steps configuration for Direct vs Smart routing
 *
 * Each step includes:
 * - diagram: Which scenario to display
 * - title: i18n key for step title (format: `rule.routing.guide.steps.{stepNumber}.title`)
 * - content: i18n key for explanation text
 * - annotations: Optional callout annotations for key elements
 */
export const ROUTING_GUIDE_STEPS: GuideStep[] = [
    // --- Direct routing: start from zero, then routing behaviour ---
    {
        // Step 1: you need a credential before anything routes.
        diagram: TierDiagramType.EMPTY,
        mode: 'direct',
        toolbarHighlight: 'connectAI',
        title: 'rule.routing.guide.steps.connectAI.title',
        content: 'rule.routing.guide.steps.connectAI.content',
        annotations: [
            { target: '.entry-node', text: 'rule.routing.guide.steps.connectAI.annotation.toolbar' },
            { target: '.service-node-0', text: 'rule.routing.guide.steps.connectAI.annotation.empty' },
        ],
    },
    {
        // Step 2: from an empty rule, add your first model; New Rule adds more rules.
        diagram: TierDiagramType.EMPTY,
        mode: 'direct',
        toolbarHighlight: 'newRule',
        title: 'rule.routing.guide.steps.addModel.title',
        content: 'rule.routing.guide.steps.addModel.content',
        annotations: [
            { target: '.action-add-node', text: 'rule.routing.guide.steps.addModel.annotation.addModel' },
            { target: '.entry-node', text: 'rule.routing.guide.steps.addModel.annotation.newRule' },
        ],
    },
    {
        // Step 3: swap / edit / remove a model that's already there.
        diagram: TierDiagramType.SINGLE_PROVIDER,
        mode: 'direct',
        title: 'rule.routing.guide.steps.editModel.title',
        content: 'rule.routing.guide.steps.editModel.content',
        annotations: [
            { target: '.service-node-0', text: 'rule.routing.guide.steps.editModel.annotation.click' },
            { target: '.service-node-0', text: 'rule.routing.guide.steps.editModel.annotation.remove' },
        ],
    },
    {
        diagram: TierDiagramType.TWO_PROVIDERS_SAME_TIER,
        mode: 'direct',
        title: 'rule.routing.guide.steps.loadBalance.title',
        content: 'rule.routing.guide.steps.loadBalance.content',
        annotations: [
            { target: '.tier-node-0', text: 'rule.routing.guide.steps.loadBalance.annotation.sameTier' },
            { target: '.service-node-0', text: 'rule.routing.guide.steps.loadBalance.annotation.services' },
        ],
    },
    {
        diagram: TierDiagramType.DIRECT_MULTIPLE_TIERS,
        mode: 'direct',
        title: 'rule.routing.guide.steps.tierFallback.title',
        content: 'rule.routing.guide.steps.tierFallback.content',
        annotations: [
            { target: '.tier-node-0', text: 'rule.routing.guide.steps.tierFallback.annotation.primary' },
            { target: '.tier-node-1', text: 'rule.routing.guide.steps.tierFallback.annotation.fallback' },
        ],
    },
    // --- Smart routing ---
    {
        diagram: TierDiagramType.SMART_BASIC,
        mode: 'smart',
        title: 'rule.routing.guide.steps.smartIntro.title',
        content: 'rule.routing.guide.steps.smartIntro.content',
        annotations: [
            { target: '.smart-button', text: 'rule.routing.guide.steps.smartIntro.annotation.smartButton' },
            { target: '.service-node-1', text: 'rule.routing.guide.steps.smartIntro.annotation.conditional' },
        ],
    },
    {
        diagram: TierDiagramType.SMART_CONDITIONS,
        mode: 'smart',
        title: 'rule.routing.guide.steps.smartConditions.title',
        content: 'rule.routing.guide.steps.smartConditions.content',
        annotations: [
            { target: '.service-node-1', text: 'rule.routing.guide.steps.smartConditions.annotation.modelBased' },
            { target: '.service-node-2', text: 'rule.routing.guide.steps.smartConditions.annotation.tokenBased' },
        ],
    },
    {
        diagram: TierDiagramType.SMART_COMPLEX,
        mode: 'smart',
        title: 'rule.routing.guide.steps.smartAdvanced.title',
        content: 'rule.routing.guide.steps.smartAdvanced.content',
        annotations: [
            { target: '.service-node-0', text: 'rule.routing.guide.steps.smartAdvanced.annotation.defaultRoute' },
            { target: '.service-node-1', text: 'rule.routing.guide.steps.smartAdvanced.annotation.claudeRoute' },
            { target: '.service-node-2', text: 'rule.routing.guide.steps.smartAdvanced.annotation.largeContext' },
        ],
    },
];
