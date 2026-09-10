// Client-side sticker-sheet slicing: cut one generated grid image into its
// individual tiles, entirely in the browser (canvas), so the user gets usable
// assets without a round trip or a second tool.
//
// The cut is a plain even grid — deliberately. Models do not place a sheet's
// cells on an exact lattice, so instead of guessing at content boundaries the
// user drags the frame the grid divides and nudges one gutter, with a live
// overlay showing where every cut lands.

import { fetchBlob } from './download';
import {
    analyzeBackground,
    removeBackground,
    type BackgroundAnalysis,
    type BackgroundKind,
    type RGBAImage,
} from './imageMatte';
import type { GifFrame } from './gif';

/**
 * The part of the image the grid is cut out of, in fractions of the image's
 * own width/height. Models rarely fill the canvas with the sheet — the useful
 * region is often off-centre — so the frame is a free rectangle rather than a
 * symmetric margin.
 */
export interface CropRect {
    x: number;
    y: number;
    width: number;
    height: number;
}

export interface GridSpec {
    rows: number;
    cols: number;
    /** The region being cut up. Defaults to the whole image. */
    crop: CropRect;
    /** Fraction of a cell removed as spacing between neighbouring cells. */
    gutter: number;
}

export interface TileRect {
    index: number;
    row: number;
    col: number;
    x: number;
    y: number;
    width: number;
    height: number;
}

// 3x3 is the shape a grid image almost always comes back as, and the whole
// image is the frame to start from. The gutter maximum is the single source of
// truth for both the slider bound and the clamp below, so the UI can never
// offer a value the geometry would quietly reject.
export const FULL_CROP: CropRect = { x: 0, y: 0, width: 1, height: 1 };
export const DEFAULT_GRID: GridSpec = { rows: 3, cols: 3, crop: FULL_CROP, gutter: 0 };
export const GUTTER_MAX = 0.4;
/** No edge of the frame may pass another; this is how close they may come. */
export const CROP_MIN = 0.05;

const clamp = (value: number, min: number, max: number): number => Math.min(Math.max(value, min), max);

/**
 * Clamps a frame into the image and keeps it at least `CROP_MIN` on each axis,
 * so a drag that overshoots (or a stale value) can never produce a grid with
 * zero-sized cells.
 */
export const normalizeCrop = (crop: CropRect | undefined): CropRect => {
    if (!crop) return FULL_CROP;
    const x = clamp(crop.x, 0, 1 - CROP_MIN);
    const y = clamp(crop.y, 0, 1 - CROP_MIN);
    return {
        x,
        y,
        width: clamp(crop.width, CROP_MIN, 1 - x),
        height: clamp(crop.height, CROP_MIN, 1 - y),
    };
};

export const isFullCrop = (crop: CropRect): boolean => (
    crop.x <= 0 && crop.y <= 0 && crop.width >= 1 && crop.height >= 1
);

/**
 * Cut rectangles for an evenly divided grid, in source-image pixels.
 *
 * The grid divides the frame, not the image: everything outside it is left
 * out entirely. The gutter is taken out of each cell symmetrically (half per
 * side), which keeps every tile the same size.
 */
export const computeTileRects = (width: number, height: number, spec: GridSpec): TileRect[] => {
    const rows = Math.max(1, Math.floor(spec.rows));
    const cols = Math.max(1, Math.floor(spec.cols));
    const crop = normalizeCrop(spec.crop);
    const originX = crop.x * width;
    const originY = crop.y * height;
    const innerWidth = Math.max(1, crop.width * width);
    const innerHeight = Math.max(1, crop.height * height);
    const cellWidth = innerWidth / cols;
    const cellHeight = innerHeight / rows;
    const gutter = clamp(spec.gutter, 0, GUTTER_MAX);
    const insetX = (cellWidth * gutter) / 2;
    const insetY = (cellHeight * gutter) / 2;

    const rects: TileRect[] = [];
    for (let row = 0; row < rows; row += 1) {
        for (let col = 0; col < cols; col += 1) {
            const x = Math.round(originX + col * cellWidth + insetX);
            const y = Math.round(originY + row * cellHeight + insetY);
            const right = Math.round(originX + (col + 1) * cellWidth - insetX);
            const bottom = Math.round(originY + (row + 1) * cellHeight - insetY);
            rects.push({
                index: row * cols + col,
                row,
                col,
                x: clamp(x, 0, width),
                y: clamp(y, 0, height),
                width: Math.max(1, clamp(right, 0, width) - clamp(x, 0, width)),
                height: Math.max(1, clamp(bottom, 0, height) - clamp(y, 0, height)),
            });
        }
    }
    return rects;
};

/**
 * Loads any playground image source (data URL or provider URL) as an
 * <img>. Provider URLs are fetched first and handed to the image as an object
 * URL: drawing a cross-origin URL directly taints the canvas and makes
 * toBlob() throw, which is exactly the step this module exists to perform.
 */
export const loadImage = async (src: string): Promise<{ image: HTMLImageElement; release: () => void }> => {
    const objectUrl = URL.createObjectURL(await fetchBlob(src));
    try {
        const image = await new Promise<HTMLImageElement>((resolve, reject) => {
            const element = new Image();
            element.onload = () => resolve(element);
            element.onerror = () => reject(new Error('failed to decode image'));
            element.src = objectUrl;
        });
        return { image, release: () => URL.revokeObjectURL(objectUrl) };
    } catch (error) {
        URL.revokeObjectURL(objectUrl);
        throw error;
    }
};

const canvasToBlob = (canvas: HTMLCanvasElement): Promise<Blob> => new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
        if (blob) resolve(blob);
        else reject(new Error('failed to encode tile'));
    }, 'image/png');
});

const context2d = (canvas: HTMLCanvasElement): CanvasRenderingContext2D => {
    const context = canvas.getContext('2d', { willReadFrequently: true });
    if (!context) throw new Error('canvas 2d context unavailable');
    return context;
};

/** Matte settings carried alongside the cut, when background cleanup is on. */
export interface MatteSpec {
    kind: BackgroundKind;
    tolerance: number;
    colors?: [number, number, number][];
}

export interface TileRenderOptions {
    /** Scales the tile so its longer side matches this many pixels. */
    exportSize?: number | null;
    matte?: MatteSpec | null;
    /** Forces an exact output box — frames of one animation must all match. */
    box?: { width: number; height: number } | null;
}

const outputSize = (rect: TileRect, options: TileRenderOptions): { width: number; height: number } => {
    if (options.box) return options.box;
    const scale = options.exportSize ? options.exportSize / Math.max(rect.width, rect.height) : 1;
    return {
        width: Math.max(1, Math.round(rect.width * scale)),
        height: Math.max(1, Math.round(rect.height * scale)),
    };
};

/**
 * Draws one tile at its output size and applies the matte, if any. The matte
 * runs here rather than on the whole sheet because a tile's own edge is what
 * seeds the flood fill: per-tile, the background always reaches the border.
 */
export const renderTilePixels = (
    image: HTMLImageElement,
    rect: TileRect,
    options: TileRenderOptions = {},
): RGBAImage => {
    const { width, height } = outputSize(rect, options);
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const context = context2d(canvas);
    context.imageSmoothingQuality = 'high';
    context.drawImage(image, rect.x, rect.y, rect.width, rect.height, 0, 0, width, height);
    const pixels = context.getImageData(0, 0, width, height);
    if (!options.matte || options.matte.kind === 'none') return pixels;
    return removeBackground(pixels, options.matte);
};

const pixelsToCanvas = (pixels: RGBAImage): HTMLCanvasElement => {
    const canvas = document.createElement('canvas');
    canvas.width = pixels.width;
    canvas.height = pixels.height;
    const context = context2d(canvas);
    // Filled through createImageData rather than `new ImageData(data, …)`:
    // the constructor overload insists on a plain ArrayBuffer backing.
    const target = context.createImageData(pixels.width, pixels.height);
    target.data.set(pixels.data);
    context.putImageData(target, 0, 0);
    return canvas;
};

/** Renders one tile as a PNG. */
export const renderTile = async (
    image: HTMLImageElement,
    rect: TileRect,
    options: TileRenderOptions = {},
): Promise<Blob> => canvasToBlob(pixelsToCanvas(renderTilePixels(image, rect, options)));

/** A data URL of one tile, for the in-dialog animation preview. */
export const renderTileDataUrl = (
    image: HTMLImageElement,
    rect: TileRect,
    options: TileRenderOptions = {},
): string => pixelsToCanvas(renderTilePixels(image, rect, options)).toDataURL('image/png');

/**
 * Frames for an animation: every tile rendered into one shared box, because a
 * GIF has a single canvas and rounding leaves tiles a pixel apart.
 */
export const renderAnimationFrames = (
    image: HTMLImageElement,
    rects: TileRect[],
    options: TileRenderOptions = {},
): { width: number; height: number; frames: GifFrame[] } => {
    if (rects.length === 0) throw new Error('animation needs at least one tile');
    const box = options.box ?? outputSize(rects[0], options);
    const frames = rects.map((rect) => ({
        data: renderTilePixels(image, rect, { ...options, box }).data,
    }));
    return { width: box.width, height: box.height, frames };
};

export interface SheetAnalysis extends BackgroundAnalysis {
    /** Whether any pixel is see-through — a video export then needs a backdrop. */
    hasAlpha: boolean;
}

/** Reads the whole sheet's pixels once, for its background and its alpha. */
export const analyzeSheetBackground = (image: HTMLImageElement): SheetAnalysis => {
    const canvas = document.createElement('canvas');
    canvas.width = image.naturalWidth;
    canvas.height = image.naturalHeight;
    const context = context2d(canvas);
    context.drawImage(image, 0, 0);
    const pixels = context.getImageData(0, 0, canvas.width, canvas.height);
    return { ...analyzeBackground(pixels), hasAlpha: imageHasAlpha(pixels.data) };
};

export const imageHasAlpha = (data: Uint8ClampedArray): boolean => {
    for (let i = 3; i < data.length; i += 4) {
        if (data[i] < 255) return true;
    }
    return false;
};

export const tileFileName = (stem: string, index: number, total: number): string => {
    const width = String(total).length;
    return `${stem}-${String(index + 1).padStart(width, '0')}.png`;
};

// Frame duration is typed or stepped in milliseconds rather than picked from
// a list. GIF stores it in hundredths of a second, so it snaps to 10 ms; the
// floor is what browsers still honour (they clamp anything faster), and the
// ceiling is long enough for a slideshow.
export const FRAME_DELAY_MIN = 20;
export const FRAME_DELAY_MAX = 5000;
export const FRAME_DELAY_STEP = 10;
export const DEFAULT_FRAME_DELAY = 200;
export const clampFrameDelay = (value: number): number => {
    if (!Number.isFinite(value)) return DEFAULT_FRAME_DELAY;
    const snapped = Math.round(value / FRAME_DELAY_STEP) * FRAME_DELAY_STEP;
    return Math.min(FRAME_DELAY_MAX, Math.max(FRAME_DELAY_MIN, snapped));
};
