// The Image Playground's session logic, kept out of the components that render
// it: what a result renders from, how the reference row reorders, which tiles
// the overview shows and what a downloaded file is called. All pure, so the
// behaviour that is easiest to break silently is the behaviour under test
// (imageGenSession.test.ts).

import type {
    GalleryTile,
    GenerationRun,
    ImageResult,
    ImportedImage,
    Quality,
    SelectedImage,
} from './ImageGenPlayground.types';

export const formatBytes = (bytes: number): string => (bytes >= 1024 * 1024
    ? `${(bytes / (1024 * 1024)).toFixed(1)} MB`
    : `${Math.max(1, Math.round(bytes / 1024))} KB`);

// A result's data URL is built once per result object and remembered: the
// concatenation copies the whole base64 payload (megabytes for a 1024px PNG),
// and this runs for every image on every render of the strip, the overview and
// the filmstrip. Keyed weakly, so a result that leaves the session is collected
// with its cached string.
const srcCache = new WeakMap<ImageResult, string>();

// What an API result renders from: the URL when the provider returned one,
// otherwise the inline base64 it sent instead.
export const resultSrc = (image: ImageResult): string => {
    const cached = srcCache.get(image);
    if (cached !== undefined) return cached;
    const src = image.url || (image.b64_json ? `data:image/png;base64,${image.b64_json}` : '');
    srcCache.set(image, src);
    return src;
};

/**
 * One of a run's images, as the lightbox wants it. Every surface that opens an
 * image belonging to a run — the card, the overview, the filmstrip — needs the
 * same nine fields off the same run, so they say it once here.
 */
export const runImage = (
    run: GenerationRun,
    kind: 'output' | 'source',
    index: number,
    src: string,
): SelectedImage => ({
    src,
    prompt: run.prompt,
    model: run.model,
    size: run.size,
    quality: run.quality as Quality,
    index,
    kind,
    runId: run.id,
});

/**
 * Moves one reference image to another slot, keeping every other image's
 * relative order — the same result as dragging a card within a list. Out of
 * range or a no-op move returns the list untouched, so callers can hand it
 * whatever a drag reported without pre-checking.
 */
export const reorderReferences = <T>(list: T[], from: number, to: number): T[] => {
    if (from === to || from < 0 || to < 0 || from >= list.length || to >= list.length) return list;
    const next = [...list];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    return next;
};

/**
 * Applies the reference-image cap in one place, for every way an image gets
 * into the row, and reports what the cap cost so the caller can say it out loud.
 *
 * The two overflow policies are deliberate and different: a batch of files
 * dropped on the row fills the free slots and leaves the rest out (`ignore` —
 * the images already in the row were put there on purpose), while "use this one
 * as a reference" is a request for one specific image and always honours it,
 * letting the oldest make room (`evict`).
 */
export const addReferences = <T>(
    current: T[],
    incoming: T[],
    max: number,
    overflow: 'ignore' | 'evict',
): { next: T[]; ignored: number; evicted: number } => {
    if (overflow === 'ignore') {
        const accepted = incoming.slice(0, Math.max(0, max - current.length));
        return { next: [...current, ...accepted], ignored: incoming.length - accepted.length, evicted: 0 };
    }
    const combined = [...current, ...incoming];
    const next = combined.slice(-max);
    return { next, ignored: 0, evicted: combined.length - next.length };
};

/**
 * The overview's tiles: one per image rather than one per run (a run that asked
 * for two images is two tiles — the grid is about images), plus one tile for
 * every run that failed or is still going, because an overview that drops
 * states is not an overview of the session.
 *
 * Newest first, the opposite of the strip: there the newest is the one it
 * scrolls to, here it should be the first thing the eye lands on.
 */
export const buildGalleryTiles = (runs: GenerationRun[], imported: ImportedImage[]): GalleryTile[] => {
    const fromRuns = runs.flatMap<GalleryTile>((run) => {
        const at = run.createdAt ?? 0;
        if (run.status === 'pending') return [{ key: run.id, at, kind: 'pending', run }];
        if (run.status === 'failed') return [{ key: run.id, at, kind: 'failed', run }];
        return run.images
            .map((image, imageIndex) => ({ imageIndex, src: resultSrc(image) }))
            .filter(({ src }) => src)
            .map(({ imageIndex, src }) => ({
                key: `${run.id}-${imageIndex}`,
                at,
                kind: 'output' as const,
                run,
                imageIndex,
                src,
            }));
    });
    const fromImports = imported.map<GalleryTile>((item) => ({
        key: item.id,
        at: item.createdAt,
        kind: 'import',
        item,
    }));
    return [...fromRuns, ...fromImports].sort((a, b) => b.at - a.at);
};

/**
 * Filters the overview by what the user can remember about an image: the words
 * they wrote, the model they wrote them for, or — for something they brought
 * in — its file name.
 */
export const filterGalleryTiles = (tiles: GalleryTile[], query: string): GalleryTile[] => {
    const needle = query.trim().toLowerCase();
    if (!needle) return tiles;
    return tiles.filter((tile) => (tile.kind === 'import'
        ? tile.item.name.toLowerCase().includes(needle)
        : `${tile.run.prompt} ${tile.run.model}`.toLowerCase().includes(needle)));
};

/**
 * The file name (without extension) a saved image gets. It has to say which
 * image this was: a run's original saved under the run's prompt reads like the
 * result, and a reference or an imported file already has a name of its own
 * that is better than anything derived from the prompt.
 *
 * `slug` is the caller's slugifier applied to free text — passed in so this
 * stays free of the download plumbing.
 */
export const downloadStem = (image: SelectedImage, slug: (text: string) => string): string => {
    if (image.kind === 'reference' || image.kind === 'import') {
        // `label` is the file name (or "Sketch"); drop a trailing extension so
        // the real one can be appended from the blob's own type.
        const name = (image.label ?? '').replace(/\.[a-z0-9]+$/i, '');
        return slug(name);
    }
    const stem = slug(image.prompt);
    return image.kind === 'source'
        ? `${stem}-original-${image.index + 1}`
        : `${stem}-${image.index + 1}`;
};

// How a run's slots are laid out. The results strip scrolls sideways and has a
// fixed height, so width is the cheap axis: never more than two rows (tiles
// stay at least half the panel tall, big enough to pick a favourite without
// opening each one), and the card grows wider with n instead.
const SLOT_WIDTH_ONE_ROW = 220;
const SLOT_WIDTH_TWO_ROWS = 170;
const SLOT_GAP = 8;
const CARD_PADDING = 24;

export const runGridLayout = (slots: number) => {
    const rows = slots <= 2 ? 1 : 2;
    const cols = Math.max(1, Math.ceil(slots / rows));
    const slotWidth = rows === 1 ? SLOT_WIDTH_ONE_ROW : SLOT_WIDTH_TWO_ROWS;
    return { rows, cols, cardWidth: cols * slotWidth + (cols - 1) * SLOT_GAP + CARD_PADDING };
};
