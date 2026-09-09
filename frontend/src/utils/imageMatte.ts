// Background cleanup for playground output: the two fake backgrounds models
// keep painting instead of an alpha channel.
//
// A model asked for "transparent background" very often answers with a picture
// *of* transparency — the grey checkerboard — or with a green screen, because
// that is what its training images look like. Both are one step away from the
// asset the user actually wanted, and both are mechanical to undo, so the
// playground undoes them rather than sending the user to an image editor.
//
// Everything here is pure over a structural RGBA image, so the geometry can be
// unit-tested without a canvas.

export interface RGBAImage {
    data: Uint8ClampedArray;
    width: number;
    height: number;
}

export type BackgroundKind = 'none' | 'checker' | 'green';

export interface BackgroundAnalysis {
    kind: BackgroundKind;
    /** The two checkerboard tones, when that is what the border holds. */
    colors: [number, number, number][];
}

export const DEFAULT_TOLERANCE = 0.5;
// Chroma radius around the key colour, at tolerance 0 and at 1. A flat green
// screen sits within a few hundredths of its key; a cyan glow or a white arc
// blended with it is half a unit away, so the whole usable range lives here.
const CHROMA_LIMIT_MIN = 0.03;
const CHROMA_LIMIT_RANGE = 0.19;

/** Border ring sampled as "what surrounds the subject", at least 2px wide. */
const borderWidth = (image: RGBAImage): number => Math.max(
    2,
    Math.round(Math.min(image.width, image.height) * 0.03),
);

const forEachBorderPixel = (image: RGBAImage, visit: (offset: number) => void): void => {
    const band = borderWidth(image);
    for (let y = 0; y < image.height; y += 1) {
        const insideRow = y >= band && y < image.height - band;
        for (let x = 0; x < image.width; x += 1) {
            if (insideRow && x >= band && x < image.width - band) {
                x = image.width - band - 1;
                continue;
            }
            visit((y * image.width + x) * 4);
        }
    }
};

const isNeutral = (r: number, g: number, b: number): boolean => (
    Math.max(r, g, b) - Math.min(r, g, b) <= 26
);

const isGreenish = (r: number, g: number, b: number): boolean => (
    g > 60 && g - Math.max(r, b) > 32
);

/**
 * The dominant border colours, quantised to 8 levels per channel so JPEG
 * ringing does not split one flat backdrop into a dozen buckets. `accept`
 * narrows the sample to the kind of colour being looked for.
 */
const dominantBorderColors = (
    image: RGBAImage,
    accept: (r: number, g: number, b: number) => boolean,
    take: number,
): { colors: [number, number, number][]; coverage: number } => {
    const counts = new Map<number, { count: number; r: number; g: number; b: number }>();
    let sampled = 0;
    forEachBorderPixel(image, (offset) => {
        sampled += 1;
        const r = image.data[offset];
        const g = image.data[offset + 1];
        const b = image.data[offset + 2];
        if (image.data[offset + 3] < 128 || !accept(r, g, b)) return;
        const key = ((r >> 5) << 6) | ((g >> 5) << 3) | (b >> 5);
        const bucket = counts.get(key) ?? { count: 0, r: 0, g: 0, b: 0 };
        bucket.count += 1;
        bucket.r += r;
        bucket.g += g;
        bucket.b += b;
        counts.set(key, bucket);
    });
    const top = [...counts.values()].sort((a, b) => b.count - a.count).slice(0, take);
    const covered = top.reduce((total, bucket) => total + bucket.count, 0);
    return {
        colors: top.map((bucket) => ([
            Math.round(bucket.r / bucket.count),
            Math.round(bucket.g / bucket.count),
            Math.round(bucket.b / bucket.count),
        ] as [number, number, number])),
        coverage: sampled > 0 ? covered / sampled : 0,
    };
};

const dominantNeutralTones = (image: RGBAImage) => dominantBorderColors(image, isNeutral, 2);
const dominantGreen = (image: RGBAImage) => dominantBorderColors(image, isGreenish, 1);

/**
 * A checkerboard is not just two greys — it *alternates*. Counting the tone
 * changes along the top row is what separates it from a flat grey backdrop,
 * which this feature deliberately leaves alone (the user asked for the fake
 * transparency to go, not for every plain background to be keyed out).
 */
const alternates = (image: RGBAImage, colors: [number, number, number][]): boolean => {
    if (colors.length < 2) return false;
    let changes = 0;
    let previous = -1;
    for (let x = 0; x < image.width; x += 1) {
        const offset = x * 4;
        const luminance = image.data[offset];
        const nearest = Math.abs(luminance - colors[0][0]) <= Math.abs(luminance - colors[1][0]) ? 0 : 1;
        if (previous >= 0 && nearest !== previous) changes += 1;
        previous = nearest;
    }
    return changes >= 4;
};

export const analyzeBackground = (image: RGBAImage): BackgroundAnalysis => {
    let green = 0;
    let opaque = 0;
    forEachBorderPixel(image, (offset) => {
        if (image.data[offset + 3] < 128) return;
        opaque += 1;
        if (isGreenish(image.data[offset], image.data[offset + 1], image.data[offset + 2])) green += 1;
    });
    if (opaque === 0) return { kind: 'none', colors: [] };
    // The key colour is read off the image, not assumed: everything downstream
    // measures distance to *this* green rather than "greener than red and
    // blue", which a white glow painted over a green screen also satisfies.
    if (green / opaque >= 0.6) return { kind: 'green', colors: dominantGreen(image).colors };

    const { colors, coverage } = dominantNeutralTones(image);
    const distinct = colors.length === 2
        && Math.abs(colors[0][0] - colors[1][0]) >= 6
        && Math.abs(colors[0][0] - colors[1][0]) <= 110;
    if (coverage >= 0.75 && distinct && alternates(image, colors)) return { kind: 'checker', colors };
    return { kind: 'none', colors };
};

const clamp01 = (value: number): number => Math.min(1, Math.max(0, value));

/**
 * A pixel's colour with brightness divided out — the (Cb, Cr) chroma of
 * YCbCr, normalised by luma. This is the quantity a green screen is *for*:
 * every pixel of the backdrop shares it, whether that corner of the screen is
 * lit or shadowed, while anything painted on top does not.
 *
 * The luma floor keeps near-black pixels from dividing by nothing and landing
 * on an arbitrary hue; they end up far from any key, which is correct — black
 * is artwork, not backdrop.
 */
const LUMA_FLOOR = 24;
const normalizedChroma = (r: number, g: number, b: number): [number, number] => {
    const luma = Math.max(LUMA_FLOOR, 0.299 * r + 0.587 * g + 0.114 * b);
    return [
        (-0.169 * r - 0.331 * g + 0.5 * b) / luma,
        (0.5 * r - 0.419 * g - 0.081 * b) / luma,
    ];
};

/**
 * How much a pixel reads as background, 1 = certainly, 0 = certainly not.
 * Full transparency inside the inner radius, fading to opaque at twice it —
 * the band is what gives anti-aliased edges a soft alpha, not a staircase.
 *
 * A green screen is keyed the way keying is meant to work: one key colour,
 * measured in chroma, with a tolerance around it. "Greener than it is red or
 * blue" is not that test — a white sword arc or a cyan glow drawn over the
 * screen satisfies it too, which is exactly how a sprite sheet loses its
 * effects. In chroma those blends sit far from the key and survive, while a
 * shadowed patch of the same screen still keys out.
 *
 * The checkerboard is the opposite case: its two tones are neutral, so they
 * have no chroma to compare and are matched by plain RGB distance instead.
 */
const matchScore = (
    kind: Exclude<BackgroundKind, 'none'>,
    colors: [number, number, number][],
    tolerance: number,
    r: number,
    g: number,
    b: number,
): number => {
    let best = Number.POSITIVE_INFINITY;
    if (kind === 'green') {
        const [cb, cr] = normalizedChroma(r, g, b);
        for (const key of colors) {
            const [kb, kr] = normalizedChroma(key[0], key[1], key[2]);
            const distance = Math.hypot(cb - kb, cr - kr);
            if (distance < best) best = distance;
        }
        const limit = CHROMA_LIMIT_MIN + tolerance * CHROMA_LIMIT_RANGE;
        return clamp01((limit * 2 - best) / limit);
    }
    for (const [kr, kg, kb] of colors) {
        const distance = Math.sqrt((r - kr) ** 2 + (g - kg) ** 2 + (b - kb) ** 2);
        if (distance < best) best = distance;
    }
    const limit = 16 + tolerance * 96;
    return clamp01((limit * 2 - best) / limit);
};

/** Pulls a pixel's green back to what its red and blue can justify. */
const despill = (data: Uint8ClampedArray, offset: number): void => {
    const neutralGreen = Math.round((data[offset] + data[offset + 2]) / 2);
    if (data[offset + 1] > neutralGreen) data[offset + 1] = neutralGreen;
};

export interface RemoveBackgroundOptions {
    kind: BackgroundKind;
    /** 0..1; higher keys out more of the near-background fringe. */
    tolerance?: number;
    /** Checker tones, when already known from `analyzeBackground`. */
    colors?: [number, number, number][];
}

/**
 * Clears the background into the alpha channel, returning a new image.
 *
 * The clear is a flood fill seeded from the edges, not a global colour match:
 * a global match punches holes in every grey the artwork itself uses (an eye
 * highlight, a shadow), and the user has no way to see why. Only background
 * that actually reaches the frame's edge is background.
 */
export const removeBackground = (image: RGBAImage, options: RemoveBackgroundOptions): RGBAImage => {
    const output: RGBAImage = {
        width: image.width,
        height: image.height,
        data: new Uint8ClampedArray(image.data),
    };
    if (options.kind === 'none') return output;
    const kind = options.kind;
    const tolerance = clamp01(options.tolerance ?? DEFAULT_TOLERANCE);
    const colors = options.colors?.length
        ? options.colors
        : (kind === 'green' ? dominantGreen(image) : dominantNeutralTones(image)).colors;
    if (colors.length === 0) return output;

    const { width, height, data } = output;
    const pixels = width * height;
    const visited = new Uint8Array(pixels);
    const stack = new Int32Array(pixels);
    let top = 0;

    const push = (index: number) => {
        if (visited[index]) return;
        visited[index] = 1;
        const offset = index * 4;
        if (data[offset + 3] === 0) {
            // Already transparent: it conducts the fill but needs no change.
            stack[top] = index;
            top += 1;
            return;
        }
        const score = matchScore(kind, colors, tolerance, data[offset], data[offset + 1], data[offset + 2]);
        if (score <= 0) return;
        data[offset + 3] = Math.round(data[offset + 3] * (1 - score));
        if (kind === 'green' && data[offset + 3] > 0) despill(data, offset);
        stack[top] = index;
        top += 1;
    };

    for (let x = 0; x < width; x += 1) {
        push(x);
        push((height - 1) * width + x);
    }
    for (let y = 0; y < height; y += 1) {
        push(y * width);
        push(y * width + width - 1);
    }
    while (top > 0) {
        top -= 1;
        const index = stack[top];
        const x = index % width;
        const y = (index - x) / width;
        if (x > 0) push(index - 1);
        if (x < width - 1) push(index + 1);
        if (y > 0) push(index - width);
        if (y < height - 1) push(index + width);
    }

    // A kept pixel that touches cleared background still carries the screen's
    // colour cast, however tight the key was. One pass over that boundary is
    // what removes the green rim a distance-based key otherwise leaves.
    if (kind === 'green') {
        for (let index = 0; index < pixels; index += 1) {
            const offset = index * 4;
            if (data[offset + 3] === 0) continue;
            const x = index % width;
            const y = (index - x) / width;
            const touchesCleared = (x > 0 && data[(index - 1) * 4 + 3] === 0)
                || (x < width - 1 && data[(index + 1) * 4 + 3] === 0)
                || (y > 0 && data[(index - width) * 4 + 3] === 0)
                || (y < height - 1 && data[(index + width) * 4 + 3] === 0);
            if (touchesCleared) despill(data, offset);
        }
    }
    return output;
};
