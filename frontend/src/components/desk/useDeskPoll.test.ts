import {act, cleanup, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {useDeskPoll} from './useDeskPoll';

beforeEach(() => vi.useFakeTimers());
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe('Desk polling', () => {
    it('waits for slow refreshes and coalesces reconnect events', async () => {
        let finish!: () => void;
        const refresh = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
        const view = renderHook(() => useDeskPoll(refresh, 1500));
        await act(async () => vi.advanceTimersByTimeAsync(6000));
        expect(refresh).toHaveBeenCalledTimes(1);
        act(() => {
            window.dispatchEvent(new Event('online'));
            window.dispatchEvent(new Event('focus'));
        });
        expect(refresh).toHaveBeenCalledTimes(1);
        await act(async () => finish());
        await act(async () => vi.advanceTimersByTimeAsync(1));
        expect(refresh).toHaveBeenCalledTimes(2);
        view.unmount();
        await act(async () => finish());
        await act(async () => vi.advanceTimersByTimeAsync(6000));
        expect(refresh).toHaveBeenCalledTimes(2);
    });

    it('immediately refreshes an idle session on reconnect and uses the latest callback', async () => {
        const first = vi.fn().mockResolvedValue(undefined);
        const second = vi.fn().mockResolvedValue(undefined);
        const view = renderHook(({refresh}) => useDeskPoll(refresh, 5000), {initialProps: {refresh: first}});
        await act(async () => {});
        view.rerender({refresh: second});
        await act(async () => window.dispatchEvent(new Event('online')));
        expect(first).toHaveBeenCalledTimes(1);
        expect(second).toHaveBeenCalledTimes(1);
        await act(async () => vi.advanceTimersByTimeAsync(5000));
        expect(second).toHaveBeenCalledTimes(2);
    });

    it('removes timers and wake listeners when disabled', async () => {
        const refresh = vi.fn().mockResolvedValue(undefined);
        const view = renderHook(({enabled}) => useDeskPoll(refresh, 1500, enabled), {initialProps: {enabled: true}});
        await act(async () => {});
        view.rerender({enabled: false});
        await act(async () => {
            window.dispatchEvent(new Event('online'));
            await vi.advanceTimersByTimeAsync(5000);
        });
        expect(refresh).toHaveBeenCalledTimes(1);
    });

    it('aborts a stuck read after 15 seconds and retries, cancelling the next read on unmount', async () => {
        const signals: AbortSignal[] = [];
        const refresh = vi.fn((signal: AbortSignal) => new Promise<void>((resolve) => {
            signals.push(signal);
            signal.addEventListener('abort', () => resolve(), {once: true});
        }));
        const view = renderHook(() => useDeskPoll(refresh, 1500));
        await act(async () => vi.advanceTimersByTimeAsync(15_000));
        expect(signals[0].aborted).toBe(true);
        expect(signals[0].reason.name).toBe('TimeoutError');
        await act(async () => vi.advanceTimersByTimeAsync(1500));
        expect(refresh).toHaveBeenCalledTimes(2);
        view.unmount();
        expect(signals[1].aborted).toBe(true);
        expect(signals[1].reason.name).toBe('AbortError');
    });
});
