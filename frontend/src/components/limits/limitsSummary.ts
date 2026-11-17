import {shortCount} from './amount';
import type {Limits} from './LimitsEditor';

type T = (key: string, options?: Record<string, unknown>) => string;


/** 3600 -> "1h", 18000 -> "5h", 604800 -> "7d"; anything odd falls back to seconds. */
export function formatWindow(seconds: number): string {
    if (seconds % 86400 === 0) return `${seconds / 86400}d`;
    if (seconds % 3600 === 0) return `${seconds / 3600}h`;
    if (seconds % 60 === 0) return `${seconds / 60}m`;
    return `${seconds}s`;
}

/** One human line per configured cap, e.g. ["2 concurrent", "200 requests / 1h", "steer away at 75%"]. */
export function summarizeLimits(l: Limits | undefined | null, t: T): string[] {
    if (!l) return [];
    const lines: string[] = [];
    const window = formatWindow(l.window_seconds || 3600);
    if (l.max_concurrent) lines.push(t('limits.summary.concurrent', {n: l.max_concurrent}));
    if (l.max_requests) lines.push(t('limits.summary.requests', {n: shortCount(l.max_requests), window}));
    if (l.max_tokens) lines.push(t('limits.summary.tokens', {n: shortCount(l.max_tokens), window}));
    if (l.soft_percent) lines.push(t('limits.summary.steer', {n: l.soft_percent}));
    if (l.quota_percent) lines.push(t('limits.summary.quota', {n: l.quota_percent}));
    return lines;
}

/** How many caps are configured (concurrency, requests, tokens, steer-away, quota stop). */
export function countLimits(l: Limits | undefined | null): number {
    if (!l) return 0;
    return [l.max_concurrent, l.max_requests, l.max_tokens, l.soft_percent, l.quota_percent].filter(Boolean).length;
}
