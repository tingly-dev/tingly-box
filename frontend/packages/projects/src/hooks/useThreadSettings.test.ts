import { act, renderHook } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { useThreadSettings, mergeThreadSettings } from './useThreadSettings';
import type { Thread } from '../client';

const { put, notify } = vi.hoisted(() => ({ put: vi.fn(), notify: vi.fn() }));
vi.mock('../client', async (actual) => ({ ...await actual<typeof import('../client')>(), api: { PUT: put } }));
vi.mock('./useNotify', () => ({ useNotify: () => notify }));
const thread = (id: string, extra = {}): Thread => ({ id, project_id: 'p', model: 'observed', ...extra } as Thread);
const response = (th: Thread) => ({ data: th, response: new Response() });
beforeEach(() => vi.clearAllMocks());

it('guards sends synchronously, serializes controls and keeps pending saves scoped across selection changes', async () => {
    let finish!: (value: unknown) => void;
    put.mockReturnValue(new Promise((r) => { finish = r; }));
    const changed = vi.fn();
    const a = thread('a');
    const b = thread('b');
    const hook = renderHook(({ selected }) => useThreadSettings(selected, false, changed), { initialProps: { selected: a } });
    let save!: Promise<void>;
    act(() => {
        save = hook.result.current.setLaunch({ profile: '', model: 'requested' });
        expect(hook.result.current.canSend()).toBe(false);
        void hook.result.current.setPermission('plan');
    });
    expect(put).toHaveBeenCalledTimes(1);
    expect(put).toHaveBeenCalledWith('/api/v1/threads/{id}/model', { params: { path: { id: 'a' } }, body: { model: 'requested' } });
    hook.rerender({ selected: b });
    expect(hook.result.current.saving).toBe(false);
    expect(hook.result.current.canSend()).toBe(true);
    hook.rerender({ selected: a });
    expect(hook.result.current.saving).toBe(true);
    await act(async () => { finish(response(thread('a', { model_override: 'requested' }))); await save; });
    expect(changed).toHaveBeenCalledWith(expect.objectContaining({ id: 'a', model_override: 'requested' }));
    expect(hook.result.current.canSend()).toBe(true);
});

it('retains confirmed settings on failure and unlocks for retry', async () => {
    put.mockRejectedValueOnce(new Error('save failed'));
    const changed = vi.fn();
    const hook = renderHook(() => useThreadSettings(thread('a'), false, changed));
    await act(async () => { await hook.result.current.setPermission('plan'); });
    expect(changed).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledWith('save failed', 'error');
    expect(hook.result.current.saving).toBe(false);
});

it('saves folder profile before its model and keeps a successfully saved profile when model saving fails', async () => {
    put.mockResolvedValueOnce(response(thread('a', { profile: 'new', model: '' }))).mockRejectedValueOnce(new Error('bad model'));
    const changed = vi.fn();
    const hook = renderHook(() => useThreadSettings(thread('a', { profile: 'old', model: 'opus' }), true, changed));
    await act(async () => { await hook.result.current.setLaunch({ profile: 'new', model: 'sonnet' }); });
    expect(put.mock.calls.map((c) => c[0])).toEqual(['/api/v1/threads/{id}/profile', '/api/v1/threads/{id}/model']);
    expect(changed).toHaveBeenCalledOnce();
    expect(changed).toHaveBeenCalledWith(expect.objectContaining({ profile: 'new', model: '' }));
});

it('merges settings without rolling back streamed status, usage or observed worker models', () => {
    const current = thread('a', { status: 'ready', turns: 3, model: 'latest-observed' });
    const saved = thread('a', { status: 'working', turns: 2, model: 'old-observed', model_override: 'chosen', permission_mode: 'plan' });
    expect(mergeThreadSettings(current, saved, false)).toMatchObject({ status: 'ready', turns: 3, model: 'latest-observed', model_override: 'chosen', permission_mode: 'plan' });
});


it('saves effort per thread and preserves it when merging settings', async () => {
    const saved = thread('a', { effort_override: 'high' });
    put.mockResolvedValueOnce(response(saved));
    const changed = vi.fn();
    const hook = renderHook(() => useThreadSettings(thread('a'), false, changed));
    await act(async () => { await hook.result.current.setEffort('high'); });
    expect(put).toHaveBeenCalledWith('/api/v1/threads/{id}/effort', { params: { path: { id: 'a' } }, body: { effort: 'high' } });
    expect(mergeThreadSettings(thread('a'), saved, false).effort_override).toBe('high');
});
