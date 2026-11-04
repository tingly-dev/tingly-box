import type { Thread } from '../client';

type ContextFields = Pick<Thread, 'context_tokens' | 'context_window'>;

/** How full the context was on the last call; `pct` is unknown when the model's window was not reported. */
export function contextFill(t: ContextFields): { used: number; window: number; pct: number | undefined } | undefined {
    if (!t.context_tokens) return undefined;
    const window = t.context_window || 0;
    return { used: t.context_tokens, window, pct: window > 0 ? Math.min(100, Math.round((t.context_tokens / window) * 100)) : undefined };
}

/** The share of prompt tokens served from the cache; `input_tokens` counts the cached ones too. */
export function cacheHitPct(t: Pick<Thread, 'input_tokens' | 'cache_read_tokens'>): number | undefined {
    return t.input_tokens > 0 ? Math.round((t.cache_read_tokens / t.input_tokens) * 100) : undefined;
}

export function formatTurnDuration(ms: number): string {
    if (ms < 60_000) return `${(ms / 1000).toFixed(1).replace(/\.0$/, '')}s`;
    const s = Math.round(ms / 1000);
    return `${Math.floor(s / 60)}m ${s % 60}s`;
}
