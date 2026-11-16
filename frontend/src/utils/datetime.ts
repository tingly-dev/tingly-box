// Format date to local ISO string (with timezone offset).
// Backend stores local time, so we send local time with timezone offset.
export const toLocalISOString = (date: Date): string => {
    const tzOffset = -date.getTimezoneOffset();
    const sign = tzOffset >= 0 ? '+' : '-';
    const pad = (n: number) => String(Math.floor(Math.abs(n))).padStart(2, '0');
    return (
        date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) +
        'T' + pad(date.getHours()) + ':' + pad(date.getMinutes()) + ':' + pad(date.getSeconds()) +
        sign + pad(tzOffset / 60) + ':' + pad(tzOffset % 60)
    );
};

// Create a Date at local midnight (00:00:00 local time).
export const getLocalMidnight = (date: Date): Date => {
    const d = new Date(date.getFullYear(), date.getMonth(), date.getDate());
    return d;
};

// Display formatter: local datetime string for a timestamp, falling back to
// the raw input when it can't be parsed.
export const formatTimestamp = (timestamp: string): string => {
    try {
        return new Date(timestamp).toLocaleString();
    } catch {
        return timestamp;
    }
};

// Log rows: today's entries show only the time ("3:09:59 AM"), older ones add
// a short date ("Oct 23, 3:09:59 AM"). Falls back to the raw input when it
// can't be parsed.
export const formatLogTime = (timestamp: string, now: Date = new Date()): string => {
    const date = new Date(timestamp);
    if (Number.isNaN(date.getTime())) return timestamp;
    const time = date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit', second: '2-digit' });
    if (date.toDateString() === now.toDateString()) return time;
    return `${date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}, ${time}`;
};
