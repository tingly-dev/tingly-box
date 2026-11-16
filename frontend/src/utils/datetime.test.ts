import { describe, expect, it } from 'vitest';
import { formatLogTime } from './datetime';

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
