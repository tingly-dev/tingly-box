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

const write = (db: IDBDatabase, key: string, value: unknown): Promise<void> => new Promise((resolve) => {
    try {
        const transaction = db.transaction(STORE, 'readwrite');
        transaction.objectStore(STORE).put(value, key);
        transaction.oncomplete = () => resolve();
        transaction.onerror = () => resolve();
        transaction.onabort = () => resolve();
    } catch {
        resolve();
    }
});

/** What the last session left behind, or empty lists when nothing was kept. */
export const loadPlaygroundSession = async <Run, Import>(): Promise<PlaygroundSessionSnapshot<Run, Import>> => {
    const db = await openDb();
    if (!db) return { runs: [], imports: [] };
    try {
        const [runs, imports] = await Promise.all([read<Run[]>(db, 'runs'), read<Import[]>(db, 'imports')]);
        return { runs: Array.isArray(runs) ? runs : [], imports: Array.isArray(imports) ? imports : [] };
    } finally {
        db.close();
    }
};

// Writes coalesce: a burst of state changes (a run appended, then completed)
// becomes one transaction, and a write that starts while another is in flight
// simply carries the newest snapshot.
let pending: PlaygroundSessionSnapshot<unknown, unknown> | null = null;
let flushing: Promise<void> | null = null;

const flush = async (): Promise<void> => {
    while (pending) {
        const snapshot = pending;
        pending = null;
        const db = await openDb();
        if (!db) return;
        try {
            await write(db, 'runs', snapshot.runs);
            await write(db, 'imports', snapshot.imports);
        } finally {
            db.close();
        }
    }
};

export const savePlaygroundSession = <Run, Import>(snapshot: PlaygroundSessionSnapshot<Run, Import>): Promise<void> => {
    pending = snapshot;
    if (!flushing) {
        flushing = flush().finally(() => { flushing = null; });
    }
    return flushing;
};
