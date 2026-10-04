import { useSyncExternalStore } from 'react';

// Durable copy of what the user builds on the image pages — profiles, kept
// images, prompt snippets — so a reload does not throw it away.
//
// IndexedDB for the same reason as the run history (utils/playgroundSession.ts):
// the values carry images, far past what localStorage holds. One record per
// item plus an ordered id list per collection, and a save writes only the
// items whose object changed — store items are replaced, never mutated, so
// identity says what changed. Typing in a profile's prompt then rewrites that
// one profile, not every profile's images.
//
// Best-effort like the history: no IndexedDB (or a private window refusing
// it) means in-memory behaviour, never an error in the user's face.
//
// A page-free module: the sidebar reaches it through the profile store.
//
// Browser-local until the backend store lands (.design/image-profile.md §4).

const DB_NAME = 'tingly-image-workspace';
const VERSION = 1;
const ORDER = 'order';

export type Collection = 'profiles' | 'assets' | 'snippets';
const COLLECTIONS: Collection[] = ['profiles', 'assets', 'snippets'];

interface Identified { id: string }

// One connection for the tab's lifetime, unlike the history's open-per-save:
// the pagehide flush below has to start its transaction synchronously.
let connection: IDBDatabase | null = null;
let opening: Promise<IDBDatabase | null> | null = null;
const openConnection = (): Promise<IDBDatabase | null> => new Promise((resolve) => {
    try {
        if (typeof indexedDB === 'undefined') {
            resolve(null);
            return;
        }
        const request = indexedDB.open(DB_NAME, VERSION);
        request.onupgradeneeded = () => {
            for (const name of [...COLLECTIONS, ORDER]) {
                if (!request.result.objectStoreNames.contains(name)) request.result.createObjectStore(name);
            }
        };
        request.onsuccess = () => {
            // A newer version opened elsewhere: step aside, reopen on next use.
            request.result.onversionchange = () => {
                request.result.close();
                connection = null;
                opening = null;
            };
            connection = request.result;
            resolve(request.result);
        };
        request.onerror = () => resolve(null);
        request.onblocked = () => resolve(null);
    } catch {
        resolve(null);
    }
});

const openDb = (): Promise<IDBDatabase | null> => {
    if (connection) return Promise.resolve(connection);
    opening ??= openConnection();
    return opening;
};

const read = <T>(db: IDBDatabase, store: string, key: string): Promise<T | undefined> => new Promise((resolve) => {
    try {
        const request = db.transaction(store, 'readonly').objectStore(store).get(key);
        request.onsuccess = () => resolve(request.result as T | undefined);
        request.onerror = () => resolve(undefined);
    } catch {
        resolve(undefined);
    }
});

// What each collection's store holds right now, by id, as the very objects
// written. `null` = unknown: the next save rewrites the collection whole.
const stored: Record<Collection, Map<string, unknown> | null> = { profiles: null, assets: null, snippets: null };

/** The collection as the last session left it, in order; empty when nothing was kept. */
export const loadCollection = async <T extends Identified>(name: Collection): Promise<T[]> => {
    const db = await openDb();
    if (!db) return [];
    const ids = await read<string[]>(db, ORDER, name);
    if (!Array.isArray(ids)) return [];
    const results: Array<T | undefined> = await Promise.all(ids.map((id) => read<T>(db, name, String(id))));
    const items = results.filter((item): item is T => item !== undefined);
    // A listed record that did not read back: the store is not what the
    // list says, so leave it unknown and let the next save rewrite it.
    stored[name] = items.length === ids.length ? new Map(items.map((item) => [item.id, item])) : null;
    return items;
};

// One collection, one transaction: changed items put, removed ones deleted,
// the order rewritten. Resolves with whether it committed, so a failed write
// leaves `stored` as it was and the next save retries from there.
const write = (db: IDBDatabase, name: Collection, items: Identified[]): Promise<boolean> => new Promise((resolve) => {
    try {
        const known = stored[name];
        const transaction = db.transaction([name, ORDER], 'readwrite');
        const store = transaction.objectStore(name);
        if (!known) store.clear();
        const ids = new Set<string>();
        for (const item of items) {
            ids.add(item.id);
            if (known?.get(item.id) !== item) store.put(item, item.id);
        }
        known?.forEach((_, id) => {
            if (!ids.has(id)) store.delete(id);
        });
        transaction.objectStore(ORDER).put([...ids], name);
        // Nothing more is coming: commit now rather than when the task ends,
        // which on a page being unloaded may be never.
        transaction.commit?.();
        transaction.oncomplete = () => resolve(true);
        transaction.onerror = () => resolve(false);
        transaction.onabort = () => resolve(false);
    } catch {
        resolve(false);
    }
});

// Saves coalesce per collection: a burst of edits (one per keystroke in a
// prompt) becomes one transaction after the burst, carrying the newest list.
const SAVE_DELAY_MS = 250;
const pending: Partial<Record<Collection, Identified[]>> = {};
let timer: ReturnType<typeof setTimeout> | null = null;
let flushing: Promise<void> | null = null;

// Starts one transaction per pending collection, synchronously.
const writePending = (db: IDBDatabase): Promise<unknown> => {
    const writes = COLLECTIONS.map((name) => {
        const items = pending[name];
        if (!items) return null;
        delete pending[name];
        return write(db, name, items).then((committed) => {
            if (committed) stored[name] = new Map(items.map((item) => [item.id, item]));
        });
    });
    return Promise.all(writes);
};

const flush = async (): Promise<void> => {
    while (Object.keys(pending).length > 0) {
        const db = await openDb();
        if (!db) return;
        await writePending(db);
    }
};

const startFlush = () => {
    timer = null;
    if (!flushing) flushing = flush().finally(() => { flushing = null; });
};

export const saveCollection = <T extends Identified>(name: Collection, items: T[]) => {
    pending[name] = items;
    if (timer === null) timer = setTimeout(startFlush, SAVE_DELAY_MS);
};

// A reload or a closed tab inside the delay would drop the last edit; start
// the write now instead. Best effort: Chrome often abandons a transaction
// begun while the page unloads, so the delay stays short.
if (typeof window !== 'undefined') {
    window.addEventListener('pagehide', () => {
        if (timer === null) return;
        clearTimeout(timer);
        timer = null;
        if (connection) void writePending(connection);
        else startFlush();
    });
}

// Mock mode (`pnpm dev:mock`) shows seeded examples instead and keeps nothing:
// automation and screenshots get the same page every time, and the seeds
// never reach — or ship to — a real user. Call sites pass the seed as
// `USE_MOCK ? () => import(...) : null` so a production build drops it.
export const USE_MOCK = import.meta.env.VITE_USE_MOCK === 'true';

export interface WorkspaceCollection<T extends Identified> {
    get: () => T[];
    set: (next: T[]) => void;
    useItems: () => T[];
    // False until the stored items are in: a page reached by reload should not
    // say "not found" / "nothing kept yet" about items still being read.
    useReady: () => boolean;
}

// A module-level store over one collection: starts empty, fills from
// IndexedDB (or the mock seed), and writes every change back.
export const createWorkspaceCollection = <T extends Identified>(
    name: Collection,
    seed: (() => Promise<T[]>) | null,
): WorkspaceCollection<T> => {
    let items: T[] = [];
    let ready = false;
    const listeners = new Set<() => void>();
    const emit = () => listeners.forEach((listener) => listener());
    const subscribe = (listener: () => void) => {
        listeners.add(listener);
        return () => { listeners.delete(listener); };
    };
    const persist = seed === null;

    const hydrate = (loaded: T[]) => {
        // Anything created before the read finished stays, after what was stored.
        const loadedIds = new Set(loaded.map((item) => item.id));
        const local = items.filter((item) => !loadedIds.has(item.id));
        items = [...loaded, ...local];
        ready = true;
        if (persist && local.length > 0) saveCollection(name, items);
        emit();
    };
    void (seed ?? (() => loadCollection<T>(name)))().then(hydrate, () => hydrate([]));

    return {
        get: () => items,
        set: (next) => {
            items = next;
            // Saving before the read lands would overwrite the stored list
            // with a partial one; hydrate() saves the merge instead.
            if (persist && ready) saveCollection(name, items);
            emit();
        },
        useItems: () => useSyncExternalStore(subscribe, () => items, () => items),
        useReady: () => useSyncExternalStore(subscribe, () => ready, () => ready),
    };
};
