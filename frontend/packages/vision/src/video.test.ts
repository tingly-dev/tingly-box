import { describe, expect, it } from 'vitest';
import { evenSize, MAX_VIDEO_LOOPS, MIN_VIDEO_MS, planVideoLoops, videoFileName } from './video';

describe('planVideoLoops', () => {
    it('repeats a short sequence until it clears the minimum length', () => {
        // 9 frames × 200 ms = 1.8 s → two loops reach 3.6 s
        expect(planVideoLoops(9, 200)).toBe(2);
        expect(planVideoLoops(9, 200) * 9 * 200).toBeGreaterThanOrEqual(MIN_VIDEO_MS);
    });

    it('plays a sequence that is already long enough exactly once', () => {
        expect(planVideoLoops(16, 500)).toBe(1);
    });

    it('never asks for zero loops, and caps the count', () => {
        expect(planVideoLoops(0, 200)).toBe(1);
        expect(planVideoLoops(2, 1)).toBe(MAX_VIDEO_LOOPS);
    });
});

describe('evenSize', () => {
    it('pads odd dimensions up by one pixel, leaving even ones alone', () => {
        expect(evenSize({ width: 341, height: 341 })).toEqual({ width: 342, height: 342 });
        expect(evenSize({ width: 512, height: 341 })).toEqual({ width: 512, height: 342 });
        expect(evenSize({ width: 512, height: 512 })).toEqual({ width: 512, height: 512 });
    });
});

describe('videoFileName', () => {
    it('takes the extension from the container the browser could produce', () => {
        expect(videoFileName('cat', 9, { container: 'mp4', codec: 'avc', extension: 'mp4' })).toBe('cat-9.mp4');
        expect(videoFileName('cat', 9, { container: 'webm', codec: 'vp9', extension: 'webm' })).toBe('cat-9.webm');
    });
});
