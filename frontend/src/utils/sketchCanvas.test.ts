import { describe, expect, it } from 'vitest';
import {
    applyStrokeStyle,
    applyTransform,
    fitTransform,
    IDENTITY_TRANSFORM,
    renderStroke,
    renderStrokes,
    strokeWidthFor,
    transformStrokes,
    BRUSH_SIZES,
    ERASER_WIDTH_MULTIPLIER,
    SKETCH_BACKGROUND,
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

// A 2D context stub: the assertions are about what gets asked of canvas, not
// about pixels, so jsdom's missing canvas implementation is not in the way.
const fakeContext = () => {
    const calls: string[] = [];
    const state: Record<string, unknown> = {};
    return {
        calls,
        state,
        ctx: {
            set lineCap(v: string) { state.lineCap = v; },
            set lineJoin(v: string) { state.lineJoin = v; },
            set globalCompositeOperation(v: string) { state.composite = v; },
            set strokeStyle(v: string) { state.strokeStyle = v; },
            set lineWidth(v: number) { state.lineWidth = v; },
            beginPath: () => { calls.push('begin'); },
            moveTo: (x: number, y: number) => { calls.push(`move ${Math.round(x)},${Math.round(y)}`); },
            lineTo: (x: number, y: number) => { calls.push(`line ${Math.round(x)},${Math.round(y)}`); },
            stroke: () => { calls.push('stroke'); },
        } as unknown as CanvasRenderingContext2D,
    };
};

const DIMS = { width: 1024, height: 1024 };

describe('strokes as data', () => {
    it('makes the eraser wider than the pen at the same brush size', () => {
        const pen = strokeWidthFor({ tool: 'pen', brush: 'medium' }, DIMS);
        const eraser = strokeWidthFor({ tool: 'eraser', brush: 'medium' }, DIMS);
        expect(eraser).toBeGreaterThan(pen);
        expect(eraser / pen).toBeCloseTo(ERASER_WIDTH_MULTIPLIER, 6);
    });

    it('paints the eraser in the background colour, not the pen colour', () => {
        const { ctx, state } = fakeContext();
        applyStrokeStyle(ctx, { tool: 'eraser', color: '#dc2626', brush: 'thin' }, DIMS);
        expect(state.strokeStyle).toBe(SKETCH_BACKGROUND);
        applyStrokeStyle(ctx, { tool: 'pen', color: '#dc2626', brush: 'thin' }, DIMS);
        expect(state.strokeStyle).toBe('#dc2626');
    });

    it('replays a stroke through every point in order', () => {
        const { ctx, calls } = fakeContext();
        renderStroke(ctx, {
            tool: 'pen',
            color: '#111827',
            brush: 'medium',
            points: [{ x: 0, y: 0 }, { x: 10, y: 20 }, { x: 30, y: 40 }],
        }, DIMS);
        expect(calls).toEqual(['begin', 'move 0,0', 'line 10,20', 'line 30,40', 'stroke']);
    });

    it('leaves a dot for a stroke with a single point', () => {
        const { ctx, calls } = fakeContext();
        renderStroke(ctx, { tool: 'pen', color: '#111827', brush: 'thin', points: [{ x: 5, y: 5 }] }, DIMS);
        expect(calls).toEqual(['begin', 'move 5,5', 'line 5,5', 'stroke']);
    });

    it('ignores an empty stroke and replays a list in order', () => {
        const { ctx, calls } = fakeContext();
        renderStrokes(ctx, [
            { tool: 'pen', color: '#111827', brush: 'thin', points: [] },
            { tool: 'pen', color: '#111827', brush: 'thin', points: [{ x: 1, y: 1 }] },
            { tool: 'eraser', color: '#111827', brush: 'thin', points: [{ x: 2, y: 2 }] },
        ], DIMS);
        expect(calls).toEqual(['begin', 'move 1,1', 'line 1,1', 'stroke', 'begin', 'move 2,2', 'line 2,2', 'stroke']);
    });
});

describe('moving a sketch between canvas sizes', () => {
    it('is a no-op when the size did not change', () => {
        expect(fitTransform({ width: 1024, height: 1024 }, { width: 1024, height: 1024 }))
            .toBe(IDENTITY_TRANSFORM);
    });

    it('scales uniformly and centres, so nothing is squashed or lost', () => {
        const t = fitTransform({ width: 1024, height: 1024 }, { width: 512, height: 512 });
        expect(t.scale).toBeCloseTo(0.5);
        expect(applyTransform({ x: 1024, y: 1024 }, t)).toEqual({ x: 512, y: 512 });

        const tall = fitTransform({ width: 1024, height: 1024 }, { width: 1024, height: 1792 });
        expect(tall.scale).toBeCloseTo(1);
        // Centred in the taller canvas rather than pinned to the top.
        expect(applyTransform({ x: 512, y: 512 }, tall)).toEqual({ x: 512, y: 896 });
    });

    it('keeps every drawing inside the new canvas', () => {
        const from = { width: 1024, height: 1792 };
        const to = { width: 512, height: 512 };
        const t = fitTransform(from, to);
        for (const corner of [{ x: 0, y: 0 }, { x: from.width, y: from.height }]) {
            const mapped = applyTransform(corner, t);
            expect(mapped.x).toBeGreaterThanOrEqual(0);
            expect(mapped.x).toBeLessThanOrEqual(to.width);
            expect(mapped.y).toBeGreaterThanOrEqual(0);
            expect(mapped.y).toBeLessThanOrEqual(to.height);
        }
    });

    it('maps stroke points and leaves the rest of the stroke alone', () => {
        const t = fitTransform({ width: 1024, height: 1024 }, { width: 512, height: 512 });
        const [stroke] = transformStrokes([
            { tool: 'eraser', color: '#dc2626', brush: 'thick', points: [{ x: 100, y: 200 }] },
        ], t);
        expect(stroke.points).toEqual([{ x: 50, y: 100 }]);
        expect(stroke.tool).toBe('eraser');
        expect(stroke.brush).toBe('thick');
    });

    it('refuses to divide by a zero-sized canvas', () => {
        expect(fitTransform({ width: 0, height: 0 }, { width: 512, height: 512 })).toBe(IDENTITY_TRANSFORM);
    });
});
