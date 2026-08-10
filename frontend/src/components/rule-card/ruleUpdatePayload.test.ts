import { vi } from 'vitest';
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

describe('tier normalization', () => {
    it('compacts tier gaps in the payload (contiguous from 0)', () => {
        const gappy = makeConfig({
            providers: [
                { uuid: 'svc-1', provider: 'prov-1', model: 'a', tier: 1 },
                { uuid: 'svc-2', provider: 'prov-1', model: 'b', tier: 3 },
                { uuid: 'svc-3', provider: 'prov-1', model: 'c', tier: 3 },
            ],
        });

        const payload = buildRuleUpdatePayload(baseRule, gappy);

        expect(payload.services.map((s) => s.tier)).toEqual([0, 1, 1]);
    });

    it('normalizes after the filter, so a dropped incomplete entry closes its tier', () => {
        const withIncomplete = makeConfig({
            providers: [
                // Incomplete (no model) entry holds T0 alone — it is filtered
                // out of the payload, so the survivors must renumber from 0.
                { uuid: 'svc-0', provider: 'prov-1', model: '', tier: 0 },
                { uuid: 'svc-1', provider: 'prov-1', model: 'a', tier: 1 },
                { uuid: 'svc-2', provider: 'prov-1', model: 'b', tier: 2 },
            ],
        });

        const payload = buildRuleUpdatePayload(baseRule, withIncomplete);

        expect(payload.services.map((s) => s.tier)).toEqual([0, 1]);
    });
});
