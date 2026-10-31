import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ThreadEvent } from '../client';
import { forgetTranscripts, mergeEvents, TAIL, useTranscript } from './useTranscript';

const m = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('../client', async (actual) => ({ ...(await actual<typeof import('../client')>()), api: { GET: m.get } }));

const ev = (id: string, minute: number, thread = 't1', content = id): ThreadEvent => {
    const at = new Date(Date.UTC(2026, 9, 21, 0, minute)).toISOString();
    return { id, thread_id: thread, kind: 'user', content, created_at: at, updated_at: at };
};
const answer = (events: ThreadEvent[], more = false) => ({ data: { events, more }, response: new Response() });
const queries = () => m.get.mock.calls.map(([, o]) => (o as { params: { query: Record<string, unknown> } }).params.query);

beforeEach(() => { m.get.mockReset(); });
afterEach(() => forgetTranscripts());

describe('mergeEvents', () => {
    it('replaces by id, adds new ones in created order, and never adds one older than the window', () => {
        const list = [ev('b', 2), ev('c', 3)];
        const out = mergeEvents(list, [ev('a', 1), { ...ev('c', 3), content: 'changed' }, ev('d', 4), ev('bb', 2.5)]);
        expect(out.map((e) => e.id)).toEqual(['b', 'bb', 'c', 'd']);
        expect(out[2].content).toBe('changed');
    });
});

describe('useTranscript', () => {
    it('opens on the latest page, loads earlier ones on request, and on return reads only what changed', async () => {
        m.get.mockResolvedValueOnce(answer([ev('c', 3), ev('d', 4)], true));
        const { result, rerender } = renderHook(({ id }) => useTranscript(id), { initialProps: { id: 't1' } });
        await waitFor(() => expect(result.current.events?.map((e) => e.id)).toEqual(['c', 'd']));
        expect(result.current.more).toBe(true);
        expect(queries()[0]).toEqual({ limit: TAIL });

        m.get.mockResolvedValueOnce(answer([ev('a', 1), ev('b', 2)], false));
        await act(() => result.current.loadEarlier());
        expect(result.current.events?.map((e) => e.id)).toEqual(['a', 'b', 'c', 'd']);
        expect(result.current.more).toBe(false);
        expect(queries()[1]).toEqual({ limit: TAIL, before: 'c' });

        // Away and back: the cached transcript shows at once, then only what changed is read.
        m.get.mockResolvedValueOnce(answer([], false));
        rerender({ id: 't2' });
        await waitFor(() => expect(m.get).toHaveBeenCalledTimes(3));
        m.get.mockResolvedValueOnce(answer([{ ...ev('b', 2), content: 'answered', updated_at: ev('x', 9).created_at }, ev('e', 5)]));
        rerender({ id: 't1' });
        expect(result.current.events?.map((e) => e.id)).toEqual(['a', 'b', 'c', 'd']);
        await waitFor(() => expect(result.current.events?.map((e) => e.id)).toEqual(['a', 'b', 'c', 'd', 'e']));
        expect(result.current.events?.[1].content).toBe('answered');
        expect(queries()[3]).toEqual({ since: ev('d', 4).updated_at });
    });

    it('keeps a live entry that arrives before the first read lands', async () => {
        let land: (v: unknown) => void = () => undefined;
        m.get.mockReturnValueOnce(new Promise((r) => (land = r)));
        const { result } = renderHook(() => useTranscript('t1'));
        act(() => result.current.apply(ev('live', 5)));
        await act(async () => land(answer([ev('a', 1)])));
        expect(result.current.events?.map((e) => e.id)).toEqual(['a', 'live']);
    });
    it('shares an in-flight catch-up and retries after failure', async () => {
        let reject!: (reason: Error) => void;
        m.get.mockReturnValueOnce(new Promise((_, r) => { reject = r; }));
        const { result } = renderHook(() => useTranscript('t1'));
        const first = result.current.catchUp();
        expect(result.current.catchUp()).toBe(first);
        expect(m.get).toHaveBeenCalledTimes(1);
        await act(async () => { reject(new Error('offline')); await expect(first).rejects.toThrow('offline'); });
        m.get.mockResolvedValueOnce(answer([ev('a', 1)]));
        await act(() => result.current.catchUp());
        expect(result.current.events?.map((e) => e.id)).toEqual(['a']);
        expect(m.get).toHaveBeenCalledTimes(2);
    });

    it('discards old responses after switching A to B to A and retains each thread’s early events', async () => {
        const land: Array<(value: unknown) => void> = [];
        m.get.mockImplementation(() => new Promise((resolve) => land.push(resolve)));
        const { result, rerender } = renderHook(({ id }) => useTranscript(id), { initialProps: { id: 'a' } });
        act(() => result.current.apply(ev('early-a', 2, 'a')));
        rerender({ id: 'b' });
        act(() => result.current.apply(ev('early-b', 2, 'b')));
        rerender({ id: 'a' });
        await act(async () => land[2](answer([ev('c', 1, 'a')])));
        expect(result.current.events?.map((e) => e.id)).toEqual(['c', 'early-a']);
        await act(async () => land[0](answer([ev('stale', 3, 'a')])));
        expect(result.current.events?.map((e) => e.id)).toEqual(['c', 'early-a']);
        rerender({ id: 'b' });
        await act(async () => land[3](answer([ev('b', 1, 'b')])));
        expect(result.current.events?.map((e) => e.id)).toEqual(['b', 'early-b']);
        await act(async () => land[1](answer([ev('stale-b', 3, 'b')])));
        expect(result.current.events?.map((e) => e.id)).toEqual(['b', 'early-b']);
    });

    it('pages the conversation from its own first event, not a usage event before it, and reads a subagent\'s steps on request', async () => {
        const usage = { ...ev('u0', 0), kind: 'usage' };
        m.get.mockResolvedValueOnce(answer([usage, ev('c', 3), { ...ev('call', 4), kind: 'tool_use', request_id: 'a1' }], true));
        const { result } = renderHook(() => useTranscript('t1'));
        await waitFor(() => expect(result.current.events).toHaveLength(3));

        m.get.mockResolvedValueOnce(answer([usage, ev('b', 2)], false));
        await act(() => result.current.loadEarlier());
        expect(queries()[1]).toEqual({ limit: TAIL, before: 'c' });
        expect(result.current.events?.map((e) => e.id)).toEqual(['u0', 'b', 'c', 'call']);

        expect(result.current.subagents.steps('a1')).toEqual({ loaded: false });
        m.get.mockResolvedValueOnce(answer([{ ...ev('s2', 6), parent: 'a1' }], true));
        await act(() => result.current.subagents.load('a1'));
        expect(queries()[2]).toEqual({ parent: 'a1', limit: TAIL });
        expect(result.current.subagents.steps('a1')).toEqual({ loaded: true, more: true });
        m.get.mockResolvedValueOnce(answer([{ ...ev('s1', 5), parent: 'a1' }], false));
        await act(() => result.current.subagents.load('a1', true));
        expect(queries()[3]).toEqual({ parent: 'a1', limit: TAIL, before: 's2' });
        expect(result.current.events?.map((e) => e.id)).toEqual(['u0', 'b', 'c', 'call', 's1', 's2']);
        expect(result.current.subagents.steps('a1')).toEqual({ loaded: true, more: false });
    });

    it('loads the conversation back to a call in one read, finding the call first when its event is not known', async () => {
        m.get.mockResolvedValueOnce(answer([ev('d', 4), ev('e', 5)], true));
        const { result } = renderHook(() => useTranscript('t1'));
        await waitFor(() => expect(result.current.events).not.toBeNull());

        const call = { ...ev('b', 2), kind: 'tool_use', content: 'Agent', request_id: 'a1' };
        m.get.mockResolvedValueOnce(answer([call]));
        m.get.mockResolvedValueOnce(answer([call, ev('c', 3)], true));
        await act(() => result.current.subagents.locate('a1'));
        expect(queries().slice(1)).toEqual([{ call: 'a1' }, { from: 'b', before: 'd' }]);
        expect(result.current.events?.map((e) => e.id)).toEqual(['b', 'c', 'd', 'e']);
        expect(result.current.more).toBe(true);

        // Loaded now: nothing more to read.
        await act(() => result.current.subagents.locate('a1', 'b'));
        expect(m.get).toHaveBeenCalledTimes(3);
    });
});

