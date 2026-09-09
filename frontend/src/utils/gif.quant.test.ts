import { describe, expect, it } from 'vitest';
import { encodeGifBytes } from './gif';

// Reads the global palette back out of an encoded GIF.
const paletteOf = (bytes: Uint8Array): [number, number, number][] => {
    const bits = bytes[10] & 0x07;
    const entries = 1 << (bits + 1);
    const out: [number, number, number][] = [];
    for (let i = 0; i < entries; i += 1) out.push([bytes[13 + i * 3], bytes[14 + i * 3], bytes[15 + i * 3]]);
    return out;
};

const nearest = (palette: [number, number, number][], rgb: [number, number, number]): number => (
    Math.min(...palette.map(([r, g, b]) => Math.hypot(r - rgb[0], g - rgb[1], b - rgb[2])))
);

describe('gif palette', () => {
    it('keeps every accent colour when one colour dominates the frame', () => {
        // A sprite: mostly bright white (dominant), with a handful of rare
        // accents — red eyes, orange tassel, cyan glow, near-black hair.
        const width = 64;
        const height = 64;
        const data = new Uint8ClampedArray(width * height * 4);
        const accents: [number, number, number][] = [[210, 30, 40], [240, 150, 30], [40, 190, 240], [24, 24, 32]];
        for (let i = 0; i < width * height; i += 1) {
            const accent = i < accents.length * 20 ? accents[Math.floor(i / 20)] : null;
            const [r, g, b] = accent ?? [250, 250, 250];
            data.set([r, g, b, 255], i * 4);
        }
        const palette = paletteOf(encodeGifBytes({ width, height, frames: [{ data }], delayMs: 100 }));
        for (const accent of accents) {
            expect(nearest(palette, accent), `accent ${accent.join(',')}`).toBeLessThan(6);
        }
        expect(nearest(palette, [250, 250, 250])).toBeLessThan(6);
    });

    it('represents every input colour when there are fewer than 255 of them', () => {
        // Many distinct dark colours plus one dominant bright one. The bright
        // colour sorts last on every channel — the case where a median split
        // can produce an empty half and stall the cut.
        const width = 80;
        const height = 80;
        const data = new Uint8ClampedArray(width * height * 4);
        const inputs = new Set<string>();
        // 6 levels per channel: 216 dark colours, each used a few times.
        for (let i = 0; i < width * height; i += 1) {
            const dark = i < 1200;
            const n = i % 216;
            const r = dark ? (n % 6) * 10 : 250;
            const g = dark ? (Math.floor(n / 6) % 6) * 10 : 250;
            const b = dark ? Math.floor(n / 36) * 10 : 250;
            inputs.add(`${r},${g},${b}`);
            data.set([r, g, b, 255], i * 4);
        }
        expect(inputs.size).toBeLessThan(255);
        const palette = paletteOf(encodeGifBytes({ width, height, frames: [{ data }], delayMs: 100 }));
        for (const input of inputs) {
            const rgb = input.split(',').map(Number) as [number, number, number];
            // 5-bit buckets: anything sharing a bucket may be averaged, so the
            // bound is one bucket width per channel, not zero.
            expect(nearest(palette, rgb), input).toBeLessThan(14);
        }
    });
});
