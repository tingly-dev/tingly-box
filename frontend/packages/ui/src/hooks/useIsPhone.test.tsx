import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PHONE_QUERY } from '../theme/mobile';
import { useIsPhone } from './useIsPhone';

const stubMatchMedia = (matches: (query: string) => boolean) => {
    vi.stubGlobal('matchMedia', (query: string) => ({
        matches: matches(query), media: query, addEventListener: vi.fn(), removeEventListener: vi.fn(),
        addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(), onchange: null,
    }));
};
afterEach(() => vi.unstubAllGlobals());

describe('useIsPhone', () => {
    it('is true on a narrow touch screen, read on the first render', () => {
        stubMatchMedia((q) => q === PHONE_QUERY);
        expect(renderHook(() => useIsPhone()).result.current).toBe(true);
    });

    it('is false on a desktop window', () => {
        stubMatchMedia(() => false);
        expect(renderHook(() => useIsPhone()).result.current).toBe(false);
    });
});
