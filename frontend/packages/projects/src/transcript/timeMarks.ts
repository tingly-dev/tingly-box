// Time markers: where a long conversation says when. A marker is placed
// between two blocks and only when time passed (a new day, a long silence,
// or a long unbroken run), never per message. The decision depends only on
// the blocks before a block, so new events appended at the end never move or
// remove a marker already shown. This is the one place that decides and
// formats them; every host of the transcript gets them from here.

import {serverNow} from '../utils/serverClock';

/** Silence between two blocks that earns a marker. */
export const GAP_MS = 60 * 60 * 1000;
/** A run with no gap that long still gets a marker this long after the last one. */
export const CHECKPOINT_MS = 3 * 60 * 60 * 1000;

export interface TimeMark {
    /** The block's own time (ISO), the marker's exact value. */
    at: string;
    /** The calendar day changed (or this is the first block): name the day. */
    day: boolean;
    /** Name the time of day too: after a silence or in a long run, not on a midnight crossed mid-flow. */
    clock: boolean;
}

const stamp = (iso: string | undefined): number => {
    if (!iso) return NaN;
    return Date.parse(iso);
};

// Calendar days are compared on the viewer's local date fields, not by
// dividing elapsed hours, so a 23- or 25-hour day around a DST change is
// still one day.
const dayNumber = (ms: number): number => {
    const d = new Date(ms);
    return Math.round(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()) / 86400000);
};

/**
 * timeMarks decides, for blocks in display order (`times[i]` is when block i
 * began, undefined when unknown), which of them a marker goes before. Blocks
 * with no valid time neither get one nor take part in the comparison.
 */
export function timeMarks(times: (string | undefined)[]): Map<number, TimeMark> {
    const marks = new Map<number, TimeMark>();
    let prev = NaN;
    let lastMark = NaN;
    times.forEach((iso, i) => {
        const at = stamp(iso);
        if (Number.isNaN(at) || !iso) return;
        if (Number.isNaN(prev)) {
            marks.set(i, {at: iso, day: true, clock: true});
            lastMark = at;
        } else {
            const newDay = dayNumber(at) !== dayNumber(prev);
            const silent = at - prev >= GAP_MS;
            const long = at - lastMark >= CHECKPOINT_MS;
            if (newDay || silent || long) {
                marks.set(i, {at: iso, day: newDay, clock: silent || long});
                lastMark = at;
            }
        }
        prev = at;
    });
    return marks;
}

// Intl rejects some tags browsers report (a POSIX "en-US@posix", say) and
// would take the page down with it; the runtime default stands in.
const usable = (locale: string): string | undefined => {
    try {
        return Intl.DateTimeFormat.supportedLocalesOf(locale).length > 0 ? locale : undefined;
    } catch {
        return undefined;
    }
};

const formatters = new Map<string, Intl.DateTimeFormat>();
const formatter = (locale: string, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat => {
    const key = `${locale}|${JSON.stringify(options)}`;
    let f = formatters.get(key);
    if (!f) {
        f = new Intl.DateTimeFormat(usable(locale), options);
        formatters.set(key, f);
    }
    return f;
};

const upperFirst = (text: string, locale: string): string => {
    if (!text) return text;
    const first = text.charAt(0);
    try {
        return first.toLocaleUpperCase(usable(locale)) + text.slice(1);
    } catch {
        return first.toUpperCase() + text.slice(1);
    }
};

/** The time of day in the locale's own style ("2:32 PM", "14:32"). */
export const clockLabel = (iso: string, locale: string): string => {
    const at = stamp(iso);
    return Number.isNaN(at) ? '' : formatter(locale, {hour: 'numeric', minute: '2-digit'}).format(at);
};

/** The exact moment, for a tooltip or a screen reader ("Tuesday, November 17, 2026 at 2:32 PM"). */
export const fullLabel = (iso: string, locale: string): string => {
    const at = stamp(iso);
    return Number.isNaN(at) ? '' : formatter(locale, {dateStyle: 'full', timeStyle: 'short'}).format(at);
};

/** The moment to the second, for the stamp a message shows on hover. */
export const exactLabel = (iso: string, locale: string): string => {
    const at = stamp(iso);
    return Number.isNaN(at) ? '' : formatter(locale, {dateStyle: 'medium', timeStyle: 'medium'}).format(at);
};

/**
 * The day in words: "Today" and "Yesterday" while recent, the date after
 * that (with its weekday; with its year only when it is not this year's).
 */
export const dayLabel = (iso: string, locale: string, now: number = serverNow()): string => {
    const at = stamp(iso);
    if (Number.isNaN(at)) return '';
    const ago = dayNumber(now) - dayNumber(at);
    if (ago === 0 || ago === 1) {
        let words: string;
        try {
            words = new Intl.RelativeTimeFormat(usable(locale), {numeric: 'auto'}).format(-ago, 'day');
        } catch {
            words = ago === 0 ? 'today' : 'yesterday';
        }
        return upperFirst(words, locale);
    }
    const sameYear = new Date(at).getFullYear() === new Date(now).getFullYear();
    return formatter(locale, sameYear
        ? {weekday: 'short', month: 'short', day: 'numeric'}
        : {year: 'numeric', month: 'short', day: 'numeric'}).format(at);
};

/** A marker's visible words: its day, its time of day, or both. */
export const markLabel = (mark: TimeMark, locale: string, now: number = serverNow()): string =>
    [mark.day ? dayLabel(mark.at, locale, now) : '', mark.clock ? clockLabel(mark.at, locale) : ''].filter(Boolean).join(' · ');

/** The next local midnight after `now`, in epoch milliseconds. */
export const nextMidnight = (now: number): number => {
    const d = new Date(now);
    return new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1).getTime();
};
