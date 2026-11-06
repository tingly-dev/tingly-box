import type { Thread } from '../client';

/** One quota window as a card shows it: the share left, and when it comes back. */
export interface QuotaRow {
    key: 'five_hour' | 'seven_day';
    usedPct: number;
    leftPct: number;
    /** ISO time the window resets. */
    resetsAt: string;
}

/**
 * The subscription windows Claude Code last read for a thread's account (the
 * 5h / 7d of its own status line). A window whose reset has passed is left out:
 * it describes a period that is over, and the next turn brings the new reading.
 */
export function subscriptionRows(limits: Thread['rate_limits'], nowMs: number): QuotaRow[] {
    if (!limits) return [];
    const rows: QuotaRow[] = [];
    for (const key of ['five_hour', 'seven_day'] as const) {
        const w = limits[key];
        if (!w || w.resets_at * 1000 <= nowMs) continue;
        const usedPct = Math.round(w.utilization * 100);
        rows.push({ key, usedPct, leftPct: Math.max(0, 100 - usedPct), resetsAt: new Date(w.resets_at * 1000).toISOString() });
    }
    return rows;
}

/** The bar's colour for a window that is `usedPct` spent. */
export function quotaTone(usedPct: number): 'primary' | 'warning' | 'error' {
    return usedPct >= 90 ? 'error' : usedPct >= 80 ? 'warning' : 'primary';
}
