import { describe, expect, it } from 'vitest';
import { formatQuotaAvailable, formatQuotaRemaining, formatQuotaUsage, quotaBreakdowns, quotaRemainingPercent, quotaToDisplayItems, quotaToWindows, tightestWindow } from './quota';
import type { ProviderQuota, QuotaWindow, UsageBreakdown } from './quota';

const baseWindow = {
    used: 0,
    limit: 0,
    used_percent: 0,
    unit: 'currency',
    unknown: true,
};

describe('formatQuotaUsage', () => {
    it('shows a reported available balance even when its percentage is unknown', () => {
        expect(formatQuotaUsage({
            ...baseWindow,
            available: 81.41,
            currency_code: 'CNY',
        })).toBe('81.41 CNY');
    });

    it('preserves countable usage while exposing available balance separately', () => {
        const window = {
            ...baseWindow,
            unknown: false,
            used: 30,
            limit: 100,
            used_percent: 30,
            unit: 'credits',
            available: 70,
        };

        expect(formatQuotaUsage(window)).toBe('30 / 100 credits');
        expect(formatQuotaAvailable(window)).toBe('70 credits');
    });

    it('keeps truly unknown values distinct from a zero balance', () => {
        expect(formatQuotaUsage(baseWindow)).toBe('not reported');
        expect(formatQuotaUsage({
            ...baseWindow,
            available: 0,
            currency_code: 'CNY',
        })).toBe('0 CNY');
    });
});

describe('Codex credit points', () => {
    it.each([150, 150.125, 0, 1])('formats %s points without currency or a fabricated total', (available) => {
        const window = { ...baseWindow, unit: 'credits', available };
        expect(formatQuotaAvailable(window)).toBe(`${available} credits`);
        expect(formatQuotaUsage(window)).toBe(`${available} credits`);
        expect(formatQuotaRemaining(window)).toBe(`${available} credits`);
        expect(quotaRemainingPercent(window)).toBe(0);
    });
});

describe('remaining quota', () => {
    it('prefers an explicit available amount in both value and bar', () => {
        const window = { used: 30, limit: 100, used_percent: 30, available: 65, unit: 'credits' };
        expect(formatQuotaRemaining(window)).toBe('65 / 100 credits');
        expect(quotaRemainingPercent(window)).toBe(65);
    });

    it('derives and clamps remaining quota when available is absent', () => {
        expect(formatQuotaRemaining({ used: 30, limit: 100, used_percent: 30, unit: 'requests' })).toBe('70 / 100 requests');
        expect(quotaRemainingPercent({ used: 110, limit: 100, used_percent: 110, unit: 'requests' })).toBe(0);
    });

    it('does not imply a full bar when quota is unknown', () => {
        expect(formatQuotaRemaining(baseWindow)).toBe('not reported');
        expect(quotaRemainingPercent(baseWindow)).toBe(0);
    });
});

describe('tightestWindow', () => {
    const limitWindow = (key: string, usedPercent: number, windowMinutes?: number): QuotaWindow => ({
        key,
        label: key,
        used: usedPercent,
        limit: 100,
        used_percent: usedPercent,
        unit: 'percent',
        kind: 'limit',
        window_minutes: windowMinutes,
    } as QuotaWindow);
    const quotaOf = (...windows: QuotaWindow[]) => ({ windows } as ProviderQuota);

    it('picks the most used window, not the shortest', () => {
        const weekly = limitWindow('7d', 96, 10080);
        expect(tightestWindow(quotaOf(limitWindow('5h', 12, 300), weekly))).toBe(weekly);
    });

    it('breaks ties toward the shorter window', () => {
        const session = limitWindow('5h', 40, 300);
        expect(tightestWindow(quotaOf(limitWindow('7d', 40, 10080), session))).toBe(session);
    });

    it('ignores windows with no comparable figure', () => {
        const unknown = { ...baseWindow, key: 'extra', label: 'extra', used_percent: 0 } as QuotaWindow;
        expect(tightestWindow(quotaOf(unknown))).toBeUndefined();
        expect(tightestWindow(undefined)).toBeUndefined();
    });
});

// The same readings reach the UI in whatever order the provider's API listed
// them, and that order is not stable between calls. Every shuffle of the same
// data must therefore render the same rows.
describe('stable detail order', () => {
    const permutations = <T,>(items: T[]): T[][] =>
        items.length <= 1
            ? [items]
            : items.flatMap((item, i) =>
                permutations([...items.slice(0, i), ...items.slice(i + 1)]).map(rest => [item, ...rest]));

    const limit = (key: string, label: string, minutes?: number): QuotaWindow => ({
        key, label, used: 10, limit: 100, used_percent: 10, unit: 'percent', kind: 'limit', window_minutes: minutes,
    } as QuotaWindow);
    const breakdown = (group: string, key: string, label: string, ...windows: QuotaWindow[]): UsageBreakdown =>
        ({ group, key, label, windows } as UsageBreakdown);

    it('orders account windows the same whatever order they arrive in', () => {
        const windows = [
            limit('model_b', 'Model B', 1440),
            limit('model_a', 'Model A', 1440),
            limit('weekly', 'Weekly', 10080),
            limit('session', '5h', 300),
        ];
        const expected = ['session', 'model_a', 'model_b', 'weekly'];
        for (const order of permutations(windows)) {
            expect(quotaToWindows({ windows: order } as ProviderQuota).map(w => w.key)).toEqual(expected);
        }
    });

    it('does not depend on usage', () => {
        const a = { ...limit('a', 'A', 1440), used_percent: 90 } as QuotaWindow;
        const b = { ...limit('b', 'B', 1440), used_percent: 5 } as QuotaWindow;
        expect(quotaToWindows({ windows: [b, a] } as ProviderQuota).map(w => w.key)).toEqual(['a', 'b']);
        expect(quotaToWindows({ windows: [a, b] } as ProviderQuota).map(w => w.key)).toEqual(['a', 'b']);
    });

    it('orders breakdowns by group, then label, then key', () => {
        const breakdowns = [
            breakdown('resource', 'z', 'Zeta', limit('w', 'w')),
            breakdown('model', 'gpt-b', 'GPT B', limit('w', 'w')),
            breakdown('model', 'gpt-a', 'GPT A', limit('w', 'w')),
            breakdown('feature', 'code_review', 'Code Review', limit('w', 'w')),
            breakdown('resource', 'card-2', 'Reset Credit', limit('w', 'w')),
            breakdown('resource', 'card-1', 'Reset Credit', limit('w', 'w')),
        ];
        const expected = ['code_review', 'gpt-a', 'gpt-b', 'card-1', 'card-2', 'z'];
        for (const order of permutations(breakdowns)) {
            expect(quotaBreakdowns({ breakdowns: order } as ProviderQuota).map(b => b.key)).toEqual(expected);
        }
    });

    it('orders the windows inside a breakdown too', () => {
        const weekly = limit('weekly', 'Weekly', 10080);
        const interval = limit('interval', 'Interval', 1440);
        for (const windows of [[weekly, interval], [interval, weekly]]) {
            const [bd] = quotaBreakdowns({ breakdowns: [breakdown('model', 'm', 'M', ...windows)] } as ProviderQuota);
            expect(bd.windows.map(w => w.key)).toEqual(['interval', 'weekly']);
        }
    });

    it('feeds the flattened display items in the same order', () => {
        const quota = (breakdowns: UsageBreakdown[]) => ({ windows: [], breakdowns } as unknown as ProviderQuota);
        const a = breakdown('model', 'a', 'A', limit('w', 'w'));
        const b = breakdown('model', 'b', 'B', limit('w', 'w'));
        expect(quotaToDisplayItems(quota([b, a])).map(i => i.key)).toEqual(['a', 'b', 'aggregate']);
        expect(quotaToDisplayItems(quota([a, b])).map(i => i.key)).toEqual(['a', 'b', 'aggregate']);
    });

    it('does not mutate its input', () => {
        const quota = { windows: [limit('b', 'B'), limit('a', 'A')], breakdowns: [breakdown('m', 'b', 'B'), breakdown('m', 'a', 'A')] } as unknown as ProviderQuota;
        quotaToWindows(quota);
        quotaBreakdowns(quota);
        expect(quota.windows!.map(w => w.key)).toEqual(['b', 'a']);
        expect(quota.breakdowns!.map(b => b.key)).toEqual(['b', 'a']);
    });
});
