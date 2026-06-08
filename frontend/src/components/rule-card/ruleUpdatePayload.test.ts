import { buildRuleUpdatePayload } from './ruleUpdatePayload';
import type { ConfigRecord, Rule } from '@/components/RoutingGraphTypes';

const baseRule: Pick<Rule, 'uuid' | 'scenario'> = {
    uuid: 'rule-1',
    scenario: 'anthropic',
};

function makeConfig(overrides: Partial<ConfigRecord> = {}): ConfigRecord {
    return {
        uuid: 'rule-1',
        scenario: 'anthropic',
        requestModel: 'gpt-4',
        responseModel: '',
        active: true,
        description: 'desc',
        flags: { cursorCompat: true, skipUsage: true },
        providers: [
            { uuid: 'svc-1', provider: 'prov-1', model: 'old-model', tier: 0 },
        ],
        smartEnabled: false,
        smartRouting: [],
        ...overrides,
    };
}

describe('buildRuleUpdatePayload', () => {
    it('includes flags (snake_case) so switching a model never wipes them', () => {
        // Simulate the model-select path: same config, but the service model
        // swapped to a new model — flags must survive the update payload.
        const switched = makeConfig({
            providers: [{ uuid: 'svc-1', provider: 'prov-2', model: 'new-model', tier: 0 }],
        });

        const payload = buildRuleUpdatePayload(baseRule, switched);

        expect(payload.flags).toEqual({ cursor_compat: true, skip_usage: true });
        expect(payload.services).toEqual([
            { provider: 'prov-2', model: 'new-model', weight: 0, active: true, time_window: 0, tier: 0 },
        ]);
    });

    it('emits an empty flags object (never undefined) when the rule has no flags', () => {
        const payload = buildRuleUpdatePayload(baseRule, makeConfig({ flags: undefined }));
        expect(payload.flags).toEqual({});
    });

    it('carries the full set of replace-semantics fields', () => {
        const payload = buildRuleUpdatePayload(baseRule, makeConfig());
        expect(payload).toMatchObject({
            uuid: 'rule-1',
            scenario: 'anthropic',
            request_model: 'gpt-4',
            response_model: '',
            active: true,
            description: 'desc',
            smart_enabled: false,
            smart_routing: [],
        });
    });

    it('drops services missing a provider or model', () => {
        const payload = buildRuleUpdatePayload(
            baseRule,
            makeConfig({
                providers: [
                    { uuid: 'a', provider: 'prov-1', model: 'm1' },
                    { uuid: 'b', provider: '', model: 'm2' },
                    { uuid: 'c', provider: 'prov-3', model: '' },
                ],
            }),
        );
        expect(payload.services).toEqual([
            { provider: 'prov-1', model: 'm1', weight: 0, active: true, time_window: 0, tier: 0 },
        ]);
    });
});
