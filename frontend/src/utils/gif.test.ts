import { describe, expect, it } from 'vitest';
import { encodeGifBytes } from './gif';

const solidFrame = (width: number, height: number, rgba: [number, number, number, number]) => {
    const data = new Uint8ClampedArray(width * height * 4);
    for (let i = 0; i < data.length; i += 4) {
        data[i] = rgba[0];
        data[i + 1] = rgba[1];
        data[i + 2] = rgba[2];
        data[i + 3] = rgba[3];
    }
    return { data };
};

const ascii = (bytes: Uint8Array, offset: number, length: number) => String.fromCharCode(...bytes.slice(offset, offset + length));

describe('encodeGifBytes', () => {
    it('writes a GIF89a header carrying the logical screen size', () => {
        const bytes = encodeGifBytes({
            width: 4,
            height: 3,
            frames: [solidFrame(4, 3, [255, 0, 0, 255])],
            delayMs: 200,
        });
        expect(ascii(bytes, 0, 6)).toBe('GIF89a');
        expect(bytes[6] | (bytes[7] << 8)).toBe(4);
        expect(bytes[8] | (bytes[9] << 8)).toBe(3);
        expect(bytes[10] & 0x80).toBe(0x80); // global colour table present
        expect(bytes[bytes.length - 1]).toBe(0x3B); // trailer
    });

    it('declares an infinite Netscape loop and one graphic control per frame', () => {
        const bytes = encodeGifBytes({
            width: 2,
            height: 2,
            frames: [
                solidFrame(2, 2, [255, 0, 0, 255]),
                solidFrame(2, 2, [0, 0, 255, 255]),
                solidFrame(2, 2, [0, 255, 0, 255]),
            ],
            delayMs: 200,
        });
        const text = ascii(bytes, 0, bytes.length);
        expect(text).toContain('NETSCAPE2.0');
        let controls = 0;
        let images = 0;
        for (let i = 0; i < bytes.length - 1; i += 1) {
            if (bytes[i] === 0x21 && bytes[i + 1] === 0xF9) controls += 1;
            if (bytes[i] === 0x2C) images += 1;
        }
        expect(controls).toBe(3);
        expect(images).toBeGreaterThanOrEqual(3);
    });

    it('stores the delay in hundredths of a second, never below the 2cs floor', () => {
        const find = (delayMs: number) => {
            const bytes = encodeGifBytes({ width: 1, height: 1, frames: [solidFrame(1, 1, [1, 2, 3, 255])], delayMs });
            const at = bytes.findIndex((value, index) => value === 0x21 && bytes[index + 1] === 0xF9);
            return bytes[at + 4] | (bytes[at + 5] << 8);
        };
        expect(find(200)).toBe(20);
        expect(find(5)).toBe(2);
    });

    it('marks frames that carry transparency', () => {
        const bytes = encodeGifBytes({
            width: 2,
            height: 2,
            frames: [solidFrame(2, 2, [10, 20, 30, 0])],
            delayMs: 100,
        });
        const at = bytes.findIndex((value, index) => value === 0x21 && bytes[index + 1] === 0xF9);
        expect(bytes[at + 3] & 0x01).toBe(1); // transparency flag
        expect((bytes[at + 3] >> 2) & 0x07).toBe(2); // restore to background
    });

    it('refuses to encode an empty animation', () => {
        expect(() => encodeGifBytes({ width: 1, height: 1, frames: [], delayMs: 100 })).toThrow();
    });
});

// A GIF that merely has the right headers can still be undecodable, so the
// LZW stream is checked by decoding it back — the only way to know the codes
// this writer emits are the codes a viewer will read.
const decodeFirstFrame = (bytes: Uint8Array) => {
    const bits = bytes[10] & 0x07;
    const tableEntries = 1 << (bits + 1);
    const palette = bytes.slice(13, 13 + tableEntries * 3);
    let cursor = 13 + tableEntries * 3;
    while (bytes[cursor] !== 0x2C) {
        if (bytes[cursor] === 0x21) {
            cursor += 2;
            while (bytes[cursor] !== 0) cursor += bytes[cursor] + 1;
            cursor += 1;
        } else {
            cursor += 1;
        }
    }
    const width = bytes[cursor + 5] | (bytes[cursor + 6] << 8);
    const height = bytes[cursor + 7] | (bytes[cursor + 8] << 8);
    cursor += 10;
    const minCodeSize = bytes[cursor];
    cursor += 1;
    const stream: number[] = [];
    while (bytes[cursor] !== 0) {
        const length = bytes[cursor];
        for (let i = 0; i < length; i += 1) stream.push(bytes[cursor + 1 + i]);
        cursor += length + 1;
    }

    const clearCode = 1 << minCodeSize;
    const endCode = clearCode + 1;
    let dictionary: number[][] = [];
    const reset = () => {
        dictionary = [];
        for (let i = 0; i < clearCode; i += 1) dictionary.push([i]);
        dictionary.push([], []);
    };
    reset();
    let codeSize = minCodeSize + 1;
    let buffer = 0;
    let bitCount = 0;
    let previous: number[] | null = null;
    const indices: number[] = [];
    for (const byte of stream) {
        buffer |= byte << bitCount;
        bitCount += 8;
        while (bitCount >= codeSize) {
            const code = buffer & ((1 << codeSize) - 1);
            buffer >>= codeSize;
            bitCount -= codeSize;
            if (code === clearCode) {
                reset();
                codeSize = minCodeSize + 1;
                previous = null;
                continue;
            }
            if (code === endCode) return { width, height, indices, palette };
            let entry: number[];
            if (code < dictionary.length && dictionary[code].length > 0) entry = dictionary[code];
            else if (previous) entry = [...previous, previous[0]];
            else throw new Error('bad code');
            indices.push(...entry);
            if (previous) {
                dictionary.push([...previous, entry[0]]);
                if (dictionary.length === (1 << codeSize) && codeSize < 12) codeSize += 1;
            }
            previous = entry;
        }
    }
    return { width, height, indices, palette };
};

describe('the LZW stream a viewer reads back', () => {
    it('reproduces the frame pixel for pixel', () => {
        const width = 12;
        const height = 8;
        const data = new Uint8ClampedArray(width * height * 4);
        for (let i = 0; i < width * height; i += 1) {
            const left = (i % width) < width / 2;
            data[i * 4] = left ? 220 : 20;
            data[i * 4 + 1] = left ? 40 : 180;
            data[i * 4 + 2] = 60;
            data[i * 4 + 3] = 255;
        }
        const bytes = encodeGifBytes({ width, height, frames: [{ data }], delayMs: 100 });
        const decoded = decodeFirstFrame(bytes);
        expect(decoded.width).toBe(width);
        expect(decoded.height).toBe(height);
        expect(decoded.indices).toHaveLength(width * height);
        for (let i = 0; i < width * height; i += 1) {
            const index = decoded.indices[i];
            const left = (i % width) < width / 2;
            expect(decoded.palette[index * 3]).toBeCloseTo(left ? 220 : 20, -1);
            expect(decoded.palette[index * 3 + 1]).toBeCloseTo(left ? 40 : 180, -1);
        }
    });
});

describe('a stream long enough to exhaust the dictionary', () => {
    it('still decodes to exactly one index per pixel', () => {
        // Deterministic noise: enough distinct sequences to run the code width
        // up to 12 bits and force the mid-stream clear.
        const width = 220;
        const height = 220;
        const data = new Uint8ClampedArray(width * height * 4);
        let state = 12345;
        for (let i = 0; i < width * height; i += 1) {
            state = (state * 1103515245 + 12345) & 0x7FFFFFFF;
            data[i * 4] = state & 0xFF;
            data[i * 4 + 1] = (state >> 8) & 0xFF;
            data[i * 4 + 2] = (state >> 16) & 0xFF;
            data[i * 4 + 3] = 255;
        }
        const decoded = decodeFirstFrame(encodeGifBytes({ width, height, frames: [{ data }], delayMs: 100 }));
        expect(decoded.indices).toHaveLength(width * height);
    });
});
