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
