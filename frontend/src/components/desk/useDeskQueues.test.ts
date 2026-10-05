import {act, cleanup, renderHook} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {useDeskQueues} from './useDeskQueues';

afterEach(() => {cleanup(); sessionStorage.clear(); vi.restoreAllMocks();});
describe('Desk queue recovery', () => {
    it('restores in-flight text as held without replaying its uncertain POST', () => {
        sessionStorage.setItem('desk.sessionQueues', JSON.stringify({a: {items: ['uncertain send'], inFlight: true}, b: {items: ['next turn']}}));
        const restored = renderHook(useDeskQueues);
        expect(restored.result.current[0]).toEqual({a: {items: ['uncertain send'], held: 'restored'}, b: {items: ['next turn'], held: 'restored'}});
    });
    it('ignores corrupt records and keeps usable text when storage is unavailable', () => {
        sessionStorage.setItem('desk.sessionQueues', JSON.stringify({bad: 2, a: {items: ['valid', 7, '']}}));
        const queue = renderHook(useDeskQueues);
        expect(queue.result.current[0]).toEqual({a: {items: ['valid'], held: 'restored'}});
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {throw new Error('full');});
        act(() => queue.result.current[1]((prev) => ({...prev, b: {items: ['in memory']}})));
        expect(queue.result.current[2].current.b.items).toEqual(['in memory']);
    });
});


it('keeps a route remount connected to the old in-flight send without losing new queued text', () => {
    const first = renderHook(useDeskQueues);
    act(() => first.result.current[1]({a: {items: ['sending'], inFlight: true}}));
    const finish = first.result.current[1];
    first.unmount();
    const next = renderHook(useDeskQueues);
    expect(next.result.current[0].a.inFlight).toBe(true);
    act(() => next.result.current[1]((previous) => ({...previous, a: {...previous.a, items: [...previous.a.items, 'new follow-up']}})));
    act(() => finish((previous) => ({...previous, a: {items: previous.a.items.slice(1), inFlight: false}})));
    expect(next.result.current[0].a.items).toEqual(['new follow-up']);
});
