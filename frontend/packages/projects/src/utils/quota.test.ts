import { describe, expect, it } from 'vitest';
import { quotaTone, subscriptionRows } from './quota';

const now = Date.parse('2026-11-06T10:00:00Z');
const at = (hours: number) => Math.floor(now / 1000) + hours * 3600;

describe('subscriptionRows', () => {
    it('reads the 5-hour and weekly windows as what is left, in that order', () => {
        const rows = subscriptionRows({
            status: 'allowed', updated_at: '2026-11-06T09:59:00Z',
            seven_day: { utilization: 0.17, resets_at: at(100) },
            five_hour: { utilization: 0.42, resets_at: at(2) },
        }, now);
        expect(rows.map((r) => [r.key, r.usedPct, r.leftPct])).toEqual([['five_hour', 42, 58], ['seven_day', 17, 83]]);
        expect(rows[0].resetsAt).toBe(new Date(at(2) * 1000).toISOString());
    });

    it('drops a window that has already reset: its reading says nothing about the new one', () => {
        const rows = subscriptionRows({
            status: 'allowed', updated_at: '2026-11-06T05:00:00Z',
            five_hour: { utilization: 0.9, resets_at: at(-1) },
            seven_day: { utilization: 0.3, resets_at: at(50) },
        }, now);
        expect(rows.map((r) => r.key)).toEqual(['seven_day']);
    });

    it('runs past the cap without going negative', () => {
        const [row] = subscriptionRows({ status: 'rejected', updated_at: '', five_hour: { utilization: 1.08, resets_at: at(1) } }, now);
        expect(row.usedPct).toBe(108);
        expect(row.leftPct).toBe(0);
    });

    it('is nothing for a thread whose account reports no windows', () => {
        expect(subscriptionRows(undefined, now)).toEqual([]);
    });
});

describe('quotaTone', () => {
    it('warns at 80% used and fails at 90%', () => {
        expect([quotaTone(10), quotaTone(80), quotaTone(95)]).toEqual(['primary', 'warning', 'error']);
    });
});
