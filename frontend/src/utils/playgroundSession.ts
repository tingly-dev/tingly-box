// Durable copy of the image playground's session: what was generated and
// what was brought in, so a reload does not empty the results panel.
//
// IndexedDB rather than sessionStorage because the values are base64 images —
// a handful of 1024px PNGs already exceed the few-megabyte quota that
// sessionStorage enforces, and silently losing the newest run on quota is
// worse than never persisting. Everything here is best-effort: a browser
// without IndexedDB (or a private window that refuses it) gets the in-memory
// behaviour, never an error the user has to see.

const DB_NAME = 'tingly-image-playground';
const STORE = 'session';
const VERSION = 1;

export interface PlaygroundSessionSnapshot<Run, Import> {
    runs: Run[];
    imports: Import[];
}

const openDb = (): Promise<IDBDatabase | null> => new Promise((resolve) => {
    try {
        if (typeof indexedDB === 'undefined') {
            resolve(null);
            return;
        }
        const request = indexedDB.open(DB_NAME, VERSION);
        request.onupgradeneeded = () => {
            if (!request.result.objectStoreNames.contains(STORE)) request.result.createObjectStore(STORE);
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => resolve(null);
        request.onblocked = () => resolve(null);
    } catch {
        resolve(null);
    }
});

const read = <T>(db: IDBDatabase, key: string): Promise<T | undefined> => new Promise((resolve) => {
    try {
        const request = db.transaction(STORE, 'readonly').objectStore(STORE).get(key);
        request.onsuccess = () => resolve(request.result as T | undefined);
        request.onerror = () => resolve(undefined);
    } catch {
        resolve(undefined);
    }
});

// Layout of the store: one record per run and per import, plus the ordered id
// lists that say which records make up the session. A session of a hundred
// images used to be two records rewritten whole on every change — a pending
// run landing structured-cloned every image in the session on the main thread,
// hundreds of MB at a time. Now a change writes the items that changed.
//
// Version 1 kept the two arrays under `runs` / `imports`; they are still read
// when no id lists exist, and dropped by the first save after that.
const RUN_IDS = 'runIds';
const IMPORT_IDS = 'importIds';
const LEGACY_RUNS = 'runs';
const LEGACY_IMPORTS = 'imports';
const runKey = (id: string) => `run/${id}`;
const importKey = (id: string) => `import/${id}`;

interface Identified { id: string }

// What the store holds right now, by id, as the very objects that were
// written. Session items are replaced rather than mutated when they change,
// so identity is enough to tell what needs writing. `null` means unknown —
// the next save rewrites the store from scratch.
let stored: { runs: Map<string, unknown>; imports: Map<string, unknown> } | null = null;

const readItems = async <T>(db: IDBDatabase, ids: unknown, key: (id: string) => string): Promise<T[]> => {
    if (!Array.isArray(ids)) return [];
    const items: Array<T | undefined> = await Promise.all(ids.map((id) => read<T>(db, key(String(id)))));
    return items.filter((item): item is T => item !== undefined);
};

/** What the last session left behind, or empty lists when nothing was kept. */
export const loadPlaygroundSession = async <Run extends Identified, Import extends Identified>(): Promise<PlaygroundSessionSnapshot<Run, Import>> => {
    const db = await openDb();
    if (!db) return { runs: [], imports: [] };
    try {
        const [runIds, importIds] = await Promise.all([read<string[]>(db, RUN_IDS), read<string[]>(db, IMPORT_IDS)]);
        if (Array.isArray(runIds) || Array.isArray(importIds)) {
            const [runs, imports] = await Promise.all([
                readItems<Run>(db, runIds, runKey),
                readItems<Import>(db, importIds, importKey),
            ]);
            // A listed record that did not read back means the store is not
            // what the lists say: treat it as unknown, so the next save
            // rewrites it whole instead of diffing around the gap.
            const complete = runs.length === (Array.isArray(runIds) ? runIds.length : 0)
                && imports.length === (Array.isArray(importIds) ? importIds.length : 0);
            stored = complete
                ? {
                    runs: new Map(runs.map((run) => [run.id, run])),
                    imports: new Map(imports.map((item) => [item.id, item])),
                }
                : null;
            return { runs, imports };
        }
        const [runs, imports] = await Promise.all([read<Run[]>(db, LEGACY_RUNS), read<Import[]>(db, LEGACY_IMPORTS)]);
        stored = null;
        return { runs: Array.isArray(runs) ? runs : [], imports: Array.isArray(imports) ? imports : [] };
    } finally {
        db.close();
    }
};

// Writes one snapshot as a single transaction: changed items put, removed
// items deleted, the id lists rewritten. Resolves with whether it committed,
// so a failed write leaves `stored` as it was and the next save retries it.
const writeSnapshot = (
    db: IDBDatabase,
    snapshot: PlaygroundSessionSnapshot<Identified, Identified>,
    known: typeof stored,
): Promise<boolean> => new Promise((resolve) => {
    try {
        const transaction = db.transaction(STORE, 'readwrite');
        const store = transaction.objectStore(STORE);
        if (!known) store.clear();
        const sync = (items: Identified[], previous: Map<string, unknown> | undefined, key: (id: string) => string) => {
            const ids = new Set<string>();
            for (const item of items) {
                ids.add(item.id);
                if (previous?.get(item.id) !== item) store.put(item, key(item.id));
            }
            previous?.forEach((_, id) => {
                if (!ids.has(id)) store.delete(key(id));
            });
            return [...ids];
        };
        store.put(sync(snapshot.runs, known?.runs, runKey), RUN_IDS);
        store.put(sync(snapshot.imports, known?.imports, importKey), IMPORT_IDS);
        transaction.oncomplete = () => resolve(true);
        transaction.onerror = () => resolve(false);
        transaction.onabort = () => resolve(false);
    } catch {
        resolve(false);
    }
});

// Writes coalesce: a burst of state changes (a run appended, then completed)
// becomes one transaction, and a write that starts while another is in flight
// simply carries the newest snapshot.
let pending: PlaygroundSessionSnapshot<Identified, Identified> | null = null;
let flushing: Promise<void> | null = null;

const flush = async (): Promise<void> => {
    while (pending) {
        const snapshot = pending;
        pending = null;
        const db = await openDb();
        if (!db) return;
        try {
            // A transaction is all or nothing: one that failed left the store
            // as `stored` describes it, so the next save diffs from there.
            if (await writeSnapshot(db, snapshot, stored)) {
                stored = {
                    runs: new Map(snapshot.runs.map((run) => [run.id, run])),
                    imports: new Map(snapshot.imports.map((item) => [item.id, item])),
                };
            }
        } finally {
            db.close();
        }
    }
};

export const savePlaygroundSession = <Run extends Identified, Import extends Identified>(snapshot: PlaygroundSessionSnapshot<Run, Import>): Promise<void> => {
    pending = snapshot;
    if (!flushing) {
        flushing = flush().finally(() => { flushing = null; });
    }
    return flushing;
};
