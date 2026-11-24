// The server stamps events with its own clock, and the elapsed times and
// "3m ago" labels compare those stamps with the browser's. When the two
// clocks differ (a server whose clock is not synced, a remote host), every
// timer would open at the difference. Each API response carries the server's
// time in its Date header; serverNow() is the browser clock corrected by it.

let offset = 0;
let known = false;

// Date has whole-second resolution and is truncated, so the real time is
// about half a second later.
const HEADER_MIDPOINT_MS = 500;
// Truncation and latency move each sample by about a second; only a larger
// change is a clock that moved (either side was synced or stepped).
const DEADBAND_MS = 1500;

// A cached response carries the Date of when it was first made: skipped
// when the response says it was cached.
export function noteServerDate(date: string | null | undefined, age?: string | null): void {
    if (!date || (age && age !== '0')) return;
    const at = Date.parse(date);
    if (Number.isNaN(at)) return;
    const next = at + HEADER_MIDPOINT_MS - Date.now();
    if (!known || Math.abs(next - offset) > DEADBAND_MS) {
        offset = next;
        known = true;
    }
}

/** The current time on the server's clock, in epoch milliseconds. */
export function serverNow(): number {
    return Date.now() + offset;
}

/** For tests. */
export function resetServerClock(): void {
    offset = 0;
    known = false;
}
