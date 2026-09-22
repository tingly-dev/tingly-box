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
