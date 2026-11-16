import { describe, expect, it } from 'vitest';
import { CODEX_COMPACT, compactZones, lowZoneStart, planCodexCompact } from './codexCompact';

const { window, window1M } = CODEX_COMPACT;

describe('planCodexCompact', () => {
    it('default 75% on 200K leaves 50K and is fine', () => {
        expect(planCodexCompact({ window, percent: 75 })).toEqual({
            applied: 75, line: 150_000, remain: 50_000, status: 'ok',
        });
    });

    it('82% of 200K is the limit: at it no warning, above it a warning', () => {
        expect(planCodexCompact({ window, percent: 82, effectivePercent: 95 }).status).toBe('ok');
        const plan = planCodexCompact({ window, percent: 85, effectivePercent: 95 });
        expect(plan.remain).toBe(30_000);
        expect(plan.status).toBe('low');
    });

    it('the same percent is fine on a 1M window', () => {
        expect(planCodexCompact({ window: window1M, percent: 82 }).status).toBe('ok');
    });

    it('is cut down to the usable window and says so', () => {
        const plan = planCodexCompact({ window: window1M, percent: 90 });
        expect(plan.applied).toBe(82);
        expect(plan.status).toBe('limited');
    });

    it('does not impose Codex\'s 90% ceiling, only the usable window', () => {
        const plan = planCodexCompact({ window: window1M, percent: 95, effectivePercent: 100 });
        expect(plan.applied).toBe(95);
        expect(plan.status).toBe('ok');
    });
});

describe('lowZoneStart', () => {
    it('starts at 82% on 200K and at 96.4% on 1M', () => {
        expect(lowZoneStart(window)).toBeCloseTo(82);
        expect(lowZoneStart(window1M)).toBeCloseTo(96.4);
    });
});

describe('compactZones', () => {
    it('default headroom: only the capped zone, on both windows', () => {
        expect(compactZones(window, 82)).toEqual({ low: null, cappedFrom: 82 });
        expect(compactZones(window1M, 82)).toEqual({ low: null, cappedFrom: 82 });
    });

    it('a raised usable window exposes the low zone below it', () => {
        const zones = compactZones(window, 95);
        expect(zones.low?.from).toBeCloseTo(82);
        expect(zones.low?.to).toBe(95);
        expect(zones.cappedFrom).toBe(95);
    });

    it('no capped zone when the whole window is usable', () => {
        const zones = compactZones(window1M, 100);
        expect(zones.cappedFrom).toBeNull();
        expect(zones.low?.from).toBeCloseTo(96.4);
    });
});
