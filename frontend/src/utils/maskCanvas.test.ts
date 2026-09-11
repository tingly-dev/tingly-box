import { describe, expect, it } from 'vitest';
import {
    hasMaskContent,
    renderMaskExport,
    renderMaskPreview,
    type MaskLayers,
} from './maskCanvas';
import type { Stroke } from './sketchCanvas';

const dims = { width: 100, height: 100 };

const stroke = (tool: Stroke['tool']): Stroke => ({
    tool,
    color: '#f43f5e',
    brush: 'medium',
    points: [{ x: 10, y: 10 }, { x: 40, y: 40 }],
});

// Canvas rendering does not run under jsdom, and the thing that can actually be
// wrong here is not a pixel but a direction: which composite operation each
// stroke is drawn with. A recorder answers exactly that, in order.
interface Op { op: string; composite: string }

const recorder = () => {
    const ops: Op[] = [];
    const ctx = {
        lineCap: '',
        lineJoin: '',
        strokeStyle: '',
        fillStyle: '',
        lineWidth: 0,
        globalCompositeOperation: 'source-over',
        clearRect: () => ops.push({ op: 'clear', composite: ctx.globalCompositeOperation }),
        fillRect: () => ops.push({ op: 'fill', composite: ctx.globalCompositeOperation }),
        beginPath: () => {},
        moveTo: () => {},
        lineTo: () => {},
        stroke: () => ops.push({ op: 'stroke', composite: ctx.globalCompositeOperation }),
    };
    return { ctx: ctx as unknown as CanvasRenderingContext2D, ops };
};

const layers = (strokes: Stroke[], inverted: boolean): MaskLayers => ({ size: dims, strokes, inverted });

describe('hasMaskContent', () => {
    it('is false for nothing and for erasures alone', () => {
        expect(hasMaskContent([])).toBe(false);
        expect(hasMaskContent([stroke('eraser')])).toBe(false);
        expect(hasMaskContent([{ ...stroke('pen'), points: [] }])).toBe(false);
    });

    it('is true once something is painted', () => {
        expect(hasMaskContent([stroke('pen')])).toBe(true);
    });
});

describe('renderMaskExport', () => {
    it('punches the painted region out of an opaque sheet', () => {
        const { ctx, ops } = recorder();
        renderMaskExport(ctx, layers([stroke('pen')], false), dims);

        // Opaque everywhere first, then the painted region removed: painted
        // pixels end up alpha 0, which is what the API reads as "repaint here".
        expect(ops.map((entry) => entry.op)).toEqual(['clear', 'fill', 'stroke']);
        expect(ops[1].composite).toBe('source-over');
        expect(ops[2].composite).toBe('destination-out');
    });

    it('puts the sheet back where the eraser went', () => {
        const { ctx, ops } = recorder();
        renderMaskExport(ctx, layers([stroke('pen'), stroke('eraser')], false), dims);

        // The ground is one flat colour, so restoring coverage is a plain
        // stroke — the eraser composites the opposite way from the pen.
        expect(ops.filter((entry) => entry.op === 'stroke').map((entry) => entry.composite))
            .toEqual(['destination-out', 'source-over']);
    });

    it('inverted, draws only the painted region opaque', () => {
        const { ctx, ops } = recorder();
        renderMaskExport(ctx, layers([stroke('pen'), stroke('eraser')], true), dims);

        // No full-surface fill: everything outside the strokes stays
        // transparent, which is now the editable side.
        expect(ops.some((entry) => entry.op === 'fill')).toBe(false);
        expect(ops.filter((entry) => entry.op === 'stroke').map((entry) => entry.composite))
            .toEqual(['source-over', 'destination-out']);
    });
});

describe('renderMaskPreview', () => {
    it('tints what the user painted', () => {
        const { ctx, ops } = recorder();
        renderMaskPreview(ctx, layers([stroke('pen')], false), dims);

        expect(ops.some((entry) => entry.op === 'fill')).toBe(false);
        expect(ops.filter((entry) => entry.op === 'stroke').map((entry) => entry.composite))
            .toEqual(['source-over']);
    });

    it('inverted, tints everything else instead', () => {
        const { ctx, ops } = recorder();
        renderMaskPreview(ctx, layers([stroke('pen')], true), dims);

        // The tint visibly moves to the other side of the strokes — the only
        // honest way to show what Invert did.
        expect(ops.map((entry) => entry.op)).toEqual(['clear', 'fill', 'stroke']);
        expect(ops[2].composite).toBe('destination-out');
    });
});
