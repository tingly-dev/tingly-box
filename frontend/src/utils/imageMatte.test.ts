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

describe('a green screen with effects painted over it', () => {
    // The shape of a sprite sheet that broke the first implementation: the
    // key is a flat saturated green, and the artwork's glows are blends of
    // white or cyan *with* that green — greener than they are red or blue,
    // and so keyed out entirely by a green-dominance test.
    const KEY: [number, number, number] = [34, 221, 34];
    const sheet = (): RGBAImage => {
        const image = blank(80, 80);
        for (let y = 0; y < 80; y += 1) {
            for (let x = 0; x < 80; x += 1) paint(image, x, y, [...KEY, 255]);
        }
        const fill = (x0: number, y0: number, x1: number, y1: number, rgb: [number, number, number]) => {
            for (let y = y0; y < y1; y += 1) {
                for (let x = x0; x < x1; x += 1) paint(image, x, y, [...rgb, 255]);
            }
        };
        fill(30, 30, 50, 50, [24, 24, 32]);        // the character, near black
        fill(30, 20, 50, 30, [0, 180, 255]);       // a cyan glow
        fill(50, 30, 62, 50, [150, 230, 180]);     // a white arc blended with the key
        fill(20, 50, 30, 60, [255, 255, 255]);     // a bright sparkle
        return image;
    };

    it('keeps every effect and clears only the key colour', () => {
        const image = sheet();
        const analysis = analyzeBackground(image);
        expect(analysis.kind).toBe('green');
        expect(analysis.colors[0]).toEqual(KEY);

        const cleaned = removeBackground(image, { kind: 'green', colors: analysis.colors });
        expect(alphaAt(cleaned, 2, 2)).toBe(0);        // background
        expect(alphaAt(cleaned, 78, 78)).toBe(0);
        expect(alphaAt(cleaned, 40, 40)).toBe(255);    // character
        expect(alphaAt(cleaned, 40, 25)).toBe(255);    // cyan glow
        expect(alphaAt(cleaned, 55, 40)).toBe(255);    // white arc over green
        expect(alphaAt(cleaned, 25, 55)).toBe(255);    // sparkle
    });

    it('keys a shaded patch of the same screen, which is why chroma is the metric', () => {
        const image = sheet();
        // Same backdrop, lit at half brightness — a different RGB colour, the
        // same chroma. A plain RGB distance would leave this behind.
        for (let y = 60; y < 78; y += 1) {
            for (let x = 2; x < 20; x += 1) paint(image, x, y, [17, 110, 17, 255]);
        }
        const cleaned = removeBackground(image, { kind: 'green', colors: [KEY] });
        expect(alphaAt(cleaned, 10, 70)).toBe(0);
    });

    it('clears the key everywhere, including pockets the artwork encloses', () => {
        const image = sheet();
        // A ring of artwork with the backdrop trapped inside it — between an
        // arm and a body, inside the loop of a sword arc. Those pixels never
        // touch the frame's edge, and a key that only follows connected
        // background leaves every one of them green.
        for (let y = 8; y < 24; y += 1) {
            for (let x = 8; x < 24; x += 1) {
                const onRing = y === 8 || y === 23 || x === 8 || x === 23;
                if (onRing) paint(image, x, y, [26, 26, 34, 255]);
            }
        }
        const cleaned = removeBackground(image, { kind: 'green', colors: [KEY] });
        expect(alphaAt(cleaned, 16, 16)).toBe(0);   // trapped backdrop
        expect(alphaAt(cleaned, 8, 16)).toBe(255);  // the ring itself
    });

    it('changes alpha only — a key must never repaint the artwork', () => {
        const image = sheet();
        const before = new Uint8ClampedArray(image.data);
        const cleaned = removeBackground(image, { kind: 'green', colors: [KEY] });
        for (let i = 0; i < cleaned.data.length; i += 4) {
            if (cleaned.data[i + 3] === 0) continue;
            expect([cleaned.data[i], cleaned.data[i + 1], cleaned.data[i + 2]])
                .toEqual([before[i], before[i + 1], before[i + 2]]);
        }
    });
});
