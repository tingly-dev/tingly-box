// The image library: prompt material and reference images the user chose to
// keep, as opposed to the playground session, which is everything that
// happened. A session gets cleared; the library is what survives that.
//
// Prompt material is kept as pieces, not only as whole prompts: the reusable
// asset is usually a term ("rim lighting") or a descriptive phrase ("a quiet
// street after rain, reflections on the asphalt"), and a whole prompt is one
// kind of piece among three. Today a person splits a prompt into pieces; the
// shape is meant to let an AI do the splitting later, and an agent pick
// pieces by kind and tag to assemble a prompt. See .design/image-library.md.
//
// IndexedDB for the same reason as utils/playgroundSession.ts — references are
// base64 images, far past what localStorage holds — and best-effort in the
// same way: a browser that refuses IndexedDB reads an empty library and
// reports failed saves, it never throws at the page.

const DB_NAME = 'tingly-image-library';
const VERSION = 1;
const PROMPTS = 'prompts';
const REFERENCES = 'references';

// What a piece of prompt material is, by how it is used: a whole prompt
// replaces the field; a term or a phrase is added to what is there.
export type PromptPieceKind = 'prompt' | 'term' | 'phrase';

export const PROMPT_PIECE_KINDS: PromptPieceKind[] = ['prompt', 'term', 'phrase'];

export interface LibraryPrompt {
    id: string;
    kind: PromptPieceKind;
    // Optional: a short prompt is its own name. The list falls back to the
    // text's first line.
    title: string;
    text: string;
    // Free-form labels — subject, style, lighting, a project name. Kept
    // lower-case and de-duplicated; they are how pieces are found again (and,
    // later, how an agent picks them).
    tags: string[];
    // The whole prompt this piece was split from, if any.
    sourceId?: string;
    createdAt: number;
    updatedAt: number;
}

export interface LibraryReference {
    id: string;
    name: string;
    // A data URL, the representation the playground uses for every image, so
    // a kept reference goes back into a request without conversion.
    src: string;
    width?: number;
    height?: number;
    bytes: number;
    createdAt: number;
}

type StoreName = typeof PROMPTS | typeof REFERENCES;

const openDb = (): Promise<IDBDatabase | null> => new Promise((resolve) => {
    try {
        if (typeof indexedDB === 'undefined') {
            resolve(null);
            return;
        }
        const request = indexedDB.open(DB_NAME, VERSION);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains(PROMPTS)) db.createObjectStore(PROMPTS, { keyPath: 'id' });
            if (!db.objectStoreNames.contains(REFERENCES)) db.createObjectStore(REFERENCES, { keyPath: 'id' });
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => resolve(null);
        request.onblocked = () => resolve(null);
    } catch {
        resolve(null);
    }
});

const readAll = async <T>(store: StoreName): Promise<T[]> => {
    const db = await openDb();
    if (!db) return [];
    try {
        return await new Promise<T[]>((resolve) => {
            try {
                const request = db.transaction(store, 'readonly').objectStore(store).getAll();
                request.onsuccess = () => resolve(Array.isArray(request.result) ? request.result as T[] : []);
                request.onerror = () => resolve([]);
            } catch {
                resolve([]);
            }
        });
    } finally {
        db.close();
    }
};

// One write, resolved with whether it committed — the caller says so when it
// did not, rather than showing an item as kept that is not.
const write = async (store: StoreName, apply: (objectStore: IDBObjectStore) => void): Promise<boolean> => {
    const db = await openDb();
    if (!db) return false;
    try {
        const ok = await new Promise<boolean>((resolve) => {
            try {
                const transaction = db.transaction(store, 'readwrite');
                apply(transaction.objectStore(store));
                transaction.oncomplete = () => resolve(true);
                transaction.onerror = () => resolve(false);
                transaction.onabort = () => resolve(false);
            } catch {
                resolve(false);
            }
        });
        if (ok) notify();
        return ok;
    } finally {
        db.close();
    }
};

// Every open view of the library (the page, the playground's pickers) hears
// about a write made by any other, so saving from the playground shows up in
// its own "saved prompts" menu without a reload.
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((listener) => listener());
export const subscribeImageLibrary = (listener: () => void): (() => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
};

const newId = () => (typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`);

/** Lower-cased, trimmed, de-duplicated, in first-seen order. */
export const normalizeTags = (tags: string[]): string[] => {
    const seen = new Set<string>();
    for (const tag of tags) {
        const value = tag.trim().toLowerCase();
        if (value) seen.add(value);
    }
    return [...seen];
};

// Fills in fields a record may lack, so the rest of the code never has to ask.
const normalizePrompt = (prompt: Partial<LibraryPrompt> & Pick<LibraryPrompt, 'id' | 'text'>): LibraryPrompt => ({
    id: prompt.id,
    kind: prompt.kind && PROMPT_PIECE_KINDS.includes(prompt.kind) ? prompt.kind : 'prompt',
    title: prompt.title ?? '',
    text: prompt.text,
    tags: Array.isArray(prompt.tags) ? normalizeTags(prompt.tags) : [],
    ...(prompt.sourceId ? { sourceId: prompt.sourceId } : {}),
    createdAt: prompt.createdAt ?? 0,
    updatedAt: prompt.updatedAt ?? prompt.createdAt ?? 0,
});

/** Every tag used across the pieces, most used first. */
export const collectTags = (prompts: LibraryPrompt[]): string[] => {
    const counts = new Map<string, number>();
    prompts.forEach((prompt) => prompt.tags.forEach((tag) => counts.set(tag, (counts.get(tag) ?? 0) + 1)));
    return [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([tag]) => tag);
};

// A candidate piece offered when splitting a prompt. `kind` is a guess the
// person confirms or flips before anything is saved.
export interface PromptPieceCandidate {
    text: string;
    kind: Exclude<PromptPieceKind, 'prompt'>;
}

// Where a line is cut (after the prompt is cut into lines): sentence ends and the list separators
// prompts are usually written with, in both Latin and CJK punctuation.
const PIECE_SEPARATORS = /[,，;；、|]+|(?<=[.!?])\s+|(?<=[。！？])\s*/;
// A bullet or number opening a line: "- ", "• ", "1. ", "2) ".
const LIST_MARKER = /^(?:[-*•]|\d+[.)])\s+/;
// Up to this many words (or CJK characters) with no sentence punctuation
// reads as a term; anything longer describes, so it is a phrase.
const TERM_MAX_WORDS = 4;
const TERM_MAX_CJK = 8;

const looksLikeTerm = (text: string): boolean => {
    if (/[.!?。！？]$/.test(text)) return false;
    const cjk = (text.match(/[\u3040-\u30ff\u3400-\u9fff\uac00-\ud7af]/g) ?? []).length;
    if (cjk > 0) return cjk <= TERM_MAX_CJK && text.replace(/\s/g, '').length <= TERM_MAX_CJK + 4;
    return text.split(/\s+/).length <= TERM_MAX_WORDS;
};

/**
 * A first cut of a prompt into pieces, by punctuation alone — a starting
 * point for the person splitting it, not a judgement of what matters. This is
 * the seam an AI splitter replaces: same input, same candidate shape.
 */
export const suggestPromptPieces = (text: string): PromptPieceCandidate[] => {
    const seen = new Set<string>();
    const pieces: PromptPieceCandidate[] = [];
    const parts = text.split('\n').flatMap((line) => line.trim().replace(LIST_MARKER, '').split(PIECE_SEPARATORS));
    for (const raw of parts) {
        const piece = (raw ?? '').trim();
        if (!piece || seen.has(piece.toLowerCase())) continue;
        seen.add(piece.toLowerCase());
        pieces.push({ text: piece, kind: looksLikeTerm(piece) ? 'term' : 'phrase' });
    }
    return pieces;
};

/**
 * Adds a piece to a prompt: after a comma when the prompt ends mid-list, on a
 * new sentence after a full stop, bare when the prompt is empty.
 */
export const appendPromptPiece = (prompt: string, piece: string): string => {
    const base = prompt.replace(/\s+$/, '');
    const addition = piece.trim();
    if (!base) return addition;
    if (!addition) return base;
    if (/[,，;；、:：]$/.test(base)) return `${base} ${addition}`;
    if (/[.!?。！？]$/.test(base)) return `${base} ${addition}`;
    return `${base}, ${addition}`;
};

/** Newest-edited first: the prompt being iterated on is the one wanted next. */
export const sortPrompts = (prompts: LibraryPrompt[]): LibraryPrompt[] => (
    [...prompts].sort((a, b) => b.updatedAt - a.updatedAt)
);

/** Newest first. */
export const sortReferences = (references: LibraryReference[]): LibraryReference[] => (
    [...references].sort((a, b) => b.createdAt - a.createdAt)
);

/** What a prompt is called in a list: its title, else its first line. */
export const promptLabel = (prompt: Pick<LibraryPrompt, 'title' | 'text'>): string => (
    prompt.title.trim() || prompt.text.trim().split('\n')[0].trim()
);

/** Case-insensitive match on title, text and tags (pieces) or name (references). */
export const matchesQuery = (haystack: string[], query: string): boolean => {
    const needle = query.trim().toLowerCase();
    if (!needle) return true;
    return haystack.some((value) => value.toLowerCase().includes(needle));
};

/**
 * The prompt already kept with exactly this text, if any. Saving the same
 * prompt twice is almost always a second click, not a wish for two copies.
 */
export const findPromptByText = (prompts: LibraryPrompt[], text: string): LibraryPrompt | undefined => {
    const target = text.trim();
    return prompts.find((prompt) => prompt.text.trim() === target);
};

export const loadLibraryPrompts = async (): Promise<LibraryPrompt[]> => (
    sortPrompts((await readAll<LibraryPrompt>(PROMPTS)).filter((prompt) => typeof prompt?.text === 'string').map(normalizePrompt))
);

export const loadLibraryReferences = async (): Promise<LibraryReference[]> => sortReferences(await readAll<LibraryReference>(REFERENCES));

export interface LibraryPromptInput {
    // Set to update an existing piece in place.
    id?: string;
    kind?: PromptPieceKind;
    title?: string;
    text: string;
    tags?: string[];
    sourceId?: string;
    createdAt?: number;
}

const buildPrompt = (input: LibraryPromptInput, now: number): LibraryPrompt => normalizePrompt({
    id: input.id ?? newId(),
    kind: input.kind ?? 'prompt',
    title: (input.title ?? '').trim(),
    text: input.text.trim(),
    tags: input.tags ?? [],
    sourceId: input.sourceId,
    createdAt: input.createdAt ?? now,
    updatedAt: now,
});

/** Creates a piece, or updates it when `id` names an existing one. */
export const saveLibraryPrompt = async (input: LibraryPromptInput): Promise<LibraryPrompt | null> => {
    const prompt = buildPrompt(input, Date.now());
    return (await write(PROMPTS, (store) => { store.put(prompt); })) ? prompt : null;
};

/** Saves several pieces in one transaction — the result of splitting a prompt. */
export const saveLibraryPrompts = async (inputs: LibraryPromptInput[]): Promise<LibraryPrompt[] | null> => {
    const now = Date.now();
    const prompts = inputs.map((input, index) => buildPrompt(input, now + index));
    return (await write(PROMPTS, (store) => { prompts.forEach((prompt) => store.put(prompt)); })) ? prompts : null;
};

export const deleteLibraryPrompt = (id: string): Promise<boolean> => write(PROMPTS, (store) => { store.delete(id); });

export const saveLibraryReferences = async (
    inputs: Array<Omit<LibraryReference, 'id' | 'createdAt'>>,
): Promise<LibraryReference[] | null> => {
    const now = Date.now();
    // Staggered by a millisecond so a batch keeps the order it arrived in.
    const references = inputs.map((input, index): LibraryReference => ({ ...input, id: newId(), createdAt: now + index }));
    return (await write(REFERENCES, (store) => { references.forEach((reference) => store.put(reference)); })) ? references : null;
};

export const renameLibraryReference = async (reference: LibraryReference, name: string): Promise<boolean> => (
    write(REFERENCES, (store) => { store.put({ ...reference, name: name.trim() || reference.name }); })
);

export const deleteLibraryReference = (id: string): Promise<boolean> => write(REFERENCES, (store) => { store.delete(id); });
