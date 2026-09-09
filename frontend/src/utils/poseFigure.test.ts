import { describe, expect, it } from 'vitest';
import {
    applyPreset,
    createFigure,
    distanceToSegment,
    figureBounds,
    figureVisualBounds,
    flipFigure,
    hitTestBody,
    hitTestJoint,
    isScaleHandleHit,
    JOINT_KEYS,
    MIN_FIGURE_HEIGHT,
    moveJoint,
    scaleFigure,
    scaleHandlePoint,
    translateFigure,
} from './poseFigure';

const DIMS = { width: 1024, height: 1024 };

describe('createFigure', () => {
    it('centres the figure and sizes it to 70% of the canvas height', () => {
        const figure = createFigure('standing', DIMS);
        const bounds = figureBounds(figure);
        expect(bounds.height).toBeCloseTo(1024 * 0.7 * (0.96 - 0.055), 1);
        expect(bounds.x + bounds.width / 2).toBeCloseTo(512, 0);
        expect(bounds.y + bounds.height / 2).toBeCloseTo(512, 0);
    });

    it('keeps every figure inside a narrow canvas', () => {
        const figure = createFigure('armsUp', { width: 512, height: 1792 });
        const bounds = figureVisualBounds(figure);
        expect(bounds.x).toBeGreaterThanOrEqual(0);
        expect(bounds.x + bounds.width).toBeLessThanOrEqual(512);
    });

    it('gives each figure a distinct id', () => {
        expect(createFigure('standing', DIMS).id).not.toBe(createFigure('standing', DIMS).id);
    });

    it('defines every joint for every preset', () => {
        for (const preset of ['standing', 'walking', 'sitting', 'armsUp'] as const) {
            const figure = createFigure(preset, DIMS);
            for (const key of JOINT_KEYS) {
                expect(Number.isFinite(figure.joints[key].x)).toBe(true);
                expect(Number.isFinite(figure.joints[key].y)).toBe(true);
            }
        }
    });
});

describe('applyPreset', () => {
    it('swaps the pose in place, keeping position, size and id', () => {
        const figure = scaleFigure(translateFigure(createFigure('standing', DIMS), 120, -40), 0.5);
        const before = figureBounds(figure);
        const after = applyPreset(figure, 'sitting', DIMS);
        const afterBounds = figureBounds(after);
        expect(after.id).toBe(figure.id);
        expect(afterBounds.height).toBeCloseTo(before.height, 4);
        expect(afterBounds.x + afterBounds.width / 2).toBeCloseTo(before.x + before.width / 2, 4);
        expect(afterBounds.y + afterBounds.height / 2).toBeCloseTo(before.y + before.height / 2, 4);
        expect(after.joints.kneeL.x).not.toBeCloseTo(figure.joints.kneeL.x, 1);
    });
});

describe('transforms', () => {
    it('translates every joint by the same delta', () => {
        const figure = createFigure('standing', DIMS);
        const moved = translateFigure(figure, 10, -5);
        for (const key of JOINT_KEYS) {
            expect(moved.joints[key].x).toBeCloseTo(figure.joints[key].x + 10);
            expect(moved.joints[key].y).toBeCloseTo(figure.joints[key].y - 5);
        }
    });

    it('moves a single joint without touching the others', () => {
        const figure = createFigure('standing', DIMS);
        const moved = moveJoint(figure, 'wristL', { x: 1, y: 2 });
        expect(moved.joints.wristL).toEqual({ x: 1, y: 2 });
        expect(moved.joints.wristR).toEqual(figure.joints.wristR);
    });

    it('scales about the centre, leaving it fixed', () => {
        const figure = createFigure('standing', DIMS);
        const before = figureBounds(figure);
        const scaled = scaleFigure(figure, 2);
        const after = figureBounds(scaled);
        expect(after.height).toBeCloseTo(before.height * 2, 4);
        expect(after.x + after.width / 2).toBeCloseTo(before.x + before.width / 2, 4);
    });

    it('refuses to shrink a figure below the grabbable minimum', () => {
        const figure = createFigure('standing', DIMS);
        const scaled = scaleFigure(figure, 0.0001);
        expect(figureBounds(scaled).height).toBeGreaterThanOrEqual(MIN_FIGURE_HEIGHT - 1e-6);
    });

    it('ignores a non-finite or negative scale factor', () => {
        const figure = createFigure('standing', DIMS);
        expect(figureBounds(scaleFigure(figure, Number.NaN)).height).toBeCloseTo(figureBounds(figure).height, 4);
        expect(figureBounds(scaleFigure(figure, -2)).height).toBeCloseTo(figureBounds(figure).height, 4);
    });

    it('mirrors horizontally about the figure centre', () => {
        const figure = createFigure('walking', DIMS);
        const before = figureBounds(figure);
        const flipped = flipFigure(figure);
        const after = figureBounds(flipped);
        expect(after.x).toBeCloseTo(before.x, 4);
        expect(after.width).toBeCloseTo(before.width, 4);
        expect(flipped.joints.ankleL.x).toBeCloseTo(before.x * 2 + before.width - figure.joints.ankleL.x, 4);
        expect(flipFigure(flipped).joints.ankleL.x).toBeCloseTo(figure.joints.ankleL.x, 4);
    });
});

describe('hit testing', () => {
    it('finds the nearest joint within the radius and nothing outside it', () => {
        const figure = createFigure('standing', DIMS);
        const wrist = figure.joints.wristL;
        expect(hitTestJoint(figure, { x: wrist.x + 3, y: wrist.y - 2 }, 20)).toBe('wristL');
        expect(hitTestJoint(figure, { x: 5, y: 5 }, 20)).toBeNull();
    });

    it('hits the body on a bone and the head, but not the gap between the legs', () => {
        const figure = createFigure('standing', DIMS);
        const knee = figure.joints.kneeL;
        expect(hitTestBody(figure, knee)).toBe(true);
        expect(hitTestBody(figure, figure.joints.head)).toBe(true);
        const between = { x: (figure.joints.ankleL.x + figure.joints.ankleR.x) / 2, y: figure.joints.ankleL.y };
        expect(hitTestBody(figure, between)).toBe(false);
        expect(hitTestBody(figure, { x: 0, y: 0 })).toBe(false);
    });

    it('puts the scale handle at the bottom-right of the padded box', () => {
        const figure = createFigure('standing', DIMS);
        const visual = figureVisualBounds(figure);
        const handle = scaleHandlePoint(figure);
        expect(handle.x).toBeCloseTo(visual.x + visual.width, 4);
        expect(handle.y).toBeCloseTo(visual.y + visual.height, 4);
        expect(isScaleHandleHit(figure, { x: handle.x + 4, y: handle.y + 4 }, 12)).toBe(true);
        expect(isScaleHandleHit(figure, { x: handle.x + 40, y: handle.y }, 12)).toBe(false);
    });
});

describe('distanceToSegment', () => {
    it('measures perpendicular distance inside the segment', () => {
        expect(distanceToSegment({ x: 5, y: 3 }, { x: 0, y: 0 }, { x: 10, y: 0 })).toBeCloseTo(3);
    });

    it('clamps to the endpoints outside the segment', () => {
        expect(distanceToSegment({ x: -4, y: 0 }, { x: 0, y: 0 }, { x: 10, y: 0 })).toBeCloseTo(4);
        expect(distanceToSegment({ x: 14, y: 0 }, { x: 0, y: 0 }, { x: 10, y: 0 })).toBeCloseTo(4);
    });

    it('handles a degenerate segment', () => {
        expect(distanceToSegment({ x: 3, y: 4 }, { x: 0, y: 0 }, { x: 0, y: 0 })).toBeCloseTo(5);
    });
});
