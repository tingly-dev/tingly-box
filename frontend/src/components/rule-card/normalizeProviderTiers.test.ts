import { normalizeProviderTiers } from './utils';

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

    it('returns identical entries untouched when already contiguous', () => {
        const input = [
            { uuid: 'a', tier: 0 },
            { uuid: 'b', tier: 1 },
        ];
        const result = normalizeProviderTiers(input);
        expect(result[0]).toBe(input[0]);
        expect(result[1]).toBe(input[1]);
    });

    it('treats a missing tier as 0', () => {
        const result = normalizeProviderTiers([{ uuid: 'a' }, { uuid: 'b', tier: 2 }]);
        expect(result.map((p) => p.tier ?? 0)).toEqual([0, 1]);
    });

    it('handles an empty list', () => {
        expect(normalizeProviderTiers([])).toEqual([]);
    });
});
