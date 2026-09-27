// The image library's data: what is kept, and the pure logic over it. No
// storage and no React here — the store (store.ts) persists these shapes and
// the UI renders them. See .design/image-library.md.
//
// Prompt material is kept as pieces, not only as whole prompts: the reusable
// asset is usually a term ("rim lighting") or a descriptive phrase ("a quiet
// street after rain"), and a whole prompt is one kind of piece among three.
// Today a person splits a prompt into pieces; the shape lets an AI do the
// splitting later, and an agent pick pieces by kind and tag to assemble one.

// By how it is used: a whole prompt replaces the prompt field; a term or a
// phrase is added to what is there.
export type PieceKind = 'prompt' | 'term' | 'phrase';

export const PIECE_KINDS: PieceKind[] = ['prompt', 'term', 'phrase'];

export interface PromptPiece {
    id: string;
    kind: PieceKind;
    // Only whole prompts have one, and it is optional; see pieceLabel.
    title: string;
    text: string;
    // Lower-case, de-duplicated. How pieces are found again — and, later, how
    // an agent picks them.
    tags: string[];
    // The whole prompt this piece was split from, if any.
    sourceId?: string;
    createdAt: number;
    updatedAt: number;
}

// What a caller sends to create a piece, or to update one when `id` is set.
export interface PieceInput {
    id?: string;
    kind: PieceKind;
    title?: string;
    text: string;
    tags?: string[];
    sourceId?: string;
}

export interface LibraryImage {
    id: string;
    name: string;
    // A data URL — what the playground uses for every image, so a kept image
    // goes back into a request without conversion.
    src: string;
    width?: number;
    height?: number;
    bytes: number;
    createdAt: number;
}

export type ImageInput = Omit<LibraryImage, 'id' | 'createdAt'>;

/** Lower-cased, trimmed, de-duplicated, in first-seen order. */
export const normalizeTags = (tags: string[]): string[] => {
    const seen = new Set<string>();
    for (const tag of tags) {
        const value = tag.trim().toLowerCase();
        if (value) seen.add(value);
    }
    return [...seen];
};

/** What a piece is called in a list: its title, else its first line. */
export const pieceLabel = (piece: Pick<PromptPiece, 'title' | 'text'>): string => (
    piece.title.trim() || piece.text.trim().split('\n')[0].trim()
);

/** Every tag in use, most used first. */
export const collectTags = (pieces: PromptPiece[]): string[] => {
    const counts = new Map<string, number>();
    pieces.forEach((piece) => piece.tags.forEach((tag) => counts.set(tag, (counts.get(tag) ?? 0) + 1)));
    return [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([tag]) => tag);
};

/** Case-insensitive substring match against any of the fields. */
export const matchesQuery = (fields: string[], query: string): boolean => {
    const needle = query.trim().toLowerCase();
    return !needle || fields.some((value) => value.toLowerCase().includes(needle));
};

/**
 * The whole prompt already kept with exactly this text, if any — saving the
 * same prompt twice is almost always a second click, not a wish for two.
 */
export const findPromptByText = (pieces: PromptPiece[], text: string): PromptPiece | undefined => {
    const target = text.trim();
    return pieces.find((piece) => piece.kind === 'prompt' && piece.text.trim() === target);
};

/**
 * Adds a term or phrase to a prompt: after a comma when the prompt ends
 * mid-list, as a new sentence after a full stop, bare when it is empty.
 */
export const appendPiece = (prompt: string, piece: string): string => {
    const base = prompt.replace(/\s+$/, '');
    const addition = piece.trim();
    if (!base || !addition) return base || addition;
    return /[,，;；、:：.!?。！？]$/.test(base) ? `${base} ${addition}` : `${base}, ${addition}`;
};

// --- Splitting ---------------------------------------------------------------

// A piece offered when splitting a prompt. `kind` is a guess the person
// confirms or flips before anything is saved.
export interface PieceCandidate {
    text: string;
    kind: Exclude<PieceKind, 'prompt'>;
}

// Within a line: list separators and sentence ends, Latin and CJK. A Latin
// full stop only cuts when a space follows, so "1.5x" stays whole.
const SEPARATORS = /[,，;；、|]+|(?<=[.!?])\s+|(?<=[。！？])\s*/;
// A bullet or number opening a line: "- ", "• ", "1. ", "2) ".
const LIST_MARKER = /^(?:[-*•]|\d+[.)])\s+/;
const SENTENCE_END = /[.!?。！？]+$/;
const CJK = /[぀-ヿ㐀-鿿가-힯]/g;
// At most this many words (or CJK characters) reads as a term.
const TERM_MAX_WORDS = 4;
const TERM_MAX_CJK = 8;

// Judged on the words alone, so a short fragment that happens to end a
// sentence ("电影感光影。") is still a term.
const looksLikeTerm = (text: string): boolean => {
    const cjk = (text.match(CJK) ?? []).length;
    if (cjk > 0) return cjk <= TERM_MAX_CJK && text.replace(/\s/g, '').length <= TERM_MAX_CJK + 4;
    return text.split(/\s+/).length <= TERM_MAX_WORDS;
};

/**
 * A first cut of a prompt into pieces, by punctuation alone — a starting
 * point for the person splitting it, not a judgement of what matters. This is
 * the seam an AI splitter replaces: same input, same candidate shape.
 */
export const suggestPieces = (text: string): PieceCandidate[] => {
    const seen = new Set<string>();
    const candidates: PieceCandidate[] = [];
    const parts = text.split('\n').flatMap((line) => line.trim().replace(LIST_MARKER, '').split(SEPARATORS));
    for (const raw of parts) {
        const piece = (raw ?? '').trim();
        const key = piece.toLowerCase();
        if (!piece || seen.has(key)) continue;
        seen.add(key);
        // A term is added into the middle of other prompts, so it goes in
        // without the full stop it happened to end on.
        const bare = piece.replace(SENTENCE_END, '').trim();
        candidates.push(bare && looksLikeTerm(bare) ? { text: bare, kind: 'term' } : { text: piece, kind: 'phrase' });
    }
    return candidates;
};
