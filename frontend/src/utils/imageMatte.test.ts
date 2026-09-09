import { describe, expect, it } from 'vitest';
import { analyzeBackground, removeBackground, type RGBAImage } from './imageMatte';

const blank = (width: number, height: number): RGBAImage => ({
    width,
    height,
    data: new Uint8ClampedArray(width * height * 4),
});

const paint = (image: RGBAImage, x: number, y: number, rgba: [number, number, number, number]) => {
    const offset = (y * image.width + x) * 4;
    image.data.set(rgba, offset);
};

const checkerSheet = (size = 64, cell = 8): RGBAImage => {
    const image = blank(size, size);
    for (let y = 0; y < size; y += 1) {
        for (let x = 0; x < size; x += 1) {
            const light = ((Math.floor(x / cell) + Math.floor(y / cell)) % 2) === 0;
            paint(image, x, y, light ? [255, 255, 255, 255] : [204, 204, 204, 255]);
        }
    }
    return image;
};

const greenSheet = (size = 64): RGBAImage => {
    const image = blank(size, size);
    for (let y = 0; y < size; y += 1) {
        for (let x = 0; x < size; x += 1) paint(image, x, y, [0, 200, 40, 255]);
    }
    return image;
};

const alphaAt = (image: RGBAImage, x: number, y: number) => image.data[(y * image.width + x) * 4 + 3];

describe('analyzeBackground', () => {
    it('recognises the fake-transparency checkerboard', () => {
        expect(analyzeBackground(checkerSheet()).kind).toBe('checker');
    });

    it('recognises a green screen', () => {
        expect(analyzeBackground(greenSheet()).kind).toBe('green');
    });

    it('leaves a plain flat background alone — two tones that never alternate are not a checkerboard', () => {
        const image = blank(64, 64);
        for (let y = 0; y < 64; y += 1) {
            for (let x = 0; x < 64; x += 1) paint(image, x, y, [240, 240, 240, 255]);
        }
        expect(analyzeBackground(image).kind).toBe('none');
    });

    it('reports nothing for an already transparent sheet', () => {
        expect(analyzeBackground(blank(32, 32)).kind).toBe('none');
    });
});

describe('removeBackground', () => {
    it('clears the checkerboard and keeps the artwork', () => {
        const image = checkerSheet();
        const analysis = analyzeBackground(image);
        for (let y = 24; y < 40; y += 1) {
            for (let x = 24; x < 40; x += 1) paint(image, x, y, [200, 30, 30, 255]);
        }
        const cleaned = removeBackground(image, { kind: analysis.kind, colors: analysis.colors });
        expect(alphaAt(cleaned, 0, 0)).toBe(0);
        expect(alphaAt(cleaned, 63, 63)).toBe(0);
        expect(alphaAt(cleaned, 32, 32)).toBe(255);
    });

    it('keys out a green screen and despills the artwork edge', () => {
        const image = greenSheet();
        for (let y = 24; y < 40; y += 1) {
            for (let x = 24; x < 40; x += 1) paint(image, x, y, [220, 180, 60, 255]);
        }
        const cleaned = removeBackground(image, { kind: 'green' });
        expect(alphaAt(cleaned, 1, 1)).toBe(0);
        expect(alphaAt(cleaned, 32, 32)).toBe(255);
    });

    it('only clears background that reaches the edge, so grey inside the artwork survives', () => {
        const image = checkerSheet();
        const analysis = analyzeBackground(image);
        for (let y = 16; y < 48; y += 1) {
            for (let x = 16; x < 48; x += 1) paint(image, x, y, [40, 40, 200, 255]);
        }
        // A white highlight enclosed by the subject: the same colour as the
        // background, but not connected to it.
        for (let y = 30; y < 34; y += 1) {
            for (let x = 30; x < 34; x += 1) paint(image, x, y, [255, 255, 255, 255]);
        }
        const cleaned = removeBackground(image, { kind: 'checker', colors: analysis.colors });
        expect(alphaAt(cleaned, 0, 0)).toBe(0);
        expect(alphaAt(cleaned, 31, 31)).toBe(255);
    });

    it('is a no-op when nothing is to be cleared', () => {
        const image = checkerSheet();
        const cleaned = removeBackground(image, { kind: 'none' });
        expect(cleaned.data).toEqual(image.data);
    });
});
