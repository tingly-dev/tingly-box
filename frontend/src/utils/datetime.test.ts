import { describe, expect, it } from 'vitest';
import { formatLogTime, getLocalMidnight, toLocalISOString } from './datetime';

describe('formatLogTime', () => {
    const now = new Date(2026, 9, 24, 15, 0, 0);

    it('shows only the time for today', () => {
        const value = formatLogTime(new Date(2026, 9, 24, 3, 9, 59).toISOString(), now);
        expect(value).not.toMatch(/Oct|2026/);
        expect(value).toMatch(/3:09:59/);
    });

    it('adds a short date for an earlier day', () => {
        const value = formatLogTime(new Date(2026, 9, 23, 3, 9, 59).toISOString(), now);
        expect(value).toMatch(/23/);
        expect(value).toMatch(/3:09:59/);
    });

    it('returns the input when it cannot be parsed', () => {
        expect(formatLogTime('not a time', now)).toBe('not a time');
    });
});

describe('local-day boundary (today / tomorrow)', () => {
    // Constructed in local time, so these invariants hold in every timezone.
    it('getLocalMidnight returns 00:00 local and the window contains the instant', () => {
        const at = new Date(2026, 10, 26, 1, 58, 0); // 01:58 local, past midnight
        const midnight = getLocalMidnight(at);

        expect(midnight.getFullYear()).toBe(2026);
        expect(midnight.getMonth()).toBe(10);
        expect(midnight.getDate()).toBe(26);
        expect(midnight.getHours()).toBe(0);
        expect(midnight.getMinutes()).toBe(0);
        expect(midnight.getSeconds()).toBe(0);

        // The "today" window [midnight, now] always contains the instant.
        expect(midnight.getTime()).toBeLessThanOrEqual(at.getTime());
        expect(at.getTime() - midnight.getTime()).toBeLessThan(24 * 60 * 60 * 1000);
    });

    it('toLocalISOString carries an explicit offset and round-trips the instant', () => {
        const midnight = getLocalMidnight(new Date(2026, 10, 26, 1, 58, 0));
        const iso = toLocalISOString(midnight);

        expect(iso).toMatch(/[+-]\d{2}:\d{2}$/);
        // Same wall clock at local midnight.
        expect(iso.startsWith('2026-11-26T00:00:00')).toBe(true);
        // The offset makes the serialized instant unambiguous.
        expect(new Date(iso).getTime()).toBe(midnight.getTime());
    });

    // Exact UTC instants only make sense under a known zone; run this suite
    // with TZ=Asia/Shanghai (or TZ=America/Los_Angeles for the west case) to
    // exercise them. Under UTC they are skipped.
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;

    it('maps a UTC+8 local midnight to 16:00Z the day before', () => {
        if (zone !== 'Asia/Shanghai') return;
        // 2026-11-25T17:58Z is 2026-11-26 01:58 in UTC+8.
        const at = new Date('2026-11-25T17:58:00Z');
        const midnight = getLocalMidnight(at);
        expect(midnight.toISOString()).toBe('2026-11-25T16:00:00.000Z');
        expect(toLocalISOString(midnight)).toBe('2026-11-26T00:00:00+08:00');
    });

    it('maps a UTC-8 local midnight to 08:00Z the same day', () => {
        if (zone !== 'America/Los_Angeles') return;
        // 2026-11-26T09:00Z is 2026-11-26 01:00 in PST (UTC-8, no DST).
        const at = new Date('2026-11-26T09:00:00Z');
        const midnight = getLocalMidnight(at);
        expect(midnight.toISOString()).toBe('2026-11-26T08:00:00.000Z');
        expect(toLocalISOString(midnight)).toBe('2026-11-26T00:00:00-08:00');
    });
});
