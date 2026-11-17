import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { ProviderQuota } from '@/types/quota';
import { useQuotaBars } from './useQuotaBars';

const win = (key: string, minutes: number) => ({
    key, label: key, used: 10, limit: 100, used_percent: 10, unit: 'percent', kind: 'limit', window_minutes: minutes,
});
const quotaOf = (breakdowns: unknown[]) => ({ windows: [], breakdowns } as unknown as ProviderQuota);

describe('useQuotaBars order', () => {
    const bds = [
        { group: 'resource', key: 'b', label: 'B', windows: [win('weekly', 10080), win('interval', 1440)] },
        { group: 'model', key: 'z', label: 'Z', windows: [win('w', 60)] },
        { group: 'resource', key: 'a', label: 'A', windows: [win('w', 60)] },
    ];

    it('gives the same groups, entries and lead window for any arrival order', () => {
        const summarise = (list: unknown[]) => {
            const { result } = renderHook(() => useQuotaBars(quotaOf(list)));
            return result.current.resourceItems.map(g => ({
                group: g.key,
                entries: g.entries.map(e => `${e.key}:${e.window.key}`),
            }));
        };
        const expected = [
            { group: 'model', entries: ['z:w'] },
            { group: 'resource', entries: ['a:w', 'b:interval'] },
        ];
        expect(summarise(bds)).toEqual(expected);
        expect(summarise([...bds].reverse())).toEqual(expected);
        expect(summarise([bds[2], bds[0], bds[1]])).toEqual(expected);
    });
});
