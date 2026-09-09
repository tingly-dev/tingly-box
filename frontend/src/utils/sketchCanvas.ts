// Pure helpers behind the sketch canvas: sizing, pointer mapping and the
// undo history. Kept away from the DOM so they can be unit-tested; the
// component itself is a thin shell over these plus the 2D context calls.

export interface CanvasDimensions {
    width: number;
    height: number;
}

export const DEFAULT_SKETCH_DIMENSIONS: CanvasDimensions = { width: 1024, height: 1024 };

// The sketch canvas takes the Playground's output `size` ("1024x1792") as its
// own backing-store size, so the reference the model receives already has the
// shape the result is asked for — no letterboxing, no surprise crops.
export const parseImageSize = (size: string): CanvasDimensions => {
    const match = /^\s*(\d+)\s*[x×]\s*(\d+)\s*$/i.exec(size);
    if (!match) return DEFAULT_SKETCH_DIMENSIONS;
    const width = Number(match[1]);
    const height = Number(match[2]);
    if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
        return DEFAULT_SKETCH_DIMENSIONS;
    }
    return { width, height };
};

// CSS size for a canvas that must fit inside `box` without changing shape.
// Never scales up beyond the backing store: a 512 canvas shown at 1400 CSS
// px would just be blurry.
export const fitWithin = (dims: CanvasDimensions, box: CanvasDimensions): CanvasDimensions => {
    if (box.width <= 0 || box.height <= 0) return { width: 0, height: 0 };
    const scale = Math.min(box.width / dims.width, box.height / dims.height, 1);
    return {
        width: Math.max(1, Math.floor(dims.width * scale)),
        height: Math.max(1, Math.floor(dims.height * scale)),
    };
};

export interface CanvasPoint {
    x: number;
    y: number;
}

// Maps a pointer position (viewport coordinates) onto the canvas backing
// store, which is usually larger than the CSS box it is displayed in.
export const toCanvasPoint = (
    clientX: number,
    clientY: number,
    rect: { left: number; top: number; width: number; height: number },
    dims: CanvasDimensions,
): CanvasPoint => {
    if (rect.width <= 0 || rect.height <= 0) return { x: 0, y: 0 };
    return {
        x: ((clientX - rect.left) / rect.width) * dims.width,
        y: ((clientY - rect.top) / rect.height) * dims.height,
    };
};

// Brush widths are expressed relative to a 1024 canvas and scaled with the
// backing store, so "medium" draws the same visual weight on 512 and 1792.
export const BRUSH_SIZES = [
    { key: 'thin', width: 4 },
    { key: 'medium', width: 10 },
    { key: 'thick', width: 24 },
] as const;
export type BrushSizeKey = (typeof BRUSH_SIZES)[number]['key'];

export const brushWidthFor = (key: BrushSizeKey, dims: CanvasDimensions): number => {
    const base = BRUSH_SIZES.find((size) => size.key === key)?.width ?? BRUSH_SIZES[1].width;
    const scale = Math.max(dims.width, dims.height) / 1024;
    return Math.max(1, base * scale);
};

// The eraser paints the background colour: the canvas is opaque white on
// purpose (a transparent sketch is ambiguous to most image models), so
// "erase" and "paint white" are the same thing. Wider than the pen it
// replaces, because erasing is a coarser gesture than drawing.
export const ERASER_WIDTH_MULTIPLIER = 2.5;

export const SKETCH_BACKGROUND = '#ffffff';

// A small, opinionated palette: black for outlines plus a handful of clearly
// distinct hues that a model can name back ("the red circle"). Not a colour
// picker — a picker is a decision the user did not come here to make.
export const SKETCH_COLORS = ['#111827', '#dc2626', '#2563eb', '#16a34a', '#f59e0b', '#9333ea'] as const;

// Snapshot-per-stroke undo. Each entry is the frame *before* a stroke, so
// undo is a single putImageData. Capped, because a 1024² ImageData is 4 MB
// and an enthusiastic scribbler makes a lot of strokes.
export class StrokeHistory<T> {
    private readonly frames: T[] = [];

    constructor(private readonly capacity = 30) {}

    get length(): number {
        return this.frames.length;
    }

    get canUndo(): boolean {
        return this.frames.length > 0;
    }

    push(frame: T): void {
        this.frames.push(frame);
        if (this.frames.length > this.capacity) this.frames.shift();
    }

    pop(): T | undefined {
        return this.frames.pop();
    }

    clear(): void {
        this.frames.length = 0;
    }
}

// --- strokes as data ---------------------------------------------------------
//
// A stroke is kept as the points the user drew, not as the pixels it left
// behind. That is what lets a saved sketch come back editable: undo keeps
// working across a save, the layer stored next to a reference image is a list
// rather than a second full-size PNG, and the marks are resolution-independent
// if the canvas is ever re-rendered at another size.

export type StrokeTool = 'pen' | 'eraser';

export interface Stroke {
    tool: StrokeTool;
    // The pen's colour. An eraser paints the background, so it carries the
    // background colour and this field is only along for the ride.
    color: string;
    brush: BrushSizeKey;
    points: CanvasPoint[];
}

export const strokeWidthFor = (stroke: Pick<Stroke, 'tool' | 'brush'>, dims: CanvasDimensions): number => {
    const width = brushWidthFor(stroke.brush, dims);
    return stroke.tool === 'eraser' ? width * ERASER_WIDTH_MULTIPLIER : width;
};

// Shared by the live gesture and by replay, so a stroke being drawn and the
// same stroke redrawn after an undo cannot come out different.
export const applyStrokeStyle = (
    ctx: CanvasRenderingContext2D,
    stroke: Pick<Stroke, 'tool' | 'color' | 'brush'>,
    dims: CanvasDimensions,
): void => {
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.globalCompositeOperation = 'source-over';
    ctx.strokeStyle = stroke.tool === 'eraser' ? SKETCH_BACKGROUND : stroke.color;
    ctx.lineWidth = strokeWidthFor(stroke, dims);
};

export const renderStroke = (ctx: CanvasRenderingContext2D, stroke: Stroke, dims: CanvasDimensions): void => {
    if (stroke.points.length === 0) return;
    applyStrokeStyle(ctx, stroke, dims);
    const [first, ...rest] = stroke.points;
    ctx.beginPath();
    ctx.moveTo(first.x, first.y);
    // A tap with no movement still has to leave a dot.
    if (rest.length === 0) ctx.lineTo(first.x + 0.01, first.y + 0.01);
    for (const point of rest) ctx.lineTo(point.x, point.y);
    ctx.stroke();
};

export const renderStrokes = (
    ctx: CanvasRenderingContext2D,
    strokes: readonly Stroke[],
    dims: CanvasDimensions,
): void => {
    for (const stroke of strokes) renderStroke(ctx, stroke, dims);
};

// --- moving a sketch between canvas sizes -----------------------------------
//
// Layers are absolute canvas coordinates, so re-opening a saved sketch after
// the Playground's Size changed would otherwise drop half the drawing off a
// smaller canvas, or strand it in the corner of a bigger one. The mapping is
// uniform (never squashed to fit a new aspect: a squashed figure would stop
// having consistent bone lengths) and centres what it scales.

export interface CanvasTransform { scale: number; dx: number; dy: number }

export const IDENTITY_TRANSFORM: CanvasTransform = { scale: 1, dx: 0, dy: 0 };

export const fitTransform = (from: CanvasDimensions, to: CanvasDimensions): CanvasTransform => {
    if (from.width <= 0 || from.height <= 0) return IDENTITY_TRANSFORM;
    if (from.width === to.width && from.height === to.height) return IDENTITY_TRANSFORM;
    const scale = Math.min(to.width / from.width, to.height / from.height);
    return {
        scale,
        dx: (to.width - from.width * scale) / 2,
        dy: (to.height - from.height * scale) / 2,
    };
};

export const applyTransform = (point: CanvasPoint, transform: CanvasTransform): CanvasPoint => ({
    x: point.x * transform.scale + transform.dx,
    y: point.y * transform.scale + transform.dy,
});

export const transformStrokes = (strokes: readonly Stroke[], transform: CanvasTransform): Stroke[] => {
    if (transform === IDENTITY_TRANSFORM) return strokes as Stroke[];
    return strokes.map((stroke) => ({
        ...stroke,
        points: stroke.points.map((point) => applyTransform(point, transform)),
    }));
};
