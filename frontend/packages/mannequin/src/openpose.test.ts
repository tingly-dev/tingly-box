import { describe, expect, it } from 'vitest';
import {
    CAMERA_ELEVATIONS,
    cameraTurn,
    createFigure,
    describeShot,
    LENSES,
    OPENPOSE_KEYPOINTS,
    openposeKeypoints,
    setFigureLens,
} from './index';

const DIMS = { width: 1024, height: 1024 };
const at = (name: (typeof OPENPOSE_KEYPOINTS)[number]) => OPENPOSE_KEYPOINTS.indexOf(name);
const eye = CAMERA_ELEVATIONS.find((row) => row.key === 'eye')!;

describe('openposeKeypoints', () => {
    it('finds all eighteen keypoints on a figure facing the camera', () => {
        const points = openposeKeypoints(createFigure('standing', DIMS, undefined, 0, cameraTurn(eye, 0)));
        expect(points).toHaveLength(18);
        expect(points.every((p) => p !== null)).toBe(true);
    });

    it('names sides for the subject, not the viewer', () => {
        // Facing us, the subject's right shoulder is on our left. OpenPose's
        // colours are how a model tells left from right; swapped, every pose
        // would come back mirrored.
        const points = openposeKeypoints(createFigure('standing', DIMS, undefined, 0, cameraTurn(eye, 0)));
        expect(points[at('shoulderR')]!.x).toBeLessThan(points[at('shoulderL')]!.x);
        expect(points[at('hipR')]!.x).toBeLessThan(points[at('hipL')]!.x);
        expect(points[at('eyeR')]!.x).toBeLessThan(points[at('eyeL')]!.x);
    });

    it('drops the face when the figure is seen from behind', () => {
        // No nose and no eyes is how an OpenPose map says "back view".
        const points = openposeKeypoints(createFigure('standing', DIMS, undefined, 0, cameraTurn(eye, 180)));
        expect(points[at('nose')]).toBeNull();
        expect(points[at('eyeR')]).toBeNull();
        expect(points[at('eyeL')]).toBeNull();
        expect(points[at('earR')]).not.toBeNull();
        expect(points[at('earL')]).not.toBeNull();
    });

    it('shows one eye and one ear in profile', () => {
        const points = openposeKeypoints(createFigure('standing', DIMS, undefined, 0, cameraTurn(eye, 90)));
        expect(points[at('nose')]).not.toBeNull();
        expect([points[at('eyeR')], points[at('eyeL')]].filter(Boolean)).toHaveLength(1);
        expect([points[at('earR')], points[at('earL')]].filter(Boolean)).toHaveLength(1);
    });
});

describe('describeShot', () => {
    it('gives every row and every column of the camera grid its own word', () => {
        const heights = CAMERA_ELEVATIONS.map((row) => describeShot(createFigure('standing', DIMS, undefined, 0, cameraTurn(row, 0))).height);
        expect(heights).toEqual(['overhead', 'high', 'eye', 'low', 'worm']);
        const sides = [0, 45, 90, 135, 180, -45, -90].map((yaw) => describeShot(createFigure('standing', DIMS, undefined, 0, cameraTurn(eye, yaw))).side);
        expect(sides).toEqual(['front', 'threeQuarter', 'side', 'threeQuarterBack', 'back', 'threeQuarter', 'side']);
    });

    it('names the lens', () => {
        const figure = createFigure('standing', DIMS);
        expect(describeShot(figure).lens).toBe('standard');
        expect(describeShot(setFigureLens(figure, LENSES[0].distance)).lens).toBe('ultraWide');
    });
});
