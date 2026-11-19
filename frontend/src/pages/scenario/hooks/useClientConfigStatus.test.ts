import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useClientConfigStatus } from './useClientConfigStatus';

const { controlApi } = vi.hoisted(() => ({ controlApi: vi.fn() }));
vi.mock('@/services/openapi', () => ({ controlApi }));

// A read whose answer the test releases by hand.
function deferred<T>() {
    let resolve!: (v: T) => void;
    const promise = new Promise<T>((r) => { resolve = r; });
    return { promise, resolve };
}

const read = (state: string) => ({ success: true, state, differences: [] });

describe('useClientConfigStatus', () => {
    beforeEach(() => controlApi.mockReset());

    it('keeps the newest read when an older one answers last', async () => {
        const older = deferred<unknown>();
        const newer = deferred<unknown>();
        controlApi.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);

        const { result } = renderHook(() => useClientConfigStatus('codex', []));
        // The page changed something and asks again while the first read is out.
        act(() => { void result.current.refresh(); });

        await act(async () => { newer.resolve(read('applied')); });
        await act(async () => { older.resolve(read('outdated')); });

        expect(result.current.status?.state).toBe('applied');
    });

    it('does not show another tool\'s status while the new tool is read', async () => {
        const first = deferred<unknown>();
        const second = deferred<unknown>();
        controlApi.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

        const { result, rerender } = renderHook(
            ({ tool }: { tool: 'claude' | 'codex' }) => useClientConfigStatus(tool, [tool]),
            { initialProps: { tool: 'claude' as 'claude' | 'codex' } },
        );
        await act(async () => { first.resolve(read('outdated')); });
        expect(result.current.status?.state).toBe('outdated');

        rerender({ tool: 'codex' });
        expect(result.current.status).toBeNull();

        await act(async () => { second.resolve(read('applied')); });
        expect(result.current.status?.state).toBe('applied');
    });
});
