import { describe, expect, it } from 'vitest';
import {
    BRUSH_SIZES,
    brushWidthFor,
    DEFAULT_SKETCH_DIMENSIONS,
    fitWithin,
    parseImageSize,
    StrokeHistory,
    toCanvasPoint,
} from './sketchCanvas';

describe('parseImageSize', () => {
    it('reads the Playground size string into canvas dimensions', () => {
        expect(parseImageSize('1024x1792')).toEqual({ width: 1024, height: 1792 });
        expect(parseImageSize('512X512')).toEqual({ width: 512, height: 512 });
        expect(parseImageSize(' 1792 × 1024 ')).toEqual({ width: 1792, height: 1024 });
    });

    it('falls back to a square canvas for anything it cannot read', () => {
        expect(parseImageSize('auto')).toEqual(DEFAULT_SKETCH_DIMENSIONS);
        expect(parseImageSize('')).toEqual(DEFAULT_SKETCH_DIMENSIONS);
        expect(parseImageSize('0x1024')).toEqual(DEFAULT_SKETCH_DIMENSIONS);
    });
});

describe('fitWithin', () => {
    it('keeps the aspect ratio while fitting the box', () => {
        expect(fitWithin({ width: 1024, height: 1792 }, { width: 800, height: 600 })).toEqual({ width: 342, height: 600 });
        expect(fitWithin({ width: 1792, height: 1024 }, { width: 800, height: 600 })).toEqual({ width: 800, height: 457 });
    });

    it('never scales a canvas above its own backing store', () => {
        expect(fitWithin({ width: 512, height: 512 }, { width: 2000, height: 2000 })).toEqual({ width: 512, height: 512 });
    });

    it('collapses to nothing for an unmeasured box', () => {
        expect(fitWithin({ width: 1024, height: 1024 }, { width: 0, height: 0 })).toEqual({ width: 0, height: 0 });
    });
});

describe('toCanvasPoint', () => {
    it('maps CSS pixels onto the larger backing store', () => {
        const rect = { left: 100, top: 50, width: 256, height: 448 };
        expect(toCanvasPoint(100, 50, rect, { width: 1024, height: 1792 })).toEqual({ x: 0, y: 0 });
        expect(toCanvasPoint(228, 274, rect, { width: 1024, height: 1792 })).toEqual({ x: 512, y: 896 });
    });

    it('returns the origin for a zero-size rect instead of dividing by zero', () => {
        expect(toCanvasPoint(10, 10, { left: 0, top: 0, width: 0, height: 0 }, { width: 10, height: 10 })).toEqual({ x: 0, y: 0 });
    });
});

describe('brushWidthFor', () => {
    it('scales with the canvas so a stroke keeps the same visual weight', () => {
        const medium = BRUSH_SIZES[1];
        expect(brushWidthFor(medium.key, { width: 1024, height: 1024 })).toBe(medium.width);
        expect(brushWidthFor(medium.key, { width: 512, height: 512 })).toBe(medium.width / 2);
        expect(brushWidthFor(medium.key, { width: 1024, height: 1792 })).toBeCloseTo(medium.width * 1.75);
    });

    it('never drops below one device pixel', () => {
        expect(brushWidthFor('thin', { width: 32, height: 32 })).toBe(1);
    });
});

describe('StrokeHistory', () => {
    it('pops frames in reverse order', () => {
        const history = new StrokeHistory<string>();
        expect(history.canUndo).toBe(false);
        history.push('a');
        history.push('b');
        expect(history.canUndo).toBe(true);
        expect(history.pop()).toBe('b');
        expect(history.pop()).toBe('a');
        expect(history.pop()).toBeUndefined();
        expect(history.canUndo).toBe(false);
    });

    it('drops the oldest frame once over capacity', () => {
        const history = new StrokeHistory<number>(2);
        history.push(1);
        history.push(2);
        history.push(3);
        expect(history.length).toBe(2);
        expect(history.pop()).toBe(3);
        expect(history.pop()).toBe(2);
    });

    it('clears everything at once', () => {
        const history = new StrokeHistory<number>();
        history.push(1);
        history.clear();
        expect(history.canUndo).toBe(false);
    });
});
