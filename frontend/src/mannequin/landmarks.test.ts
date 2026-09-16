import { describe, expect, it } from 'vitest';
import {
    createFigure,
    figureUnit,
    JOINT_KEYS,
    JOINT_PARENT,
    POSE_LIBRARY,
    VIEW_PRESETS,
    type JointKey,
    type PoseFigure,
} from './index';
import {
    bodyForward,
    figureFromLandmarks,
    LANDMARK,
    LANDMARK_COUNT,
    landmarksFromFigure,
    turnOfBody,
    type Landmark,
} from './landmarks';

const DIMS = { width: 1024, height: 1024 };
const FRAME = { width: 1024, height: 1024 };
const WIDE = { width: 1600, height: 900 };

const everyPose = POSE_LIBRARY.flatMap((group) => group.poses);

// The mannequin is the one thing we have whose correct answer is known: put a
// pose through the landmark shape an estimator emits and it has to come back.
// No model, no photograph, no network — which is the point of the boundary.
const roundTrip = (figure: PoseFigure, frame = FRAME) => {
    const landmarks = landmarksFromFigure(figure, frame);
    const result = figureFromLandmarks(landmarks, { frame, reference: figure });
    expect(result).not.toBeNull();
    return result!;
};

const apart = (a: PoseFigure, b: PoseFigure): number => Math.max(...JOINT_KEYS.map((key) => Math.hypot(
    a.joints[key].x - b.joints[key].x,
    a.joints[key].y - b.joints[key].y,
    a.joints[key].z - b.joints[key].z,
)));

describe('landmarksFromFigure', () => {
    it('emits the full array even though a mannequin only implies part of it', () => {
        const landmarks = landmarksFromFigure(createFigure('standing', DIMS), FRAME);
        expect(landmarks).toHaveLength(LANDMARK_COUNT);
        // The joints we do have are real; the fingers and toes we do not are
        // marked invisible rather than silently placed at the origin.
        expect(landmarks[LANDMARK.shoulderL].visibility).toBe(1);
        expect(landmarks[LANDMARK.thumbL].visibility).toBe(0);
    });

    it('puts the nose in front of the ears, which is what fixes the depth axis', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.front);
        const landmarks = landmarksFromFigure(figure, FRAME);
        // Smaller z is nearer the camera in the landmark convention.
        expect(landmarks[LANDMARK.nose].z).toBeLessThan(landmarks[LANDMARK.earL].z);
    });
});

describe('figureFromLandmarks', () => {
    it('brings every pose in the library back, to within the torso reconstruction', () => {
        // Exact everywhere except the neck and the hip root, which the landmark
        // set does not contain and which are walked back up the torso axis (see
        // `readJoints`). On our own library that reconstruction is worth at
        // most ~2% of the figure — an order of magnitude below what any
        // estimator's own jitter will be.
        for (const pose of everyPose) {
            const figure = createFigure(pose, DIMS);
            const { figure: back } = roundTrip(figure);
            expect(apart(figure, back)).toBeLessThan(figureUnit(figure) * 0.03);
        }
    });

    it('is exact for the limbs, which are a rename rather than an estimate', () => {
        const figure = createFigure('running', DIMS);
        const { figure: back } = roundTrip(figure);
        const offset = (f: PoseFigure, key: JointKey) => ({
            x: f.joints[key].x - f.joints.hip.x,
            y: f.joints[key].y - f.joints.hip.y,
            z: f.joints[key].z - f.joints.hip.z,
        });
        for (const key of ['elbowL', 'wristR', 'kneeL', 'ankleR'] as JointKey[]) {
            const a = offset(figure, key);
            const b = offset(back, key);
            expect(Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z)).toBeLessThan(figureUnit(figure) * 0.03);
        }
    });

    it('survives a non-square frame', () => {
        // x is normalized by the image width and y by its height, so on a 16:9
        // photo the two are not the same distance. Read straight, a body comes
        // back squashed.
        const figure = createFigure('walking', DIMS);
        const { figure: back } = roundTrip(figure, WIDE);
        expect(apart(figure, back)).toBeLessThan(figureUnit(figure) * 0.03);
        // ...and reading it as if the photo were square would not have worked.
        const squashed = figureFromLandmarks(landmarksFromFigure(figure, WIDE), { frame: FRAME, reference: figure })!;
        expect(apart(figure, squashed.figure)).toBeGreaterThan(figureUnit(figure) * 0.03);
    });

    it('keeps a one-sided pose on the side it was on', () => {
        // The estimator names the subject's own left; our joints are named for
        // where they sit on screen. Swap them and everything still looks like a
        // person — just the mirror of the one in the photograph.
        const figure = createFigure('wave', DIMS, undefined, 0, VIEW_PRESETS.front);
        const { figure: back } = roundTrip(figure);
        const raisedBefore = figure.joints.wristR.y < figure.joints.wristL.y;
        const raisedAfter = back.joints.wristR.y < back.joints.wristL.y;
        expect(raisedAfter).toBe(raisedBefore);
    });

    it('rights a depth axis that comes back the other way round', () => {
        // A runtime that signs z differently, or a mirrored selfie, would
        // otherwise produce a figure facing away from the camera with its
        // limbs inside out. The nose is in front of the ears; that settles it.
        const figure = createFigure('reaching', DIMS, undefined, 0, VIEW_PRESETS.front);
        const flipped = landmarksFromFigure(figure, FRAME).map((l) => ({ ...l, z: -l.z }));
        const result = figureFromLandmarks(flipped, { frame: FRAME, reference: figure });
        expect(result).not.toBeNull();
        expect(apart(figure, result!.figure)).toBeLessThan(figureUnit(figure) * 0.05);
    });

    it('takes the angles from the photograph and the build from the mannequin', () => {
        // The whole retarget in one assertion: a stretched subject must come
        // back as *our* person in their pose, not as a differently proportioned
        // one. Otherwise every photo yields a new body and applying a preset
        // afterwards resizes them.
        const figure = createFigure('standing', DIMS);
        const stretched = landmarksFromFigure(figure, FRAME).map((l) => ({ ...l, y: l.y * 1.4 }));
        const result = figureFromLandmarks(stretched, { frame: FRAME, reference: figure })!;
        for (const key of JOINT_KEYS) {
            const parent = JOINT_PARENT[key];
            if (!parent) continue;
            const bone = (f: PoseFigure) => Math.hypot(
                f.joints[key].x - f.joints[parent].x,
                f.joints[key].y - f.joints[parent].y,
                f.joints[key].z - f.joints[parent].z,
            );
            expect(bone(result.figure)).toBeCloseTo(bone(figure), 3);
        }
        // ...and it is a different pose, so the test is not passing by accident.
        expect(apart(figure, result.figure)).toBeGreaterThan(figureUnit(figure) * 0.01);
    });

    it('keeps the figure where it was and the size it was', () => {
        const figure = createFigure('standing', DIMS, { x: 300, y: 640 }, 2);
        const { figure: back } = roundTrip(createFigure('running', DIMS, { x: 300, y: 640 }, 2));
        expect(figureUnit(back)).toBeCloseTo(figureUnit(figure), 3);
        expect(back.shade).toBe(2);
    });

    it('leaves an unseen limb where it was instead of amputating it', () => {
        const figure = createFigure('tPose', DIMS);
        const hidden = landmarksFromFigure(figure, FRAME).map((l, i) => (
            i === LANDMARK.elbowL || i === LANDMARK.wristL ? { ...l, visibility: 0.1 } : l
        ));
        const result = figureFromLandmarks(hidden, { frame: FRAME, reference: figure })!;
        // MediaPipe's left is our right.
        expect(result.fellBack).toContain('elbowR');
        expect(result.figure.joints.elbowR.x).toBeCloseTo(figure.joints.elbowR.x, 3);
        expect(result.confidence).toBeGreaterThan(0.5);
    });

    it('says there is no pose here rather than inventing one', () => {
        const figure = createFigure('standing', DIMS);
        const torsoless = landmarksFromFigure(figure, FRAME).map((l, i) => (
            i === LANDMARK.hipL || i === LANDMARK.hipR ? { ...l, visibility: 0 } : l
        ));
        expect(figureFromLandmarks(torsoless, { frame: FRAME, reference: figure })).toBeNull();
        expect(figureFromLandmarks([] as Landmark[], { frame: FRAME, reference: figure })).toBeNull();
    });
});

describe('turnOfBody', () => {
    it('reads back the angle a figure was built at', () => {
        for (const view of ['front', 'threeQuarter', 'side'] as const) {
            const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS[view]);
            const turn = turnOfBody(figure.joints);
            expect(turn.yaw).toBeCloseTo(VIEW_PRESETS[view].yaw, 0);
            expect(turn.pitch).toBeCloseTo(VIEW_PRESETS[view].pitch, 0);
        }
    });

    it('records the camera angle on import, rather than pretending it is zero', () => {
        // A figure lifted from a three-quarter photograph *is* at three
        // quarters, and the view readout has to say so the moment it is opened.
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.threeQuarter);
        const { figure: back } = roundTrip(figure);
        expect(back.turn?.yaw).toBeCloseTo(VIEW_PRESETS.threeQuarter.yaw, 0);
    });
});

describe('bodyForward', () => {
    it('points out of the screen for a figure facing the camera', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.front);
        expect(bodyForward(figure.joints)!.z).toBeGreaterThan(0.9);
    });

    it('points into it for one facing away', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.back);
        expect(bodyForward(figure.joints)!.z).toBeLessThan(-0.9);
    });
});

describe('the face joint', () => {
    // The sixteenth joint, and the one a photograph is uniquely good for: a
    // body cannot imply where a head is looking.
    it('comes back from the nose landmark', () => {
        const figure = createFigure('lookingBack', DIMS);
        const result = roundTrip(figure);
        expect(result.fellBack).not.toContain('face');
        const a = figure.joints.face;
        const b = result.figure.joints.face;
        expect(Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z)).toBeLessThan(figureUnit(figure) * 0.03);
    });

    it('keeps looking where the body was looking when the nose is hidden', () => {
        const figure = createFigure('standing', DIMS);
        const noNose = landmarksFromFigure(figure, FRAME).map((l, i) => (
            i === LANDMARK.nose ? { ...l, visibility: 0 } : l
        ));
        const result = figureFromLandmarks(noNose, { frame: FRAME, reference: figure })!;
        expect(result.fellBack).toContain('face');
    });
});
