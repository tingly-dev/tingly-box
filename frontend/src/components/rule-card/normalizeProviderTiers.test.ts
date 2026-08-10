import { normalizeConfigRecordTiers, normalizeProviderTiers } from './utils';
import type { ConfigRecord } from '@/components/RoutingGraphTypes';

describe('normalizeProviderTiers', () => {
    it('compacts gaps while preserving relative order', () => {
        const result = normalizeProviderTiers([
            { uuid: 'a', tier: 1 },
            { uuid: 'b', tier: 3 },
            { uuid: 'c', tier: 3 },
            { uuid: 'd', tier: 7 },
        ]);
        expect(result.map((p) => p.tier)).toEqual([0, 1, 1, 2]);
    });

    it('leaves already-contiguous tiers unchanged', () => {
        const result = normalizeProviderTiers([
            { uuid: 'a', tier: 0 },
            { uuid: 'b', tier: 1 },
        ]);
        expect(result.map((p) => p.tier)).toEqual([0, 1]);
    });

    it('treats a missing tier as 0', () => {
        const result = normalizeProviderTiers([{ uuid: 'a' }, { uuid: 'b', tier: 2 }]);
        expect(result.map((p) => p.tier ?? 0)).toEqual([0, 1]);
    });

    it('handles an empty list', () => {
        expect(normalizeProviderTiers([])).toEqual([]);
    });
});

describe('normalizeConfigRecordTiers', () => {
    it('normalizes the default pool and each smart partition independently', () => {
        const record = {
            uuid: 'r', scenario: 'openai', requestModel: 'm', responseModel: '',
            active: true, providers: [
                { uuid: 'a', provider: 'p', model: 'x', tier: 2 },
            ],
            smartEnabled: true,
            smartRouting: [
                {
                    uuid: 'sr1', description: '', ops: [],
                    services: [
                        { uuid: 's1', provider: 'p', model: 'y', tier: 1 },
                        { uuid: 's2', provider: 'p', model: 'z', tier: 4 },
                    ],
                },
            ],
        } as unknown as ConfigRecord;

        const result = normalizeConfigRecordTiers(record);

        expect(result.providers.map((p) => p.tier)).toEqual([0]);
        expect(result.smartRouting?.[0].services.map((s: any) => s.tier)).toEqual([0, 1]);
    });
});
