import { afterEach, expect, test, vi } from 'vitest';
import { noteServerDate, resetServerClock, serverNow } from './serverClock';

afterEach(() => {
    vi.useRealTimers();
    resetServerClock();
});

test('serverNow follows the server clock named by a response Date header', () => {
    vi.useFakeTimers({ now: Date.parse('2026-11-04T13:48:53Z') });
    noteServerDate('Wed, 04 Nov 2026 13:45:49 GMT');
    expect(Date.now() - serverNow()).toBeGreaterThanOrEqual(3 * 60_000 + 3_000);
    expect(Date.now() - serverNow()).toBeLessThanOrEqual(3 * 60_000 + 4_000);
});

test('serverNow is the browser clock before any response and ignores a bad header', () => {
    vi.useFakeTimers({ now: Date.parse('2026-11-04T13:48:53Z') });
    expect(serverNow()).toBe(Date.now());
    noteServerDate(null);
    noteServerDate('not a date');
    expect(serverNow()).toBe(Date.now());
});

test('a slower or cached response does not pull the clock back', () => {
    vi.useFakeTimers({ now: Date.parse('2026-11-04T13:48:53Z') });
    noteServerDate('Wed, 04 Nov 2026 13:48:53 GMT');
    const first = serverNow();
    vi.advanceTimersByTime(10_000);
    // A cached answer made an hour ago.
    noteServerDate('Wed, 04 Nov 2026 12:48:53 GMT');
    expect(serverNow() - first).toBe(10_000);
});
