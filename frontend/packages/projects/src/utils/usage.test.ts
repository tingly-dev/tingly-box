import { describe, expect, it } from 'vitest';
import { cacheHitPct, contextFill, formatTurnDuration } from './usage';

describe('contextFill', () => {
    it('is the share of the window the last call filled', () => {
        expect(contextFill({ context_tokens: 57500, context_window: 200000 })).toEqual({ used: 57500, window: 200000, pct: 29 });
    });
    it('keeps the tokens when the window is unknown, with no percentage', () => {
        expect(contextFill({ context_tokens: 4200, context_window: 0 })).toEqual({ used: 4200, window: 0, pct: undefined });
    });
    it('is nothing before a turn has reached the model', () => {
        expect(contextFill({ context_tokens: 0, context_window: 200000 })).toBeUndefined();
    });
    it('caps at 100 when the call overran the window', () => {
        expect(contextFill({ context_tokens: 230000, context_window: 200000 })?.pct).toBe(100);
    });
});

describe('cacheHitPct', () => {
    it('is the cached share of the prompt tokens sent', () => {
        expect(cacheHitPct({ input_tokens: 1000, cache_read_tokens: 750 })).toBe(75);
    });
    it('is nothing when no prompt was sent', () => {
        expect(cacheHitPct({ input_tokens: 0, cache_read_tokens: 0 })).toBeUndefined();
    });
});

describe('formatTurnDuration', () => {
    it('reads seconds, then minutes', () => {
        expect(formatTurnDuration(900)).toBe('0.9s');
        expect(formatTurnDuration(4200)).toBe('4.2s');
        expect(formatTurnDuration(95_000)).toBe('1m 35s');
    });
});
