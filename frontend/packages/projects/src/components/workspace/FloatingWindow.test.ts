import { describe, expect, it } from 'vitest';
import { WINDOW_MIN, clampRect } from './FloatingWindow';

describe('clampRect', () => {
    it('keeps a window inside the viewport', () => {
        expect(clampRect({ x: 100, y: 100, w: 500, h: 400 }, 1440, 900)).toEqual({ x: 100, y: 100, w: 500, h: 400 });
    });
    it('never lets the title bar leave the screen', () => {
        const r = clampRect({ x: 5000, y: 5000, w: 500, h: 400 }, 1440, 900);
        expect(r.x).toBeLessThanOrEqual(1440 - 120);
        expect(r.y).toBeLessThanOrEqual(900 - 40);
        expect(clampRect({ x: -5000, y: -5000, w: 500, h: 400 }, 1440, 900).y).toBeGreaterThanOrEqual(8);
    });
    it('keeps the minimum size, and fits a small viewport', () => {
        const r = clampRect({ x: 0, y: 0, w: 10, h: 10 }, 1440, 900);
        expect(r.w).toBe(WINDOW_MIN.w);
        expect(r.h).toBe(WINDOW_MIN.h);
        expect(clampRect({ x: 0, y: 0, w: 2000, h: 2000 }, 1000, 700)).toMatchObject({ w: 984, h: 684 });
    });
});
