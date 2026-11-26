import { describe, expect, it } from 'vitest';
import {
    dayBands,
    dayKey,
    dayLabel,
    hasInternalGap,
    seriesDelta,
    splitByGaps,
    gapThreshold,
    type QuotaSample,
} from './quotaHistorySeries';

const at = (iso: string, value: number): QuotaSample => ({ time: Date.parse(iso), value, window: {} as never });

describe('dayKey / dayLabel', () => {
    it('uses local calendar days', () => {
        const t = new Date(2026, 0, 5, 13, 0).getTime();
        expect(dayKey(t)).toBe('2026-01-05');
        expect(dayLabel(t)).toBe('01/05');
    });
});

describe('seriesDelta', () => {
    it('is last minus first', () => {
        expect(seriesDelta([at('2026-01-01T00:00:00', 80), at('2026-01-01T01:00:00', 55)])).toBe(-25);
    });
    it('is 0 for a single sample', () => {
        expect(seriesDelta([at('2026-01-01T00:00:00', 80)])).toBe(0);
    });
});

describe('dayBands', () => {
    it('keeps low/high/latest per day and marks a missing day as a gap', () => {
        const bands = dayBands([
            at('2026-01-01T09:00:00', 90),
            at('2026-01-01T20:00:00', 60),
            // 2026-01-02 intentionally missing
            at('2026-01-03T09:00:00', 50),
            at('2026-01-03T20:00:00', 30),
        ]);
        expect(bands.map((b) => b.label)).toEqual(['01/01', '01/02', '01/03']);
        expect(bands[0]).toMatchObject({ low: 60, high: 90, latest: 60, hasData: true });
        expect(bands[1].hasData).toBe(false);
        expect(bands[2]).toMatchObject({ low: 30, high: 50, latest: 30, hasData: true });
        expect(hasInternalGap(bands)).toBe(true);
    });

    it('does not pad days outside the data span', () => {
        const bands = dayBands([at('2026-01-10T09:00:00', 50), at('2026-01-10T20:00:00', 40)]);
        expect(bands).toHaveLength(1);
        expect(hasInternalGap(bands)).toBe(false);
    });

    it('collapses a single-sample day to a zero-height band', () => {
        const bands = dayBands([at('2026-01-01T09:00:00', 42)]);
        expect(bands[0]).toMatchObject({ low: 42, high: 42, latest: 42, hasData: true });
    });
});

describe('splitByGaps / gapThreshold', () => {
    const every5 = ['00:00', '00:05', '00:10', '00:15', '00:20'].map((hm) => {
        const [h, m] = hm.split(':').map(Number);
        return at(`2026-01-01T${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:00`, 50);
    });

    it('does not split an even cadence', () => {
        expect(splitByGaps(every5)).toHaveLength(1);
        expect(gapThreshold(every5)).toBeGreaterThanOrEqual(15 * 60 * 1000);
    });

    it('splits across a coverage gap larger than the threshold', () => {
        const withGap = [...every5, at('2026-01-01T03:00:00', 50)];
        const runs = splitByGaps(withGap);
        expect(runs).toHaveLength(2);
        expect(runs[0]).toHaveLength(5);
        expect(runs[1]).toHaveLength(1);
    });

    it('treats two points as a single run (no basis to call a gap)', () => {
        expect(splitByGaps([every5[0], every5[4]])).toHaveLength(1);
        expect(gapThreshold([every5[0], every5[4]])).toBe(Number.POSITIVE_INFINITY);
    });
});
