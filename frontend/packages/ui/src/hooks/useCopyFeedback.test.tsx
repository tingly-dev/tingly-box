import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useCopyFeedback } from './useCopyFeedback';

const writeText = vi.fn();

beforeEach(() => {
    vi.useFakeTimers();
    writeText.mockReset();
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    vi.stubGlobal('isSecureContext', true);
});
afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
});

describe('useCopyFeedback', () => {
    it('flags copied after a successful write, calls onCopied, and resets after the delay', async () => {
        writeText.mockResolvedValue(undefined);
        const onCopied = vi.fn();
        const { result } = renderHook(() => useCopyFeedback(1000));
        await act(async () => { await result.current.copy('hello', onCopied); });
        expect(writeText).toHaveBeenCalledWith('hello');
        expect(result.current.copied).toBe(true);
        expect(onCopied).toHaveBeenCalledOnce();
        act(() => { vi.advanceTimersByTime(1000); });
        expect(result.current.copied).toBe(false);
    });

    it('restarts the timer when copying again before it ran out', async () => {
        writeText.mockResolvedValue(undefined);
        const { result } = renderHook(() => useCopyFeedback(1000));
        await act(async () => { await result.current.copy('a'); });
        act(() => { vi.advanceTimersByTime(800); });
        await act(async () => { await result.current.copy('b'); });
        act(() => { vi.advanceTimersByTime(800); });
        expect(result.current.copied).toBe(true);
        act(() => { vi.advanceTimersByTime(200); });
        expect(result.current.copied).toBe(false);
    });

    it('reset() clears the flag early', async () => {
        writeText.mockResolvedValue(undefined);
        const { result } = renderHook(() => useCopyFeedback());
        await act(async () => { await result.current.copy('x'); });
        act(() => result.current.reset());
        expect(result.current.copied).toBe(false);
    });

    it('does not flag copied or call onCopied when the clipboard write is refused', async () => {
        writeText.mockRejectedValue(new Error('denied'));
        const onCopied = vi.fn();
        const { result } = renderHook(() => useCopyFeedback());
        await act(async () => { await result.current.copy('x', onCopied); });
        expect(result.current.copied).toBe(false);
        expect(onCopied).not.toHaveBeenCalled();
    });
});
