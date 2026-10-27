import type { CodexWindow } from '@/types/model';

// Mirrors the Codex catalog rules in internal/config/apply_config_codex.go
// (codexEffectiveContextWindowPercent,
// codexDefaultAutoCompactTokenLimitPct, the 200K / 1M windows). The server is
// the source of truth for what is written; this only previews it while a
// slider is dragged, so keep the numbers in sync.

export const CODEX_COMPACT = {
    min: 50,
    max: 100,
    defaultPercent: 75,
    defaultEffectivePercent: 82,
    window: 200_000,
    window1M: 1_000_000,
    // Below this many tokens left at compaction time there is too little room
    // for the summary request, system prompt, tool output and the reply. It is
    // the practical limit seen in use: 82% of a 200K window. Not a hard cap,
    // only where warnings start; larger windows reach it at a higher percent.
    minRemaining: 36_000,
} as const;

export type CodexCompactStatus = 'ok' | 'low' | 'limited';

export interface CodexCompactPlan {
    /** The percent that ends up in the catalog. */
    applied: number;
    /** Tokens at which Codex compacts. */
    line: number;
    /** Tokens still free when compaction fires. */
    remain: number;
    status: CodexCompactStatus;
}

/**
 * Resolves what a chosen percent turns into. `limited` = it exceeds the usable
 * window so it is cut down, `low` = it leaves too few tokens.
 */
export function planCodexCompact(opts: {
    window: number;
    percent: number;
    effectivePercent?: number;
}): CodexCompactPlan {
    const effective = opts.effectivePercent ?? CODEX_COMPACT.defaultEffectivePercent;
    const applied = Math.min(opts.percent, effective);
    const line = Math.floor((opts.window * applied) / 100);
    const remain = opts.window - line;
    let status: CodexCompactStatus = 'ok';
    if (opts.percent > applied) status = 'limited';
    else if (remain < CODEX_COMPACT.minRemaining) status = 'low';
    return { applied, line, remain, status };
}

/** Position (0–100) of `percent` on the slider track, which spans min–100. */
export function trackPosition(percent: number): number {
    const clamped = Math.min(Math.max(percent, CODEX_COMPACT.min), 100);
    return ((clamped - CODEX_COMPACT.min) / (100 - CODEX_COMPACT.min)) * 100;
}

/** Where the "too few tokens left" zone starts, or null when it lies beyond the track. */
export function lowZoneStart(window: number): number | null {
    const start = (1 - CODEX_COMPACT.minRemaining / window) * 100;
    return start < CODEX_COMPACT.max ? start : null;
}

export interface CodexCompactZones {
    /** Values in [from, to) leave too few tokens free; null when the range is empty. */
    low: { from: number; to: number } | null;
    /** Values at or above this are cut down to it (the usable window); null at 100. */
    cappedFrom: number | null;
}

/**
 * The stretches of the slider that need a warning: the "too few tokens left"
 * zone (only the part below the cap, since anything above is cut down to the
 * cap anyway) and the "capped by the usable window" zone.
 */
export function compactZones(window: number, effectivePercent: number): CodexCompactZones {
    const cap = Math.min(Math.max(effectivePercent, CODEX_COMPACT.min), 100);
    const lowStart = lowZoneStart(window);
    const lowFrom = lowStart === null ? null : Math.max(lowStart, CODEX_COMPACT.min);
    return {
        low: lowFrom !== null && lowFrom < cap ? { from: lowFrom, to: cap } : null,
        cappedFrom: cap < 100 ? cap : null,
    };
}

// Mirrors codexMinContextWindow / codexMaxContextWindow
// (internal/config/apply_config_codex.go).
export const CODEX_CONTEXT = {
    min: 8_000,
    max: 10_000_000,
    // Common real windows, for picking without typing.
    presets: [128_000, 200_000, 256_000, 400_000, 1_000_000],
} as const;

/**
 * Where the window Codex is told about comes from: the rule's own value
 * (`rule`), else what the rule's "Auto" gives on the server, as Codex's model
 * list says (CodexWindow.source): the routed model's input limit (`catalog`),
 * the larger window the 1M flag opens (`extended`), or a guess for a model
 * the catalog doesn't know (`default`).
 */
export type CodexWindowSource = 'rule' | CodexWindow['source'];

export interface CodexResolvedWindow {
    window: number;
    /** The most Codex may be told to use (max_context_window), at least window. */
    max: number;
    source: CodexWindowSource;
}

/**
 * What Auto writes for a model it doesn't know: 200K, or 1M with the 1M flag.
 * Shown until Codex's model list says otherwise.
 */
export function codexGuessWindow(context1M: boolean): CodexResolvedWindow {
    const window = context1M ? CODEX_COMPACT.window1M : CODEX_COMPACT.window;
    return { window, max: window, source: 'default' };
}

/**
 * The window Codex is told about: the rule's own value when in range (also its
 * ceiling), else Auto's. Mirrors CodexContextWindowFor in
 * internal/config/apply_config_codex.go.
 */
export function resolveCodexWindow(manual: number, auto: CodexResolvedWindow): CodexResolvedWindow {
    if (manual >= CODEX_CONTEXT.min && manual <= CODEX_CONTEXT.max) return { window: manual, max: manual, source: 'rule' };
    return auto;
}

/** 200000 → "200K", 1000000 → "1M", 1500000 → "1.5M", 131072 → "131K". */
export function formatTokens(n: number): string {
    if (n >= 1_000_000) return `${Number((n / 1_000_000).toFixed(2))}M`;
    if (n >= 1_000) return `${Math.round(n / 1_000)}K`;
    return String(n);
}

/**
 * Reads what a person types for a window: "256k", "1m", "1.5M", "131072",
 * "200,000". Returns null when it is not a number in range.
 */
export function parseTokens(raw: string): number | null {
    const m = raw.trim().replace(/[,_\s]/g, '').match(/^(\d+(?:\.\d+)?)([km]?)$/i);
    if (!m) return null;
    const unit = m[2].toLowerCase();
    const n = Math.round(Number(m[1]) * (unit === 'm' ? 1_000_000 : unit === 'k' ? 1_000 : 1));
    return n >= CODEX_CONTEXT.min && n <= CODEX_CONTEXT.max ? n : null;
}
