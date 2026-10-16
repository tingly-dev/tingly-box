/** 12345 → "12.3k", 1500000 → "1.5M". Token counts are shown as concrete values. */
export function formatTokens(n: number): string {
    if (n >= 1_000_000) return `${trim(n / 1_000_000)}M`;
    if (n >= 1_000) return `${trim(n / 1_000)}k`;
    return String(n);
}

function trim(x: number): string {
    return x >= 100 ? String(Math.round(x)) : x.toFixed(1).replace(/\.0$/, '');
}

export function formatBytes(n: number): string {
    if (n >= 1 << 20) return `${trim(n / (1 << 20))} MB`;
    if (n >= 1 << 10) return `${trim(n / (1 << 10))} KB`;
    return `${n} B`;
}

/** Intl rejects some tags browsers report (a POSIX "en-US@posix", say) and
 * would take the page down with it; fall back to the runtime default. */
function relativeFormat(locale: string): Intl.RelativeTimeFormat {
    try {
        return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
    } catch {
        return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
    }
}

/** Relative time ("3 min ago") using the browser's locale. */
export function timeAgo(iso: string | undefined, locale: string): string {
    if (!iso) return '';
    const diff = (new Date(iso).getTime() - Date.now()) / 1000;
    const rtf = relativeFormat(locale);
    const abs = Math.abs(diff);
    if (abs < 60) return rtf.format(Math.round(diff), 'second');
    if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute');
    if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour');
    if (abs < 86400 * 30) return rtf.format(Math.round(diff / 86400), 'day');
    if (abs < 86400 * 365) return rtf.format(Math.round(diff / (86400 * 30)), 'month');
    return rtf.format(Math.round(diff / (86400 * 365)), 'year');
}

/** The last part of a folder path (either separator), or the path itself for a root. */
export function folderName(path: string): string {
    return path.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || path;
}
