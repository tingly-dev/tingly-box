// Model tiers per Claude Code routing for mock mode (GET /scenario/claude_code/models):
// the main routing is unified (one model for every tier), profile p1 routes
// tiers separately. Shared by the scenario mock and the Desk status mock.
export type MockTier = { alias: string; model: string; provider_name: string; provider_model: string }

export const mockClaudeCodeModels: Record<string, { unified: boolean; tiers: MockTier[] }> = {
    '': { unified: true, tiers: [{ alias: '', model: 'cc', provider_name: 'Anthropic (team)', provider_model: 'claude-sonnet-4-5' }] },
    p1: {
        unified: false,
        tiers: [
            { alias: '', model: 'default', provider_name: 'DeepSeek', provider_model: 'deepseek-chat' },
            { alias: 'opus', model: 'opus', provider_name: 'Zhipu', provider_model: 'glm-4.6' },
            { alias: 'sonnet', model: 'sonnet', provider_name: 'DeepSeek', provider_model: 'deepseek-chat' },
            { alias: 'haiku', model: 'haiku', provider_name: 'DeepSeek', provider_model: 'deepseek-chat' },
            { alias: 'fable', model: 'fable', provider_name: 'Zhipu', provider_model: 'glm-4.6' },
        ],
    },
}
