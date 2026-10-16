// A Claude Code session's token use, totalled from its transcript's "usage"
// entries (one per turn, written by agentboot/engine). Read by the status
// line under a session's message box.

// TurnUsage is the payload of a "usage" transcript entry, written by the
// engine once per turn.
export interface TurnUsage {
    model?: string;
    input_tokens: number;
    output_tokens: number;
    cache_read_tokens: number;
    cache_write_tokens: number;
    context_tokens: number;
    context_window?: number;
    duration_ms?: number;
}

export interface SessionUsage {
    input: number;
    output: number;
    cacheRead: number;
    cacheWrite: number;
    // The latest turn: its model and how full its context was.
    latest: TurnUsage;
}

// sessionUsage totals a session's "usage" entries; undefined before any
// turn has reached the model.
export const sessionUsage = (entries: {kind?: string; payload?: unknown}[]): SessionUsage | undefined => {
    let total: SessionUsage | undefined;
    for (const m of entries) {
        if (m.kind !== 'usage' || m.payload == null) continue;
        const u = m.payload as TurnUsage;
        total = {
            input: (total?.input ?? 0) + (u.input_tokens ?? 0),
            output: (total?.output ?? 0) + (u.output_tokens ?? 0),
            cacheRead: (total?.cacheRead ?? 0) + (u.cache_read_tokens ?? 0),
            cacheWrite: (total?.cacheWrite ?? 0) + (u.cache_write_tokens ?? 0),
            latest: u,
        };
    }
    return total;
};

// Share of prompt tokens served from the cache, the status line's "cache" figure.
export const cacheHitPct = (u: SessionUsage): number => {
    const prompt = u.input + u.cacheRead + u.cacheWrite;
    return prompt > 0 ? Math.round((u.cacheRead / prompt) * 100) : 0;
};
