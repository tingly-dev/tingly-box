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
