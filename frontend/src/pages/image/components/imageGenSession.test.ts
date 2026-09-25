import { describe, it, expect } from 'vitest';
import { slugify } from '@/utils/download';
import {
    addReferences,
    buildGalleryTiles,
    downloadStem,
    filterGalleryTiles,
    formatBytes,
    reorderReferences,
    resultSrc,
    runGridLayout,
    stripCardBasis,
    STRIP_CARD_HEIGHT,
    runImage,
} from './imageGenSession';
import type { GenerationRun, ImportedImage, SelectedImage } from './ImageGenPlayground.types';

const run = (over: Partial<GenerationRun> & Pick<GenerationRun, 'id'>): GenerationRun => ({
    endpoint: 'generations',
    createdAt: 1,
    prompt: 'a tiny copper robot',
    model: 'gpt-image-1',
    size: '1024x1024',
    quality: 'auto',
    images: [],
    status: 'completed',
    ...over,
});

const imported = (over: Partial<ImportedImage> & Pick<ImportedImage, 'id'>): ImportedImage => ({
    src: 'data:image/png;base64,ZmlsZQ==',
    name: 'sheet.png',
    bytes: 2048,
    createdAt: 1,
    ...over,
});

const selected = (over: Partial<SelectedImage>): SelectedImage => ({
    src: 'data:image/png;base64,aW1n',
    prompt: 'a tiny copper robot',
    model: 'gpt-image-1',
    size: '1024x1024',
    quality: 'auto',
    index: 0,
    kind: 'output',
    ...over,
});

describe('resultSrc', () => {
    it('prefers the provider URL when there is one', () => {
        expect(resultSrc({ url: 'https://cdn.example/a.png', b64_json: 'aW1n' })).toBe('https://cdn.example/a.png');
    });

    it('falls back to the inline base64 as a data URL', () => {
        expect(resultSrc({ b64_json: 'aW1n' })).toBe('data:image/png;base64,aW1n');
    });

    it('reports nothing renderable as an empty string, not "undefined"', () => {
        // A result with neither is what an upstream returns when it refuses;
        // callers filter on this, so it must be falsy rather than a broken src.
        expect(resultSrc({})).toBe('');
    });
});

describe('resultSrc caching', () => {
    it('hands back the identical string for the same result object', () => {
        // The data URL copies the whole base64 payload; the strip, the overview
        // and the filmstrip all ask for it on every render.
        const image = { b64_json: 'aW1n' };
        expect(resultSrc(image)).toBe(resultSrc(image));
    });
});

describe('runImage', () => {
    it('carries the run request and the image identity into the lightbox', () => {
        const r = run({ id: 'r1', prompt: 'a bonsai', model: 'gpt-image-1' });
        expect(runImage(r, 'source', 2, 'data:image/png;base64,x')).toEqual({
            src: 'data:image/png;base64,x',
            prompt: 'a bonsai',
            model: 'gpt-image-1',
            size: '1024x1024',
            quality: 'auto',
            index: 2,
            kind: 'source',
            runId: 'r1',
        });
    });
});

describe('addReferences', () => {
    it('fills the free slots and reports what was left out', () => {
        // A batch dropped on a row that is nearly full: the images already there
        // were put there on purpose, so the extras are the ones that lose.
        const { next, ignored, evicted } = addReferences(['a', 'b', 'c', 'd'], ['e', 'f', 'g'], 5, 'ignore');
        expect(next).toEqual(['a', 'b', 'c', 'd', 'e']);
        expect({ ignored, evicted }).toEqual({ ignored: 2, evicted: 0 });
    });

    it('adds nothing, and says so, when the row is already full', () => {
        const { next, ignored } = addReferences(['a', 'b'], ['c'], 2, 'ignore');
        expect(next).toEqual(['a', 'b']);
        expect(ignored).toBe(1);
    });

    it('honours a pointed-at image by evicting the oldest', () => {
        // "Use this one as a reference" is a request for one specific image.
        const { next, ignored, evicted } = addReferences(['a', 'b', 'c'], ['d'], 3, 'evict');
        expect(next).toEqual(['b', 'c', 'd']);
        expect({ ignored, evicted }).toEqual({ ignored: 0, evicted: 1 });
    });

    it('evicts nothing while there is room', () => {
        const { next, evicted } = addReferences(['a'], ['b'], 3, 'evict');
        expect(next).toEqual(['a', 'b']);
        expect(evicted).toBe(0);
    });
});

describe('reorderReferences', () => {
    it('moves an image later without disturbing the others', () => {
        expect(reorderReferences(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a']);
    });

    it('moves an image earlier', () => {
        expect(reorderReferences(['a', 'b', 'c'], 2, 0)).toEqual(['c', 'a', 'b']);
    });

    it('returns the same list for a no-op or an out-of-range drop', () => {
        const list = ['a', 'b'];
        expect(reorderReferences(list, 1, 1)).toBe(list);
        expect(reorderReferences(list, -1, 0)).toBe(list);
        expect(reorderReferences(list, 0, 5)).toBe(list);
    });
});

describe('buildGalleryTiles', () => {
    it('makes one tile per image, not per run', () => {
        const tiles = buildGalleryTiles(
            [run({ id: 'r1', images: [{ b64_json: 'one' }, { b64_json: 'two' }] })],
            [],
        );
        expect(tiles).toHaveLength(2);
        expect(tiles.map((tile) => tile.key)).toEqual(['r1-0', 'r1-1']);
    });

    it('keeps a failed and an in-flight run as their own tile', () => {
        // An overview that drops states is not an overview of the session.
        const tiles = buildGalleryTiles([
            run({ id: 'r1', status: 'failed', error: '502' }),
            run({ id: 'r2', status: 'pending' }),
        ], []);
        expect(tiles.map((tile) => tile.kind).sort()).toEqual(['failed', 'pending']);
    });

    it('drops result entries that carry no image at all', () => {
        const tiles = buildGalleryTiles([run({ id: 'r1', images: [{}, { b64_json: 'two' }] })], []);
        expect(tiles).toHaveLength(1);
        expect(tiles[0].key).toBe('r1-1');
    });

    it('orders newest first across runs and imports alike', () => {
        const tiles = buildGalleryTiles(
            [run({ id: 'old', createdAt: 10, images: [{ b64_json: 'a' }] }),
                run({ id: 'new', createdAt: 30, images: [{ b64_json: 'b' }] })],
            [imported({ id: 'mid', createdAt: 20 })],
        );
        expect(tiles.map((tile) => (tile.kind === 'import' ? tile.item.id : tile.run.id)))
            .toEqual(['new', 'mid', 'old']);
    });
});

describe('filterGalleryTiles', () => {
    const tiles = buildGalleryTiles(
        [run({ id: 'r1', prompt: 'an isometric ramen shop', images: [{ b64_json: 'a' }] }),
            run({ id: 'r2', prompt: 'a lighthouse', model: 'dall-e-3', images: [{ b64_json: 'b' }] })],
        [imported({ id: 'i1', name: 'pose-sheet.png' })],
    );

    it('returns everything for a blank or whitespace query', () => {
        expect(filterGalleryTiles(tiles, '')).toBe(tiles);
        expect(filterGalleryTiles(tiles, '   ')).toBe(tiles);
    });

    it('matches a prompt case-insensitively', () => {
        expect(filterGalleryTiles(tiles, 'RAMEN')).toHaveLength(1);
    });

    it('matches the model a run went to', () => {
        expect(filterGalleryTiles(tiles, 'dall-e')).toHaveLength(1);
    });

    it('matches an imported file by its name', () => {
        const hits = filterGalleryTiles(tiles, 'pose');
        expect(hits).toHaveLength(1);
        expect(hits[0].kind).toBe('import');
    });
});

describe('downloadStem', () => {
    it('numbers a generated image after its prompt', () => {
        expect(downloadStem(selected({ kind: 'output', index: 1 }), slugify)).toBe('a-tiny-copper-robot-2');
    });

    it('marks a run original so it cannot be mistaken for the result', () => {
        expect(downloadStem(selected({ kind: 'source', index: 0 }), slugify)).toBe('a-tiny-copper-robot-original-1');
    });

    it('names a reference after its own file, without doubling the extension', () => {
        expect(downloadStem(selected({ kind: 'reference', label: 'pose-sheet.png', prompt: '' }), slugify))
            .toBe('pose-sheet');
    });

    it('falls back to a usable name when a reference has no label', () => {
        expect(downloadStem(selected({ kind: 'import', label: undefined, prompt: '' }), slugify)).toBe('image');
    });
});

describe('formatBytes', () => {
    it('rounds small files up to at least 1 KB rather than showing 0', () => {
        expect(formatBytes(300)).toBe('1 KB');
    });

    it('switches to MB at a megabyte', () => {
        expect(formatBytes(1024 * 1024 * 2.5)).toBe('2.5 MB');
    });
});

describe('runGridLayout', () => {
    it('keeps one or two images on a single row', () => {
        expect(runGridLayout(1)).toMatchObject({ rows: 1, cols: 1 });
        expect(runGridLayout(2)).toMatchObject({ rows: 1, cols: 2 });
    });

    it('never goes past two rows, growing wider instead', () => {
        expect(runGridLayout(3)).toMatchObject({ rows: 2, cols: 2 });
        expect(runGridLayout(4)).toMatchObject({ rows: 2, cols: 2 });
        expect(runGridLayout(5)).toMatchObject({ rows: 2, cols: 3 });
        expect(runGridLayout(10)).toMatchObject({ rows: 2, cols: 5 });
    });

    it('widens the card with each column', () => {
        expect(runGridLayout(10).cardWidth).toBeGreaterThan(runGridLayout(4).cardWidth);
    });
});

describe('stripCardBasis', () => {
    it('keeps a single image card at least its tuned width, else as wide as a square slot', () => {
        expect(stripCardBasis(null)).toBe('0 0 min(100%, max(clamp(280px, 46%, 360px), calc((min(100cqh, 396px) - 76px) / 1 * 1 + 24px)))');
        expect(stripCardBasis(runGridLayout(1))).toBe(stripCardBasis(null));
    });

    it('sizes multi-image cards for square slots across their columns and rows', () => {
        const four = runGridLayout(4);
        expect(stripCardBasis(four)).toBe(`0 0 min(100%, max(${four.cardWidth}px, calc((min(100cqh, 396px) - 84px) / 2 * 2 + 32px)))`);
        const five = runGridLayout(5);
        expect(stripCardBasis(five)).toBe(`0 0 min(100%, max(${five.cardWidth}px, calc((min(100cqh, 396px) - 84px) / 2 * 3 + 40px)))`);
    });

    it('stops growing past the height cap, the same one the card height uses', () => {
        expect(STRIP_CARD_HEIGHT).toBe('min(100%, 396px)');
    });
});
