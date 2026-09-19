// A small, shared "how long ago" formatter for a timestamp shown as
// relative time (a session's last activity, a fetch's age, ...) instead of
// a full date. `now` is a parameter (not `Date.now()` inline) so callers —
// and tests — can pin it.
export const timeAgo = (iso: string, now: number = Date.now()): string => {
    const t = new Date(iso).getTime();
    if (Number.isNaN(t)) return '';
    const seconds = Math.max(0, Math.round((now - t) / 1000));
    if (seconds < 5) return 'just now';
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.round(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.round(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.round(hours / 24);
    return `${days}d ago`;
};
