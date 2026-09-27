// Where the library is kept. Today: this browser's IndexedDB. The functions
// below are the whole contract the UI relies on, shaped like the resource API
// that will replace them (list / save / delete per collection), so moving the
// library to a backend is a rewrite of this file only.
//
// IndexedDB because images are base64 — far past what localStorage holds —
// and best-effort like utils/playgroundSession.ts: a browser that refuses
// IndexedDB reads an empty library and gets `null` back from writes, never an
// exception.

import {
    PIECE_KINDS,
    normalizeTags,
    type ImageInput,
    type LibraryImage,
    type PieceInput,
    type PromptPiece,
} from './model';

const DB_NAME = 'tingly-image-library';
const VERSION = 1;
type Collection = 'pieces' | 'images';

const openDb = (): Promise<IDBDatabase | null> => new Promise((resolve) => {
    try {
        if (typeof indexedDB === 'undefined') {
            resolve(null);
            return;
        }
        const request = indexedDB.open(DB_NAME, VERSION);
        request.onupgradeneeded = () => {
            for (const name of ['pieces', 'images'] satisfies Collection[]) {
                if (!request.result.objectStoreNames.contains(name)) request.result.createObjectStore(name, { keyPath: 'id' });
            }
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => resolve(null);
        request.onblocked = () => resolve(null);
    } catch {
        resolve(null);
    }
});

// Runs one request or transaction against a fresh connection. `run` returns
// the request whose result is wanted, or nothing for a write, which then
// resolves with whether the transaction committed.
const withStore = async <T>(
    collection: Collection,
    mode: IDBTransactionMode,
    run: (store: IDBObjectStore) => IDBRequest<T> | void,
    fallback: T,
): Promise<T> => {
    const db = await openDb();
    if (!db) return fallback;
    try {
        return await new Promise<T>((resolve) => {
            try {
                const transaction = db.transaction(collection, mode);
                const request = run(transaction.objectStore(collection));
                transaction.oncomplete = () => resolve(request ? request.result : (true as T));
                transaction.onerror = () => resolve(fallback);
                transaction.onabort = () => resolve(fallback);
            } catch {
                resolve(fallback);
            }
        });
    } finally {
        db.close();
    }
};

const readAll = async <T>(collection: Collection): Promise<T[]> => {
    const rows = await withStore<unknown>(collection, 'readonly', (store) => store.getAll() as IDBRequest<unknown>, []);
    return Array.isArray(rows) ? rows as T[] : [];
};

const write = async (collection: Collection, apply: (store: IDBObjectStore) => void): Promise<boolean> => {
    const ok = await withStore<boolean>(collection, 'readwrite', (store) => { apply(store); }, false);
    if (ok) listeners.forEach((listener) => listener());
    return ok;
};

// Every open view (the library page, the playground's menu and picker) hears
// about a write made by any other, so a save shows up everywhere at once.
const listeners = new Set<() => void>();
export const subscribeLibrary = (listener: () => void): (() => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
};

const newId = () => (typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`);

// --- Pieces ------------------------------------------------------------------

/** Newest-edited first: what is being iterated on is what is wanted next. */
export const listPieces = async (): Promise<PromptPiece[]> => (await readAll<PromptPiece>('pieces'))
    .filter((piece) => typeof piece?.text === 'string' && PIECE_KINDS.includes(piece.kind))
    .sort((a, b) => b.updatedAt - a.updatedAt);

/**
 * Creates pieces, or updates the ones whose `id` is set (keeping when they
 * were created), in one write. The whole batch lands or none of it does;
 * `null` when it did not.
 */
export const savePieces = async (inputs: PieceInput[]): Promise<PromptPiece[] | null> => {
    const now = Date.now();
    const pieces = inputs.map((input, index): PromptPiece => ({
        id: input.id ?? newId(),
        kind: input.kind,
        // Terms and phrases are their own name.
        title: input.kind === 'prompt' ? (input.title ?? '').trim() : '',
        text: input.text.trim(),
        tags: normalizeTags(input.tags ?? []),
        ...(input.sourceId ? { sourceId: input.sourceId } : {}),
        // Staggered so a batch keeps the order it was given in.
        createdAt: now + index,
        updatedAt: now + index,
    }));
    const ok = await write('pieces', (store) => pieces.forEach((piece) => {
        const previous = store.get(piece.id);
        previous.onsuccess = () => {
            const createdAt = (previous.result as PromptPiece | undefined)?.createdAt;
            store.put(createdAt ? { ...piece, createdAt } : piece);
        };
    }));
    return ok ? pieces : null;
};

export const deletePiece = (id: string): Promise<boolean> => write('pieces', (store) => { store.delete(id); });

// --- Images ------------------------------------------------------------------

/** Newest first. */
export const listImages = async (): Promise<LibraryImage[]> => (await readAll<LibraryImage>('images'))
    .filter((image) => typeof image?.src === 'string')
    .sort((a, b) => b.createdAt - a.createdAt);

export const addImages = async (inputs: ImageInput[]): Promise<LibraryImage[] | null> => {
    const now = Date.now();
    const images = inputs.map((input, index): LibraryImage => ({ ...input, id: newId(), createdAt: now + index }));
    return (await write('images', (store) => images.forEach((image) => store.put(image)))) ? images : null;
};

export const renameImage = (image: LibraryImage, name: string): Promise<boolean> => (
    write('images', (store) => { store.put({ ...image, name: name.trim() || image.name }); })
);

export const deleteImage = (id: string): Promise<boolean> => write('images', (store) => { store.delete(id); });
