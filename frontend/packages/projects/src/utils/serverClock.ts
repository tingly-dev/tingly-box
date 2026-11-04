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

// Latency, a cached response (its Date is when it was first made) and
// truncation all make a sample read the server as further behind than it
// is, never ahead. So the best estimate is the largest offset seen.
export function noteServerDate(header: string | null | undefined): void {
    if (!header) return;
    const at = Date.parse(header);
    if (Number.isNaN(at)) return;
    const next = at + HEADER_MIDPOINT_MS - Date.now();
    if (!known || next > offset) {
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
