import { describe, expect, it } from 'vitest';
import { computeTileRects, FULL_CROP, normalizeCrop, tileFileName } from './imageSlice';

describe('computeTileRects', () => {
    it('divides an image into rows x cols tiles covering the whole frame', () => {
        const rects = computeTileRects(900, 900, { rows: 3, cols: 3, crop: FULL_CROP, gutter: 0 });
        expect(rects).toHaveLength(9);
        expect(rects[0]).toMatchObject({ index: 0, row: 0, col: 0, x: 0, y: 0, width: 300, height: 300 });
        expect(rects[8]).toMatchObject({ index: 8, row: 2, col: 2, x: 600, y: 600, width: 300, height: 300 });
    });

    it('divides the frame, not the image, so an off-centre region cuts cleanly', () => {
        const rects = computeTileRects(1000, 500, {
            rows: 1,
            cols: 2,
            crop: { x: 0.5, y: 0.2, width: 0.4, height: 0.6 },
            gutter: 0,
        });
        expect(rects[0]).toMatchObject({ x: 500, y: 100, width: 200, height: 300 });
        expect(rects[1]).toMatchObject({ x: 700, y: 100, width: 200, height: 300 });
    });

    it('removes the gutter symmetrically, keeping every tile the same size', () => {
        const rects = computeTileRects(400, 400, { rows: 2, cols: 2, crop: FULL_CROP, gutter: 0.2 });
        const widths = new Set(rects.map((rect) => rect.width));
        const heights = new Set(rects.map((rect) => rect.height));
        expect(widths).toEqual(new Set([160]));
        expect(heights).toEqual(new Set([160]));
        expect(rects[0]).toMatchObject({ x: 20, y: 20 });
        expect(rects[3]).toMatchObject({ x: 220, y: 220 });
    });

    it('never returns tiles outside the image or of zero size', () => {
        const rects = computeTileRects(120, 80, {
            rows: 4,
            cols: 4,
            crop: { x: 0.9, y: 0.9, width: 0.4, height: 0.4 },
            gutter: 0.99,
        });
        for (const rect of rects) {
            expect(rect.width).toBeGreaterThan(0);
            expect(rect.height).toBeGreaterThan(0);
            expect(rect.x + rect.width).toBeLessThanOrEqual(120);
            expect(rect.y + rect.height).toBeLessThanOrEqual(80);
        }
    });

    it('clamps a degenerate grid to at least one tile', () => {
        expect(computeTileRects(100, 100, { rows: 0, cols: 0, crop: FULL_CROP, gutter: 0 })).toHaveLength(1);
    });
});

describe('normalizeCrop', () => {
    it('keeps a frame inside the image', () => {
        expect(normalizeCrop({ x: -0.2, y: 0.5, width: 2, height: 2 }))
            .toMatchObject({ x: 0, y: 0.5, width: 1, height: 0.5 });
    });

    it('never lets an edge pass another', () => {
        const crop = normalizeCrop({ x: 0.99, y: 0.99, width: 0, height: 0 });
        expect(crop.width).toBeGreaterThan(0);
        expect(crop.height).toBeGreaterThan(0);
        expect(crop.x + crop.width).toBeLessThanOrEqual(1);
        expect(crop.y + crop.height).toBeLessThanOrEqual(1);
    });

    it('treats a missing frame as the whole image', () => {
        expect(normalizeCrop(undefined)).toEqual(FULL_CROP);
    });
});

describe('tileFileName', () => {
    it('pads the index to the width of the total', () => {
        expect(tileFileName('cat', 0, 9)).toBe('cat-1.png');
        expect(tileFileName('cat', 9, 16)).toBe('cat-10.png');
        expect(tileFileName('cat', 0, 16)).toBe('cat-01.png');
    });
});
