/**
 * Token budgets are big numbers; typing "2M" beats counting zeros. Accepts a
 * plain count or a decimal with a K / M suffix (any case), and ignores spaces,
 * commas and underscores used as digit separators.
 */
const AMOUNT = /^(\d+(?:\.\d*)?|\.\d+)([km])?$/;
const FACTOR = {k: 1e3, m: 1e6} as const;

/** Result of reading what the user typed: `empty` means "no limit", `invalid` keeps the previous value. */
export type ParsedAmount = {kind: 'empty'} | {kind: 'invalid'} | {kind: 'ok'; value: number};

export function parseAmount(raw: string): ParsedAmount {
    const s = raw.trim().toLowerCase().replace(/[\s,_]/g, '');
    if (s === '') return {kind: 'empty'};
    const m = AMOUNT.exec(s);
    if (!m) return {kind: 'invalid'};
    const value = Math.round(Number(m[1]) * (m[2] ? FACTOR[m[2] as 'k' | 'm'] : 1));
    return value > 0 ? {kind: 'ok', value} : {kind: 'empty'};
}

/** 2000000 -> "2M", 1500000 -> "1.5M", 500000 -> "500K"; anything not round stays exact. */
export function formatAmount(n?: number): string {
    if (!n || n <= 0) return '';
    if (n >= 1e6 && n % 1e5 === 0) return `${n / 1e6}M`;
    if (n % 1e3 === 0) return `${n / 1e3}K`;
    return String(n);
}

const oneDecimal = (v: number) => String(Math.round(v * 10) / 10);

/**
 * The display form of a count, the same K / M the field accepts: 1700000 ->
 * "1.7M", 200000 -> "200K", 950 -> "950". Locale-independent on purpose, so what
 * is shown can be typed back as is (Intl's compact form says "170万" in Chinese).
 */
export function shortCount(n: number): string {
    if (!Number.isFinite(n) || n < 1e3) return String(Math.max(0, Math.round(n || 0)));
    if (n >= 1e6) return `${oneDecimal(n / 1e6)}M`;
    const k = Math.round(n / 100) / 10;
    return k >= 1000 ? '1M' : `${oneDecimal(k)}K`;
}
