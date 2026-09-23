// Mask canvas: the alpha channel an /images/edits request carries alongside
// the image it edits.
//
// The wire contract (see .design/image-mask.md §1) is one sentence: pixels with
// alpha 0 are the ones the model may repaint, everything opaque is kept, and
// the mask has to match the image's pixel size exactly. The editor never shows
// the user that convention — they paint *what may change* and this module
// inverts it on the way out.
//
// Strokes are the same data the sketch canvas uses, so a mask stays editable
// after it has been applied, and brush widths behave identically on a 512 and
// a 1792 image. What differs is the ground: a sketch is opaque white and its
// eraser paints white, whereas a mask is transparent and its eraser really has
// to erase (`destination-out`) — paint white here and the region would mean
// the opposite of what the user drew.

import { strokeWidthFor, type CanvasDimensions, type Stroke } from './sketchCanvas';

export interface MaskLayers {
    // The pixel size the strokes were authored on — the reference image's own
    // size, not the Playground's output Size. A mask that does not match its
    // image is rejected by the API, so this travels with the strokes.
    size: CanvasDimensions;
    strokes: Stroke[];
    // Flips which side of the strokes is editable: normally the painted region
    // is what may change; inverted, everything else is.
    inverted: boolean;
}

// Warm and clearly artificial, so the painted region reads as an overlay
// rather than as part of the photograph underneath.
export const MASK_PAINT_COLOR = '#f43f5e';
export const MASK_PREVIEW_ALPHA = 0.5;

// A mask made only of erasures selects nothing, and an empty selection is not
// a mask — it is a request to repaint the whole image, which the caller can
// express by not sending a mask at all.
export const hasMaskContent = (strokes: readonly Stroke[]): boolean =>
    strokes.some((stroke) => stroke.tool === 'pen' && stroke.points.length > 0);

// Which way round the strokes are composited.
//
//   'add'      — onto an empty surface: the pen lays pixels down, the eraser
//                lifts them. What the user sees while painting.
//   'subtract' — onto a surface already filled with `color`: the pen punches
//                its region out, and the eraser puts that same colour back.
//                Because the ground is one flat colour, "put back" is exactly
//                a source-over stroke, so both modes are one pass in stroke
//                order and an erase over a paint behaves identically either
//                way round.
export type MaskCoverageMode = 'add' | 'subtract';

// Paints the strokes as a coverage map. Callers decide what the covered region
// *means* (export and preview below) — this only answers "which pixels did the
// user paint", in whichever direction the caller needs it.
export const renderMaskCoverage = (
    ctx: CanvasRenderingContext2D,
    strokes: readonly Stroke[],
    dims: CanvasDimensions,
    color: string,
    mode: MaskCoverageMode = 'add',
): void => {
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    const paints = mode === 'add' ? 'source-over' : 'destination-out';
    const erases = mode === 'add' ? 'destination-out' : 'source-over';
    for (const stroke of strokes) {
        const { points } = stroke;
        if (points.length === 0) continue;
        ctx.globalCompositeOperation = stroke.tool === 'eraser' ? erases : paints;
        ctx.strokeStyle = color;
        ctx.lineWidth = strokeWidthFor(stroke, dims);
        ctx.beginPath();
        ctx.moveTo(points[0].x, points[0].y);
        // A tap with no movement still has to leave a dot.
        if (points.length === 1) ctx.lineTo(points[0].x + 0.01, points[0].y + 0.01);
        for (let i = 1; i < points.length; i += 1) ctx.lineTo(points[i].x, points[i].y);
        ctx.stroke();
    }
    ctx.globalCompositeOperation = 'source-over';
};

// The pixels that go on the wire. Editable region → alpha 0.
//
// Not inverted: start opaque and punch the painted region out of it.
// Inverted: draw only the painted region opaque, leaving the rest transparent.
// Both produce a mask whose RGB is irrelevant (the API reads alpha alone), so
// the colour here is only about producing solid pixels.
export const renderMaskExport = (
    ctx: CanvasRenderingContext2D,
    layers: Pick<MaskLayers, 'strokes' | 'inverted'>,
    dims: CanvasDimensions,
): void => {
    ctx.clearRect(0, 0, dims.width, dims.height);
    if (!layers.inverted) {
        ctx.globalCompositeOperation = 'source-over';
        ctx.fillStyle = '#000000';
        ctx.fillRect(0, 0, dims.width, dims.height);
        renderMaskCoverage(ctx, layers.strokes, dims, '#000000', 'subtract');
        return;
    }
    renderMaskCoverage(ctx, layers.strokes, dims, '#000000', 'add');
};

// What the user sees, in both the dialog and on the thumbnail: the region that
// *will change*, tinted. Inverting therefore visibly moves the tint to the
// other side of the strokes, which is the only honest way to show what the
// switch did.
export const renderMaskPreview = (
    ctx: CanvasRenderingContext2D,
    layers: Pick<MaskLayers, 'strokes' | 'inverted'>,
    dims: CanvasDimensions,
    color: string = MASK_PAINT_COLOR,
): void => {
    ctx.clearRect(0, 0, dims.width, dims.height);
    if (!layers.inverted) {
        renderMaskCoverage(ctx, layers.strokes, dims, color, 'add');
        return;
    }
    ctx.globalCompositeOperation = 'source-over';
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, dims.width, dims.height);
    renderMaskCoverage(ctx, layers.strokes, dims, color, 'subtract');
};

const createCanvas = (dims: CanvasDimensions): HTMLCanvasElement => {
    const canvas = document.createElement('canvas');
    canvas.width = dims.width;
    canvas.height = dims.height;
    return canvas;
};

// Renders onto a fresh canvas so neither export nor preview can disturb the
// live one the user is painting on.
const renderToCanvas = (
    dims: CanvasDimensions,
    draw: (ctx: CanvasRenderingContext2D) => void,
): HTMLCanvasElement | null => {
    const canvas = createCanvas(dims);
    const ctx = canvas.getContext('2d');
    if (!ctx) return null;
    draw(ctx);
    return canvas;
};

export const maskPreviewDataURL = (
    layers: Pick<MaskLayers, 'strokes' | 'inverted'>,
    dims: CanvasDimensions,
): string => {
    const canvas = renderToCanvas(dims, (ctx) => renderMaskPreview(ctx, layers, dims));
    return canvas ? canvas.toDataURL('image/png') : '';
};

// PNG because alpha is the payload: a JPEG mask carries no channel to read.
export const maskToFile = async (
    layers: Pick<MaskLayers, 'strokes' | 'inverted'>,
    dims: CanvasDimensions,
    name = `mask-${Date.now()}.png`,
): Promise<File | null> => {
    const canvas = renderToCanvas(dims, (ctx) => renderMaskExport(ctx, layers, dims));
    if (!canvas) return null;
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
    return blob ? new File([blob], name, { type: 'image/png' }) : null;
};
