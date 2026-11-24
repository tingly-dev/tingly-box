import type { Usage } from '../../client';

/** One row of a usage table: who spent what, and the share of the card's total. */
export interface UsageEntry {
    /** Stable row key: the model name, or the thread id. */
    key: string;
    label: string;
    /** The model a thread ran on, shown under its title. */
    secondary?: string;
    /** Threads for a model row, turns for a thread row. */
    count: number;
    inputTokens: number;
    outputTokens: number;
    /** input + output; the share's numerator. */
    total: number;
    /** Share of the card's total, 0–100 with one decimal of precision. */
    share: number;
}

export interface UsageTotals {
    input: number;
    output: number;
    /** input + output across every thread. */
    total: number;
    /** Threads that reported usage. */
    threads: number;
    /** Turns across those threads. */
    turns: number;
}

const bySpent = (a: { total: number }, b: { total: number }): number => b.total - a.total;

/** Rows whose share is measured against their own card's total, biggest first. */
function withShares(rows: Omit<UsageEntry, 'share' | 'total'>[]): UsageEntry[] {
    const withTotals = rows.map((r) => ({ ...r, total: r.inputTokens + r.outputTokens }));
    const grand = withTotals.reduce((sum, r) => sum + r.total, 0);
    return withTotals
        .sort(bySpent)
        .map((r) => ({ ...r, share: grand > 0 ? Math.round((r.total / grand) * 1000) / 10 : 0 }));
}

/**
 * The project usage response reshaped for the tables: a row per model and per
 * thread, each with its share of its own card's total. Both cards rank by
 * tokens spent so the biggest spender leads; the totals strip is summed over
 * the thread rows, the threads themselves being what reported the usage.
 */
export function usageEntries(usage: Usage): { models: UsageEntry[]; threads: UsageEntry[]; totals: UsageTotals } {
    const models = withShares(usage.by_model.map((m) => ({
        key: m.model,
        label: m.model || '—',
        count: m.threads,
        inputTokens: m.input_tokens,
        outputTokens: m.output_tokens,
    })));
    const threads = withShares(usage.by_thread.map((t) => ({
        key: t.thread_id,
        label: t.title,
        secondary: t.model || undefined,
        count: t.turns,
        inputTokens: t.input_tokens,
        outputTokens: t.output_tokens,
    })));
    const input = threads.reduce((sum, t) => sum + t.inputTokens, 0);
    const output = threads.reduce((sum, t) => sum + t.outputTokens, 0);
    return { models, threads, totals: { input, output, total: input + output, threads: threads.length, turns: threads.reduce((sum, t) => sum + t.count, 0) } };
}
