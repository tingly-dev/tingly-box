import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { usePageVisibility } from './usePageVisibility';

afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
});

describe('usePageVisibility', () => {
    it('uses the latest callback and threshold for an already hidden page', () => {
        vi.useFakeTimers();
        let hidden = false;
        vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden);
        const first = vi.fn();
        const second = vi.fn();
        const view = renderHook(({ callback, threshold }) => usePageVisibility(callback, threshold), {
            initialProps: { callback: first, threshold: 30_000 },
        });

        hidden = true;
        document.dispatchEvent(new Event('visibilitychange'));
        vi.advanceTimersByTime(5_000);
        view.rerender({ callback: second, threshold: 1_000 });
        hidden = false;
        document.dispatchEvent(new Event('visibilitychange'));

        expect(first).not.toHaveBeenCalled();
        expect(second).toHaveBeenCalledOnce();
        view.unmount();
    });
});
