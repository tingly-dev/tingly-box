import { describe, expect, it } from 'vitest';
import { buildMonthLabels } from './TokenHeatmap';

// One week = 7 ISO dates starting at `start`.
const week = (start: string) => {
    const d = new Date(`${start}T00:00:00`);
    return Array.from({ length: 7 }, (_, i) => {
        const x = new Date(d);
        x.setDate(d.getDate() + i);
        return `${x.getFullYear()}-${String(x.getMonth() + 1).padStart(2, '0')}-${String(x.getDate()).padStart(2, '0')}`;
    });
};

describe('buildMonthLabels', () => {
    it('drops a month label that would run into the next one', () => {
        // Only week 0 ends in October; week 1 already ends in November.
        const weeks = ['2025-10-20', '2025-10-27', '2025-11-03', '2025-11-10', '2025-11-17', '2025-11-24', '2025-12-01'].map(week);
        expect(buildMonthLabels(weeks, 'en')).toEqual([null, 'Nov', null, null, null, null, 'Dec']);
    });

    it('keeps labels that have room', () => {
        const weeks = ['2025-09-29', '2025-10-06', '2025-10-13', '2025-10-20', '2025-10-27'].map(week);
        // The last month keeps its label however few weeks it has left.
        expect(buildMonthLabels(weeks, 'en')).toEqual(['Oct', null, null, null, 'Nov']);
    });
});
