import {useCallback, useMemo, useSyncExternalStore} from 'react';
import type {SetStateAction} from 'react';

interface Store {
    value: unknown;
    subscribe: (listener: () => void) => () => void;
    snapshot: () => unknown;
    update: (change: (previous: unknown) => unknown) => void;
}
const stores = new Map<string, Store>();

// One store per tab/key, including across route unmounts. A late acceptance
// must update the same state the newly mounted composer is editing.
function getStore(key: string, decode: (value: unknown) => unknown): Store {
    const existing = stores.get(key);
    if (existing) return existing;
    let raw: string | null | undefined;
    const listeners = new Set<() => void>();
    const decodeRaw = (text: string | null) => {
        try {return decode(JSON.parse(text ?? '{}'));} catch {return decode({});}
    };
    const store: Store = {
        value: decode({}),
        snapshot: () => {
            try {
                const saved = sessionStorage.getItem(key);
                if (saved !== raw) {
                    raw = saved;
                    store.value = decodeRaw(saved);
                }
            } catch { /* Keep the current in-memory state. */ }
            return store.value;
        },
        subscribe: (listener) => {
            listeners.add(listener);
            const changed = (event: StorageEvent) => {
                if (event.key !== null && event.key !== key) return;
                raw = undefined;
                store.snapshot();
                listeners.forEach((notify) => notify());
            };
            window.addEventListener('storage', changed);
            return () => {listeners.delete(listener); window.removeEventListener('storage', changed);};
        },
        update: (change) => {
            const next = change(store.snapshot());
            store.value = next;
            try {
                const encoded = JSON.stringify(next);
                sessionStorage.setItem(key, encoded);
                raw = encoded;
            } catch { /* Storage restrictions do not prevent work in this tab. */ }
            listeners.forEach((notify) => notify());
        },
    };
    stores.set(key, store);
    return store;
}

export function useDeskStorage<T>(key: string, decode: (value: unknown) => T) {
    const store = getStore(key, decode);
    const value = useSyncExternalStore(store.subscribe, store.snapshot) as T;
    const update = useCallback((change: SetStateAction<T>) => {
        store.update((previous) => typeof change === 'function' ? (change as (value: T) => T)(previous as T) : change);
    }, [store]);
    const current = useMemo(() => ({get current() {return store.snapshot() as T;}}), [store]);
    return [value, update, current] as const;
}
