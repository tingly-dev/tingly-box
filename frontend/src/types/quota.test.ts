import { describe, expect, it } from 'vitest';
import { formatQuotaAvailable, formatQuotaRemaining, formatQuotaUsage, quotaRemainingPercent, tightestWindow } from './quota';
import type { ProviderQuota, QuotaWindow } from './quota';

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
