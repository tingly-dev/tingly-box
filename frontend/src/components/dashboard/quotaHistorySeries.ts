// Pure data-shaping helpers for the Quota History charts. Kept out of the
// component so they can be unit-tested without rendering recharts, and so the
// "daily low/high → per-day band" and "gap detection" rules live in one place.
import type { QuotaWindow } from '@/types/quota';

export interface QuotaSample {
    time: number;
    value: number;
    window: QuotaWindow;
}

export type QuotaMode = 'percent' | 'available';

export interface QuotaSeries {
    key: string;
    label: string;
    mode: QuotaMode;
    samples: QuotaSample[];
}

const pad = (n: number) => String(n).padStart(2, '0');

/** Local calendar day key, so day grouping matches the axis labels. */
export function dayKey(time: number): string {
    const d = new Date(time);
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function dayLabel(time: number): string {
    const d = new Date(time);
    return `${pad(d.getMonth() + 1)}/${pad(d.getDate())}`;
}

/** Change between the first and last sample of a series (in the series' unit). */
export function seriesDelta(samples: QuotaSample[]): number {
    if (samples.length < 2) return 0;
    return samples[samples.length - 1].value - samples[0].value;
}

export interface DayBand {
    key: string;
    label: string;
    /** first sample time of the day (for sorting / keys) */
    start: number;
    /** the day's lowest and highest remaining value */
    low: number;
    high: number;
    /** value of the day's latest sample */
    latest: number;
    hasData: boolean;
}

/**
 * Turn a window's samples into one band per local day. Days with no samples
 * between the first and last populated day are kept with `hasData: false` so a
 * gap in coverage is visible instead of being silently compressed away. Days
 * outside the data's own span are NOT added — the chart should not pad the
 * whole selected range when coverage is sparse.
 */
export function dayBands(samples: QuotaSample[]): DayBand[] {
    if (!samples.length) return [];
    const sorted = [...samples].sort((a, b) => a.time - b.time);
    const byDay = new Map<string, QuotaSample[]>();
    for (const s of sorted) {
        const key = dayKey(s.time);
        const list = byDay.get(key);
        if (list) list.push(s);
        else byDay.set(key, [s]);
    }

    const first = new Date(sorted[0].time);
    const last = new Date(sorted[sorted.length - 1].time);
    const cursor = new Date(first.getFullYear(), first.getMonth(), first.getDate());
    const end = new Date(last.getFullYear(), last.getMonth(), last.getDate());
    const out: DayBand[] = [];
    while (cursor.getTime() <= end.getTime()) {
        const key = dayKey(cursor.getTime());
        const list = byDay.get(key);
        if (list && list.length) {
            let low = Infinity;
            let high = -Infinity;
            for (const s of list) {
                if (s.value < low) low = s.value;
                if (s.value > high) high = s.value;
            }
            const latest = list.reduce((a, b) => (b.time > a.time ? b : a));
            out.push({ key, label: dayLabel(cursor.getTime()), start: list[0].time, low, high, latest: latest.value, hasData: true });
        } else {
            out.push({ key, label: dayLabel(cursor.getTime()), start: cursor.getTime(), low: 0, high: 0, latest: 0, hasData: false });
        }
        cursor.setDate(cursor.getDate() + 1);
    }
    return out;
}

/** A gap is a jump between consecutive samples larger than this many ms. */
export function gapThreshold(samples: QuotaSample[]): number {
    if (samples.length < 3) return Number.POSITIVE_INFINITY;
    const diffs: number[] = [];
    for (let i = 1; i < samples.length; i++) {
        const d = samples[i].time - samples[i - 1].time;
        if (d > 0) diffs.push(d);
    }
    if (!diffs.length) return Number.POSITIVE_INFINITY;
    diffs.sort((a, b) => a - b);
    const median = diffs[Math.floor(diffs.length / 2)];
    // 3× the normal cadence, never below 15 minutes (a hiccup is not an outage).
    return Math.max(median * 3, 15 * 60 * 1000);
}

/** Split samples into contiguous runs; a run break is a coverage gap. */
export function splitByGaps(samples: QuotaSample[]): QuotaSample[][] {
    if (!samples.length) return [];
    const sorted = [...samples].sort((a, b) => a.time - b.time);
    const threshold = gapThreshold(sorted);
    const runs: QuotaSample[][] = [];
    let run: QuotaSample[] = [sorted[0]];
    for (let i = 1; i < sorted.length; i++) {
        if (sorted[i].time - sorted[i - 1].time > threshold) {
            runs.push(run);
            run = [];
        }
        run.push(sorted[i]);
    }
    runs.push(run);
    return runs;
}

/** Every day band carries a real gap (a missing day between populated days). */
export function hasInternalGap(bands: DayBand[]): boolean {
    return bands.some((b) => !b.hasData);
}
