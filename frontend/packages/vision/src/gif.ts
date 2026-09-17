// Minimal GIF89a writer, so a set of sliced tiles can leave the browser as one
// animation without a build-time dependency.
//
// Same reasoning as `zip.ts`: the encoder we need is the *small* half of the
// spec — one global palette, full-size frames, LZW, a Netscape loop block —
// and a general-purpose GIF library is an order of magnitude more code than
// that. Everything here is pure over typed arrays so it is unit-testable
// without a canvas.

export interface GifFrame {
    /** RGBA pixels, `width * height * 4` bytes, as produced by `getImageData`. */
    data: Uint8ClampedArray;
}

export interface GifOptions {
    width: number;
    height: number;
    frames: GifFrame[];
    /** Per-frame duration. GIF stores hundredths of a second, so this rounds. */
    delayMs: number;
    /** 0 (the default) loops forever; n plays the sequence n times. */
    loop?: number;
}

// Alpha below this becomes the palette's transparent index. GIF has no partial
// transparency — a pixel is either drawn or it is not — so the soft edges a
// matte leaves behind have to land on one side of a threshold.
const ALPHA_THRESHOLD = 128;
const MAX_COLORS = 255; // the 256th slot is the transparent index

const BUCKET_BITS = 5;
const BUCKET_COUNT = 1 << (BUCKET_BITS * 3);
const bucketOf = (r: number, g: number, b: number): number => (
    ((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3)
);
const bucketChannel = (key: number, channel: number): number => (
    (key >> (10 - channel * 5)) & 31
);

interface Histogram {
    keys: number[];
    counts: Uint32Array;
    sums: Float64Array; // r,g,b totals per bucket, for the box average
}

const buildHistogram = (frames: GifFrame[]): Histogram => {
    const counts = new Uint32Array(BUCKET_COUNT);
    const sums = new Float64Array(BUCKET_COUNT * 3);
    for (const frame of frames) {
        const { data } = frame;
        for (let i = 0; i < data.length; i += 4) {
            if (data[i + 3] < ALPHA_THRESHOLD) continue;
            const r = data[i];
            const g = data[i + 1];
            const b = data[i + 2];
            const key = bucketOf(r, g, b);
            counts[key] += 1;
            sums[key * 3] += r;
            sums[key * 3 + 1] += g;
            sums[key * 3 + 2] += b;
        }
    }
    const keys: number[] = [];
    for (let key = 0; key < BUCKET_COUNT; key += 1) {
        if (counts[key] > 0) keys.push(key);
    }
    return { keys, counts, sums };
};

interface BoxStats {
    count: number;
    /** The channel with the widest spread, and how wide it is. */
    channel: number;
    range: number;
}

const boxStats = (box: number[], counts: Uint32Array): BoxStats => {
    let count = 0;
    const min = [32, 32, 32];
    const max = [-1, -1, -1];
    for (const key of box) {
        count += counts[key];
        for (let axis = 0; axis < 3; axis += 1) {
            const value = bucketChannel(key, axis);
            if (value < min[axis]) min[axis] = value;
            if (value > max[axis]) max[axis] = value;
        }
    }
    let channel = 0;
    let range = -1;
    for (let axis = 0; axis < 3; axis += 1) {
        if (max[axis] - min[axis] > range) {
            range = max[axis] - min[axis];
            channel = axis;
        }
    }
    return { count, channel, range };
};

/**
 * Median cut over a 5-bit histogram: repeatedly split the box that most
 * deserves it along its widest channel until the palette is full. Quantising
 * the histogram first (rather than the pixels) is what keeps this linear in
 * image size — the cut itself only ever walks 32k buckets.
 *
 * Two details that a naive version gets wrong, both of which showed up as
 * "the GIF's colours are off" on a real sprite sheet:
 *
 * - The split must leave both halves non-empty. Splitting at the weighted
 *   median puts a dominant colour that sorts last in a half of its own only
 *   if the cut is clamped to [1, n-1]; otherwise the other half is empty, the
 *   same box is picked again next round, and the palette fills up with empty
 *   entries while every accent colour is averaged into the dominant one.
 * - Which box to split is weighted by population *and* spread, not population
 *   alone. By population, the boxes that never get split are the rare ones —
 *   red eyes, an orange tassel, a white highlight — which are exactly the
 *   colours an image is recognised by.
 */
const medianCut = (histogram: Histogram, maxColors: number): number[][] => {
    const { keys, counts } = histogram;
    if (keys.length === 0) return [];
    const boxes: number[][] = [keys];
    while (boxes.length < maxColors) {
        let target = -1;
        let targetStats: BoxStats | null = null;
        let best = 0;
        boxes.forEach((box, index) => {
            if (box.length < 2) return;
            const stats = boxStats(box, counts);
            const weight = stats.count * (stats.range + 1);
            if (weight > best) {
                best = weight;
                target = index;
                targetStats = stats;
            }
        });
        if (target < 0 || !targetStats) break;
        const { channel, count: total } = targetStats as BoxStats;
        const sorted = [...boxes[target]].sort((a, b) => bucketChannel(a, channel) - bucketChannel(b, channel));
        let running = 0;
        let split = 1;
        for (let i = 0; i < sorted.length - 1; i += 1) {
            running += counts[sorted[i]];
            split = i + 1;
            if (running * 2 >= total) break;
        }
        split = Math.min(Math.max(split, 1), sorted.length - 1);
        boxes.splice(target, 1, sorted.slice(0, split), sorted.slice(split));
    }
    return boxes;
};

interface Quantized {
    /** Packed RGB triples, one per palette entry. */
    palette: Uint8Array;
    /** Histogram bucket -> palette index. */
    lookup: Uint8Array;
}

const quantize = (frames: GifFrame[], maxColors: number): Quantized => {
    const histogram = buildHistogram(frames);
    const boxes = medianCut(histogram, maxColors);
    const palette = new Uint8Array(Math.max(1, boxes.length) * 3);
    const lookup = new Uint8Array(BUCKET_COUNT);
    boxes.forEach((box, index) => {
        let count = 0;
        let r = 0;
        let g = 0;
        let b = 0;
        for (const key of box) {
            count += histogram.counts[key];
            r += histogram.sums[key * 3];
            g += histogram.sums[key * 3 + 1];
            b += histogram.sums[key * 3 + 2];
            lookup[key] = index;
        }
        const divisor = Math.max(1, count);
        palette[index * 3] = Math.round(r / divisor);
        palette[index * 3 + 1] = Math.round(g / divisor);
        palette[index * 3 + 2] = Math.round(b / divisor);
    });
    return { palette, lookup };
};

class ByteWriter {
    private chunks: number[] = [];

    byte(value: number) {
        this.chunks.push(value & 0xFF);
    }

    u16(value: number) {
        this.chunks.push(value & 0xFF, (value >>> 8) & 0xFF);
    }

    bytes(values: ArrayLike<number>) {
        for (let i = 0; i < values.length; i += 1) this.chunks.push(values[i] & 0xFF);
    }

    ascii(text: string) {
        for (let i = 0; i < text.length; i += 1) this.chunks.push(text.charCodeAt(i));
    }

    concat(): Uint8Array {
        return new Uint8Array(this.chunks);
    }
}

/**
 * GIF's variable-width LZW, LSB-first. The dictionary is keyed by
 * `(prefix << 8) | next` — a number rather than a string — because this runs
 * once per pixel per frame.
 */
const lzwEncode = (indices: Uint8Array, minCodeSize: number): Uint8Array => {
    const clearCode = 1 << minCodeSize;
    const endCode = clearCode + 1;
    const out: number[] = [];
    let bitBuffer = 0;
    let bitCount = 0;
    let codeSize = minCodeSize + 1;
    let nextCode = endCode + 1;
    const dictionary = new Map<number, number>();

    const emit = (code: number) => {
        bitBuffer |= code << bitCount;
        bitCount += codeSize;
        while (bitCount >= 8) {
            out.push(bitBuffer & 0xFF);
            bitBuffer >>= 8;
            bitCount -= 8;
        }
    };

    emit(clearCode);
    let prefix = indices.length > 0 ? indices[0] : -1;
    for (let i = 1; i < indices.length; i += 1) {
        const next = indices[i];
        const key = (prefix << 8) | next;
        const existing = dictionary.get(key);
        if (existing !== undefined) {
            prefix = existing;
            continue;
        }
        emit(prefix);
        if (nextCode < 4096) {
            dictionary.set(key, nextCode);
            nextCode += 1;
            // Widen once the code just assigned no longer fits: a decoder is
            // always one entry behind (it cannot complete an entry until the
            // following code arrives), and this timing is what keeps the two
            // in step. Widening a code earlier decodes to garbage in real
            // viewers, which is why the round-trip test below exists.
            if (nextCode > (1 << codeSize) && codeSize < 12) codeSize += 1;
        } else {
            emit(clearCode);
            dictionary.clear();
            codeSize = minCodeSize + 1;
            nextCode = endCode + 1;
        }
        prefix = next;
    }
    if (prefix >= 0) emit(prefix);
    emit(endCode);
    if (bitCount > 0) out.push(bitBuffer & 0xFF);

    // Sub-blocks: a length byte every 255 bytes, terminated by an empty one.
    const blocked = new ByteWriter();
    for (let offset = 0; offset < out.length; offset += 255) {
        const chunk = out.slice(offset, offset + 255);
        blocked.byte(chunk.length);
        blocked.bytes(chunk);
    }
    blocked.byte(0);
    return blocked.concat();
};

const colorTableBits = (entries: number): number => {
    let bits = 1;
    while (1 << (bits + 1) < entries) bits += 1;
    return Math.min(bits, 7);
};

export const encodeGifBytes = (options: GifOptions): Uint8Array => {
    const { width, height, frames } = options;
    if (frames.length === 0) throw new Error('gif needs at least one frame');
    const { palette, lookup } = quantize(frames, MAX_COLORS);
    const colorCount = palette.length / 3;
    const transparentIndex = colorCount;
    const bits = colorTableBits(colorCount + 1);
    const tableEntries = 1 << (bits + 1);
    const delay = Math.max(2, Math.round(options.delayMs / 10));

    const writer = new ByteWriter();
    writer.ascii('GIF89a');
    writer.u16(width);
    writer.u16(height);
    writer.byte(0x80 | (7 << 4) | bits); // global table present, 8-bit colour resolution
    writer.byte(0);
    writer.byte(0);
    for (let index = 0; index < tableEntries; index += 1) {
        writer.byte(palette[index * 3] ?? 0);
        writer.byte(palette[index * 3 + 1] ?? 0);
        writer.byte(palette[index * 3 + 2] ?? 0);
    }

    // Netscape 2.0 loop block — without it every viewer plays the run once.
    writer.byte(0x21);
    writer.byte(0xFF);
    writer.byte(0x0B);
    writer.ascii('NETSCAPE2.0');
    writer.byte(0x03);
    writer.byte(0x01);
    writer.u16(options.loop ?? 0);
    writer.byte(0x00);

    const pixels = width * height;
    for (const frame of frames) {
        const indices = new Uint8Array(pixels);
        let hasTransparent = false;
        for (let i = 0; i < pixels; i += 1) {
            const offset = i * 4;
            if (frame.data[offset + 3] < ALPHA_THRESHOLD) {
                indices[i] = transparentIndex;
                hasTransparent = true;
            } else {
                indices[i] = lookup[bucketOf(frame.data[offset], frame.data[offset + 1], frame.data[offset + 2])];
            }
        }
        // Disposal 2 (restore to background) is what keeps a transparent frame
        // from showing the previous one through its holes.
        writer.byte(0x21);
        writer.byte(0xF9);
        writer.byte(0x04);
        writer.byte((hasTransparent ? 2 : 1) << 2 | (hasTransparent ? 1 : 0));
        writer.u16(delay);
        writer.byte(transparentIndex);
        writer.byte(0x00);

        writer.byte(0x2C);
        writer.u16(0);
        writer.u16(0);
        writer.u16(width);
        writer.u16(height);
        writer.byte(0x00); // no local colour table, not interlaced

        const minCodeSize = Math.max(2, bits + 1);
        writer.byte(minCodeSize);
        writer.bytes(lzwEncode(indices, minCodeSize));
    }

    writer.byte(0x3B);
    return writer.concat();
};

export const encodeGif = (options: GifOptions): Blob => new Blob(
    [encodeGifBytes(options) as unknown as BlobPart],
    { type: 'image/gif' },
);
