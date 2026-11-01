import { useRef, useState } from 'react';
import { api, unwrap, type Thread, type components } from '../client';
import type { SessionLaunch } from '../host/ProjectsHost';
import { useNotify } from './useNotify';

/** Keep this above placement branches: moving a panel must not lose a pending save. */
export function useThreadSettings(thread: Thread | undefined, folder: boolean, onChanged: (thread: Thread) => void) {
    const notify = useNotify();
    const pending = useRef(new Set<string>());
    const [savingIds, setSavingIds] = useState<Set<string>>(() => new Set());
    const save = async (change: (current: Thread) => Promise<Thread>) => {
        if (!thread || pending.current.has(thread.id)) return;
        const id = thread.id;
        pending.current.add(id);
        setSavingIds(new Set(pending.current));
        try {
            await change(thread);
        } catch (e) {
            notify((e as Error).message, 'error');
        } finally {
            pending.current.delete(id);
            setSavingIds(new Set(pending.current));
        }
    };
    const changed = (next: Thread) => {
        // The callback must merge by response id, never into the current selection.
        onChanged(next);
        return next;
    };
    const setLaunch = (next: SessionLaunch) => save(async (current) => {
        const path = { params: { path: { id: current.id } } };
        let saved = current;
        if (folder && next.profile !== (saved.profile ?? '')) {
            saved = changed(await unwrap(api.PUT('/api/v1/threads/{id}/profile', { ...path, body: { profile: next.profile } })));
        }
        if (next.model !== (folder ? saved.model ?? '' : saved.model_override ?? '')) {
            saved = changed(await unwrap(api.PUT('/api/v1/threads/{id}/model', { ...path, body: { model: next.model } })));
        }
        return saved;
    });
    const setWorkerLaunch = (profile: string | null, model: string) => save(async (current) => {
        const path = { params: { path: { id: current.id } } };
        let saved = current;
        if (profile !== (saved.profile_override ?? null)) saved = changed(await unwrap(api.PUT('/api/v1/threads/{id}/profile', { ...path, body: { profile: profile ?? '', inherit: profile === null } })));
        if (model !== (saved.model_override ?? '')) saved = changed(await unwrap(api.PUT('/api/v1/threads/{id}/model', { ...path, body: { model } })));
        return saved;
    });
    const setPermission = (permission_mode: string) => save(async (current) => changed(await unwrap(api.PUT('/api/v1/threads/{id}/permission-mode', {
        params: { path: { id: current.id } }, body: { permission_mode },
    }))));
    const setEffort = (effort: components['schemas']['EffortRequest']['effort']) => save(async (current) => changed(await unwrap(api.PUT('/api/v1/threads/{id}/effort', {
        params: { path: { id: current.id } }, body: { effort },
    }))));
    return {
        saving: !!thread && savingIds.has(thread.id),
        // Also checked inside onSend, before React has rendered the disabled button.
        canSend: () => !!thread && !pending.current.has(thread.id),
        setLaunch,
        setWorkerLaunch,
        setPermission,
        setEffort,
    };
}

/** A settings response must not roll back a newer streamed status or usage update. */
export function mergeThreadSettings(current: Thread, saved: Thread, folder: boolean): Thread {
    return { ...current, ...(folder && { model: saved.model }), profile_override: saved.profile_override, model_override: saved.model_override, effort_override: saved.effort_override, profile: saved.profile, permission_mode: saved.permission_mode };
}
