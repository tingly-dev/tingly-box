import {afterEach, describe, expect, it} from 'vitest';
import {CHECKPOINT_MS, GAP_MS, clockLabel, dayLabel, exactLabel, fullLabel, markLabel, nextMidnight, timeMarks} from './timeMarks';

// Local-time stamps, so the tests mean the same in any machine's zone.
const at = (y: number, mo: number, d: number, h = 0, mi = 0) => new Date(y, mo - 1, d, h, mi).toISOString();
const NOW = new Date(2026, 10, 19, 15, 0).getTime();
const originalTZ = process.env.TZ;
afterEach(() => {
    if (originalTZ === undefined) delete process.env.TZ; else process.env.TZ = originalTZ;
});

describe('timeMarks thresholds', () => {
    it('marks the first block with its day and time', () => {
        const marks = timeMarks([at(2026, 11, 19, 9, 0), at(2026, 11, 19, 9, 5)]);
        expect([...marks.keys()]).toEqual([0]);
        expect(marks.get(0)).toMatchObject({day: true, clock: true});
    });

    it('stays quiet while messages keep coming within the hour', () => {
        const times = [0, 10, 25, 50, 105].map((m) => at(2026, 11, 19, 9, m));
        // 9:00 .. 10:45: every step is under an hour, and the run is under three.
        expect([...timeMarks(times).keys()]).toEqual([0]);
    });

    it('marks the hour after a silence of an hour or more, and not just under it', () => {
        const base = new Date(2026, 10, 19, 9, 0).getTime();
        const iso = (ms: number) => new Date(base + ms).toISOString();
        const marks = timeMarks([iso(0), iso(GAP_MS - 1), iso(GAP_MS - 1 + GAP_MS)]);
        expect([...marks.keys()]).toEqual([0, 2]);
        expect(marks.get(2)).toMatchObject({day: false, clock: true});
    });

    it('marks a new day, with no time of day when it was crossed mid-flow', () => {
        const marks = timeMarks([at(2026, 11, 18, 23, 50), at(2026, 11, 19, 0, 10)]);
        expect(marks.get(1)).toMatchObject({day: true, clock: false});
    });

    it('marks a new day with its time after a silence', () => {
        const marks = timeMarks([at(2026, 11, 18, 17, 0), at(2026, 11, 19, 9, 12)]);
        expect(marks.get(1)).toMatchObject({day: true, clock: true});
    });

    it('adds a checkpoint inside an unbroken run, measured from the last marker', () => {
        const base = new Date(2026, 10, 19, 9, 0).getTime();
        const step = 50 * 60 * 1000;
        const times = Array.from({length: 10}, (_, i) => new Date(base + i * step).toISOString());
        const keys = [...timeMarks(times).keys()];
        // 50 minutes apart never trips the gap; 4 steps (3h20) is the first past three hours.
        expect(keys).toEqual([0, 4, 8]);
        expect(4 * step).toBeGreaterThanOrEqual(CHECKPOINT_MS);
        expect(3 * step).toBeLessThan(CHECKPOINT_MS);
    });

    it('ignores blocks with no valid time', () => {
        const marks = timeMarks([undefined, at(2026, 11, 19, 9, 0), 'garbage', at(2026, 11, 19, 9, 5)]);
        expect([...marks.keys()]).toEqual([1]);
    });

    it('counts a day by the local calendar, so a 25-hour DST day is one day', () => {
        process.env.TZ = 'America/New_York';
        // 2026-11-01 is when clocks go back: that local day lasts 25 hours.
        const marks = timeMarks([new Date(2026, 10, 1, 0, 30).toISOString(), new Date(2026, 10, 1, 1, 30).toISOString(), new Date(2026, 10, 1, 23, 30).toISOString(), new Date(2026, 10, 2, 0, 5).toISOString()]);
        expect(marks.get(2)).toMatchObject({day: false, clock: true});
        expect(marks.get(3)).toMatchObject({day: true, clock: false});
    });

    it('is stable: blocks added at the end never change a marker already decided', () => {
        const times = [at(2026, 11, 17, 9, 0), at(2026, 11, 17, 9, 20), at(2026, 11, 17, 13, 0), at(2026, 11, 18, 8, 0)];
        const before = timeMarks(times.slice(0, 3));
        const after = timeMarks(times);
        for (const [i, mark] of before) expect(after.get(i)).toEqual(mark);
        expect([...after.keys()]).toEqual([0, 2, 3]);
    });

    it('moves the leading marker when earlier blocks are loaded above', () => {
        const times = [at(2026, 11, 19, 9, 0), at(2026, 11, 19, 9, 20)];
        const older = [at(2026, 11, 19, 8, 30), ...times];
        expect([...timeMarks(times).keys()]).toEqual([0]);
        expect([...timeMarks(older).keys()]).toEqual([0]);
        expect([...timeMarks([at(2026, 11, 18, 8, 0), ...times]).keys()]).toEqual([0, 1]);
    });
});

describe('time labels', () => {
    it('names today and yesterday in the locale, and older days by date', () => {
        expect(dayLabel(at(2026, 11, 19, 8, 0), 'en', NOW)).toBe('Today');
        expect(dayLabel(at(2026, 11, 18, 23, 0), 'en', NOW)).toBe('Yesterday');
        expect(dayLabel(at(2026, 11, 19, 8, 0), 'zh', NOW)).toBe('今天');
        expect(dayLabel(at(2026, 11, 18, 23, 0), 'zh', NOW)).toBe('昨天');
        expect(dayLabel(at(2026, 11, 19, 8, 0), 'ru', NOW)).toBe('Сегодня');
        expect(dayLabel(at(2026, 11, 18, 23, 0), 'ru', NOW)).toBe('Вчера');
        expect(dayLabel(at(2026, 11, 17, 9, 0), 'en', NOW)).toBe('Tue, Nov 17');
    });

    it('adds the year only for another year', () => {
        expect(dayLabel(at(2025, 12, 31, 9, 0), 'en', NOW)).toBe('Dec 31, 2025');
    });

    it('treats a day by its calendar date, not by 24-hour distance', () => {
        const lateNow = new Date(2026, 10, 19, 0, 10).getTime();
        expect(dayLabel(at(2026, 11, 18, 23, 55), 'en', lateNow)).toBe('Yesterday');
    });

    it('formats the time of day in the locale style', () => {
        expect(clockLabel(at(2026, 11, 19, 14, 32), 'en')).toMatch(/^2:32\sPM$/);
        expect(clockLabel(at(2026, 11, 19, 14, 32), 'ru')).toBe('14:32');
        expect(clockLabel(at(2026, 11, 19, 14, 32), 'zh')).toBe('14:32');
    });

    it('joins day and time, and gives the exact moment to readers who need it', () => {
        const mark = {at: at(2026, 11, 18, 9, 12), day: true, clock: true};
        expect(markLabel(mark, 'en', NOW)).toMatch(/^Yesterday · 9:12\sAM$/);
        expect(markLabel({...mark, clock: false}, 'en', NOW)).toBe('Yesterday');
        expect(markLabel({...mark, day: false}, 'en', NOW)).toMatch(/^9:12\sAM$/);
        expect(fullLabel(mark.at, 'en')).toContain('Wednesday, November 18, 2026');
        expect(exactLabel(mark.at, 'en')).toMatch(/9:12:00/);
    });

    it('survives a locale tag the runtime rejects', () => {
        expect(() => dayLabel(at(2026, 11, 19, 8, 0), 'en-US@posix', NOW)).not.toThrow();
        expect(clockLabel(at(2026, 11, 19, 14, 32), 'not a locale!')).not.toBe('');
    });

    it('finds the next local midnight', () => {
        expect(nextMidnight(new Date(2026, 10, 19, 23, 59).getTime())).toBe(new Date(2026, 10, 20).getTime());
    });
});
