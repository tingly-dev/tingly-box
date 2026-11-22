/** Compact, locale-neutral duration: 90 → "1h 30m", 86400 → "1d". Seconds in, seconds out of range round to the nearest minute. */
export function formatDurationShort(seconds: number): string {
    if (seconds < 60) return `${seconds}s`;
    const minutes = Math.round(seconds / 60);
    const d = Math.floor(minutes / 1440);
    const h = Math.floor((minutes % 1440) / 60);
    const m = minutes % 60;
    return [d && `${d}d`, h && `${h}h`, m && `${m}m`].filter(Boolean).join(' ');
}
