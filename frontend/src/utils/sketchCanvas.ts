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
