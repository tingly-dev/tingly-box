import { describe, expect, it } from 'vitest';
import type { JointKey, PoseFigure } from './index';
import {
    applyPreset,
    figureTurn,
    isTurnHandleHit,
    MAX_VIEW_PITCH,
    perspectiveAt,
    projectFigure,
    setFigureTurn,
    turnFigure,
    turnHandlePoint,
    unprojectPoint,
    projectionOf,
    VIEW_PRESETS,
    viewPresetOf,
    createFigure,
    distanceToSegment,
    figureBounds,
    figureSolids,
    MANIKIN,
    constrainFigure,
    figureUnit,
    HANDLE_KEYS,
    handlePointOf,
    HEAD_LENGTH_RATIO,
    figureVisualBounds,
    flipFigure,
    hitTestBody,
    hitTestJoint,
    isScaleHandleHit,
    JOINT_KEYS,
    JOINT_PARENT,
    leastUsedShade,
    POSE_LIBRARY,
    swingTargetOf,
    VIEW_PRESET_KEYS,
    type PosePresetKey,
    MIN_FIGURE_UNIT,
    moveJoint,
    nextFigureAt,
    placeNewFigure,
    clampScaleFactor,
    scaleFigure,
    scaleHandlePoint,
    subtreeOf,
    swingJoint,
    transformFigures,
    translateFigure,
} from './index';

const DIMS = { width: 1024, height: 1024 };

// Bone lengths are three-dimensional now, and that is the whole point: what a
// drag preserves is the bone, not its shadow on the screen. A 2D measurement
// of a foreshortened limb is *supposed* to come out short.
const projectPointScale = (point: { z: number }, projection: { anchor: { z: number }; distance: number }) =>
    perspectiveAt(point.z, projection as never);

const bone3 = (figure: PoseFigure, a: JointKey, b: JointKey) => Math.hypot(
    figure.joints[a].x - figure.joints[b].x,
    figure.joints[a].y - figure.joints[b].y,
    (figure.joints[a].z ?? 0) - (figure.joints[b].z ?? 0),
);

describe('createFigure', () => {
    it('centres the figure and sizes it to about 70% of the canvas height', () => {
        // "About", because a figure is now seen through a lens: the default
        // three-quarter view foreshortens it a little and perspective grows
        // whatever is nearest. Dead-on, the old number is exact.
        const figure = createFigure('standing', DIMS);
        const bounds = figureBounds(figure);
        expect(bounds.height).toBeGreaterThan(1024 * 0.62);
        expect(bounds.height).toBeLessThan(1024 * 0.76);
        expect(bounds.x + bounds.width / 2).toBeCloseTo(512, 0);
        expect(bounds.y + bounds.height / 2).toBeCloseTo(512, 0);
        // Dead-on it is the authored 70% plus the toes, which stick out past
        // the heel and are part of what you can see and grab.
        const front = figureBounds(createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.front)).height;
        expect(front).toBeGreaterThan(1024 * 0.7);
        expect(front).toBeLessThan(1024 * 0.72);
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

    it('defines every joint, in three dimensions, for every preset', () => {
        for (const preset of POSE_LIBRARY.flatMap((group) => group.poses)) {
            const figure = createFigure(preset, DIMS);
            for (const key of JOINT_KEYS) {
                expect(Number.isFinite(figure.joints[key].x)).toBe(true);
                expect(Number.isFinite(figure.joints[key].y)).toBe(true);
                expect(Number.isFinite(figure.joints[key].z)).toBe(true);
            }
        }
    });

    it('lands at a three-quarter view rather than dead-on', () => {
        // A front elevation is the one angle at which a 3D pose looks exactly
        // like the flat one it replaced.
        expect(figureTurn(createFigure('standing', DIMS)).yaw).not.toBe(0);
    });
});

describe('applyPreset', () => {
    it('swaps the pose in place, keeping position, size and id', () => {
        const figure = scaleFigure(translateFigure(createFigure('standing', DIMS), 120, -40), 0.5);
        const before = figureBounds(figure);
        const after = applyPreset(figure, 'sitting', DIMS);
        const afterBounds = figureBounds(after);
        expect(after.id).toBe(figure.id);
        // Not the same box — a seated figure is shorter — but the same person:
        // the torso keeps its length and the figure keeps its place.
        expect(afterBounds.height).toBeLessThan(before.height);
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

    it('moves a single joint to where the pointer is, at the depth it had', () => {
        // Alt detaches a joint from its bone length — odd proportions are a
        // drawing choice. The target has to be somewhere the elbow can
        // actually reach, though: Alt escapes the skeleton, not the rig.
        const figure = createFigure('standing', DIMS);
        const at = projectFigure(figure);
        const target = { x: at.wristL.x - 24, y: at.wristL.y - 30 };
        const moved = moveJoint(figure, 'wristL', target);
        const landed = projectFigure(moved).wristL;
        expect(landed.x).toBeCloseTo(target.x, 4);
        expect(landed.y).toBeCloseTo(target.y, 4);
        expect(moved.joints.wristL.z).toBeCloseTo(figure.joints.wristL.z, 6);
        expect(moved.joints.wristR).toEqual(figure.joints.wristR);
        // ...and the bone really did change length, which is the point of Alt.
        const before = Math.hypot(
            figure.joints.wristL.x - figure.joints.elbowL.x,
            figure.joints.wristL.y - figure.joints.elbowL.y,
            figure.joints.wristL.z - figure.joints.elbowL.z,
        );
        const after = Math.hypot(
            moved.joints.wristL.x - moved.joints.elbowL.x,
            moved.joints.wristL.y - moved.joints.elbowL.y,
            moved.joints.wristL.z - moved.joints.elbowL.z,
        );
        expect(Math.abs(after - before)).toBeGreaterThan(1);
    });

    it('will not let Alt put a joint somewhere the rig forbids', () => {
        // The escape hatch is an escape from bone length, not from anatomy.
        const figure = createFigure('standing', DIMS);
        const at = projectFigure(figure);
        // Straight up past the shoulder: that folds the elbow backwards.
        const moved = moveJoint(figure, 'wristL', { x: at.elbowL.x, y: at.shoulderL.y - 200 });
        const landed = projectFigure(moved).wristL;
        expect(Math.hypot(landed.x - at.elbowL.x, landed.y - (at.shoulderL.y - 200)))
            .toBeGreaterThan(1);
    });

    it('scales about the centre, leaving it fixed', () => {
        const figure = createFigure('standing', DIMS);
        const before = figureBounds(figure);
        const scaled = scaleFigure(figure, 2);
        const after = figureBounds(scaled);
        expect(after.height).toBeCloseTo(before.height * 2, 4);
        expect(after.x + after.width / 2).toBeCloseTo(before.x + before.width / 2, 4);
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


describe('the rig', () => {
    // What the mannequin knows about itself. Before this there was nothing
    // anywhere that knew an elbow is a hinge: a pose was a table of absolute
    // angles and a drag was a free rotation, so both the authored library and
    // the user could make a body no body can make.
    const bones = (figure: PoseFigure) => JOINT_KEYS
        .filter((key) => JOINT_PARENT[key])
        .map((key) => {
            const parent = JOINT_PARENT[key]!;
            return Math.hypot(
                figure.joints[key].x - figure.joints[parent].x,
                figure.joints[key].y - figure.joints[parent].y,
                figure.joints[key].z - figure.joints[parent].z,
            );
        });

    // Which way a hinge folded, as a signed number along the body's own
    // left-right axis. Both reference signs are taken from poses that are
    // right by eye rather than derived: the canvas y axis points down, which
    // flips the handedness of every cross product, and the hand-derived sign
    // for this was wrong the first time it was written.
    const foldOf = (figure: PoseFigure, root: JointKey, joint: JointKey, tip: JointKey) => {
        const J = figure.joints;
        const sub = (a: typeof J.hip, b: typeof J.hip) => ({ x: a.x - b.x, y: a.y - b.y, z: a.z - b.z });
        const norm = (a: { x: number; y: number; z: number }) => {
            const l = Math.hypot(a.x, a.y, a.z) || 1;
            return { x: a.x / l, y: a.y / l, z: a.z / l };
        };
        const dot = (a: { x: number; y: number; z: number }, b: { x: number; y: number; z: number }) => a.x * b.x + a.y * b.y + a.z * b.z;
        const cross = (a: { x: number; y: number; z: number }, b: { x: number; y: number; z: number }) => ({
            x: a.y * b.z - a.z * b.y, y: a.z * b.x - a.x * b.z, z: a.x * b.y - a.y * b.x,
        });
        const up = norm(sub(J.neck, J.hip));
        const span = sub(J.shoulderR, J.shoulderL);
        const across = norm({
            x: span.x - up.x * dot(span, up), y: span.y - up.y * dot(span, up), z: span.z - up.z * dot(span, up),
        });
        const u = norm(sub(J[joint], J[root]));
        const d = norm(sub(J[tip], J[joint]));
        return { bend: Math.acos(Math.max(-1, Math.min(1, dot(u, d)))) * 180 / Math.PI, fold: dot(norm(cross(u, d)), across) };
    };

    it('leaves a straight limb alone instead of jittering it', () => {
        // The first version of this had the deadband's sense inverted — it
        // read as "skip anything bent less than 83 degrees", which switched
        // the whole rig off for almost every pose while looking like it ran.
        const figure = createFigure('tPose', DIMS, undefined, 0, VIEW_PRESETS.front);
        const again = constrainFigure(figure);
        for (const key of JOINT_KEYS) {
            expect(Math.hypot(
                again.joints[key].x - figure.joints[key].x,
                again.joints[key].y - figure.joints[key].y,
                again.joints[key].z - figure.joints[key].z,
            )).toBeLessThan(figureUnit(figure) * 0.001);
        }
    });

    it('folds a knee backwards and an elbow forwards, in every pose in the library', () => {
        // The whole library, measured. Before the rig this split roughly
        // seven to one on the knee and three to one on the elbow — and a
        // cross-legged knee folded a full ninety degrees out of its plane.
        for (const pose of POSE_LIBRARY.flatMap((group) => group.poses)) {
            const figure = createFigure(pose, DIMS);
            for (const side of ['L', 'R'] as const) {
                const knee = foldOf(figure, `hip${side}` as JointKey, `knee${side}` as JointKey, `ankle${side}` as JointKey);
                if (knee.bend > 20) expect(knee.fold, `${pose} knee${side}`).toBeLessThan(0);
                const elbow = foldOf(figure, `shoulder${side}` as JointKey, `elbow${side}` as JointKey, `wrist${side}` as JointKey);
                if (elbow.bend > 20) expect(elbow.fold, `${pose} elbow${side}`).toBeGreaterThan(0);
            }
        }
    });

    it('pulls an elbow bent the wrong way back to one it can reach', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.front);
        const at = projectFigure(figure);
        // Swinging the wrist up past the shoulder folds the forearm backwards.
        const wrong = swingJoint(figure, 'wristL', { x: at.elbowL.x, y: at.shoulderL.y - 120 });
        const after = foldOf(wrong, 'shoulderL', 'elbowL', 'wristL');
        if (after.bend > 20) expect(after.fold).toBeGreaterThan(0);
    });

    it('never stretches a bone to satisfy a limit', () => {
        // A constraint that lengthens a bone has traded one impossible body
        // for another.
        for (const pose of POSE_LIBRARY.flatMap((group) => group.poses)) {
            const figure = createFigure(pose, DIMS);
            const before = bones(figure);
            const after = bones(constrainFigure(figure));
            for (let i = 0; i < before.length; i += 1) {
                expect(after[i]).toBeCloseTo(before[i], 3);
            }
        }
    });

    it('is settled: constraining an already legal figure changes nothing', () => {
        // Otherwise every drag frame would creep, and a pose would drift while
        // the pointer stood still.
        for (const pose of ['crossLegged', 'sitting', 'running', 'reclining'] as PosePresetKey[]) {
            const figure = createFigure(pose, DIMS);
            const again = constrainFigure(figure);
            for (const key of JOINT_KEYS) {
                expect(Math.hypot(
                    again.joints[key].x - figure.joints[key].x,
                    again.joints[key].y - figure.joints[key].y,
                    again.joints[key].z - figure.joints[key].z,
                ), `${pose} ${key}`).toBeLessThan(figureUnit(figure) * 0.002);
            }
        }
    });
});

describe('handles you can actually grab', () => {
    // Measured, because "it feels hard to control" is otherwise unfalsifiable.
    // The dialog draws handles at 7 screen px and picks within 14; on a 1024
    // canvas shown at about 800 css px that is roughly 9 and 18 canvas px.
    const DRAWN = 9;
    const PICK = 18;

    const tightest = (pose: PosePresetKey, view: keyof typeof VIEW_PRESETS) => {
        const figure = createFigure(pose, DIMS, undefined, 0, VIEW_PRESETS[view]);
        let worst = Infinity;
        let pair = '';
        for (let i = 0; i < HANDLE_KEYS.length; i += 1) {
            for (let j = i + 1; j < HANDLE_KEYS.length; j += 1) {
                const a = handlePointOf(figure, HANDLE_KEYS[i], DRAWN);
                const b = handlePointOf(figure, HANDLE_KEYS[j], DRAWN);
                const gap = Math.hypot(a.x - b.x, a.y - b.y);
                if (gap < worst) {
                    worst = gap;
                    pair = `${HANDLE_KEYS[i]}/${HANDLE_KEYS[j]}`;
                }
            }
        }
        return { worst, pair };
    };

    it('does not give the hip root a handle of its own', () => {
        // Dragging it translated the figure, which dragging the body already
        // does; it sat between hipL and hipR and made the pelvis a pile of
        // three dots. A handle that does nothing new is noise (principle 9).
        expect(HANDLE_KEYS).not.toContain('hip');
        expect(HANDLE_KEYS).toHaveLength(JOINT_KEYS.length - 1);
    });

    it('keeps the face handle clear of the head it points out of', () => {
        // The face bone points out of the skull, so a head turned toward or
        // away from the camera used to project the two handles onto the same
        // pixel — on this library, in more than half of every pose and view.
        for (const view of VIEW_PRESET_KEYS) {
            for (const pose of ['standing', 'lying', 'sitting', 'lookingBack'] as PosePresetKey[]) {
                const figure = createFigure(pose, DIMS, undefined, 0, VIEW_PRESETS[view]);
                const head = handlePointOf(figure, 'head', DRAWN);
                const face = handlePointOf(figure, 'face', DRAWN);
                expect(Math.hypot(head.x - face.x, head.y - face.y)).toBeGreaterThan(DRAWN * 2);
            }
        }
    });

    it('carries a pointer on the face dial back to the aim it means', () => {
        // Pushing the handle out must not cost the depth the drag reads inside
        // the bone's ball: dragging it all the way in still means "looking
        // straight at the camera".
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.threeQuarter);
        const head = projectFigure(figure).head;
        const handle = handlePointOf(figure, 'face', DRAWN);
        const target = swingTargetOf(figure, 'face', handle, DRAWN);
        // The handle's own position maps back to where the face really is.
        const face = projectFigure(figure).face;
        expect(Math.hypot(target.x - face.x, target.y - face.y)).toBeLessThan(1);
        // ...and the inner end of the dial maps to the head itself, which is
        // the bone pointing at the camera.
        const dir = { x: handle.x - head.x, y: handle.y - head.y };
        const len = Math.hypot(dir.x, dir.y);
        const inner = { x: head.x + (dir.x / len) * 1, y: head.y + (dir.y / len) * 1 };
        const aimed = swingTargetOf(figure, 'face', inner, DRAWN);
        expect(Math.hypot(aimed.x - head.x, aimed.y - head.y)).toBeLessThan(1);
    });

    it('picks the joint further down the chain when two handles coincide', () => {
        // A wrist over an elbow, a knee over a hip: the finer control is always
        // the one meant, and without this rule a foreshortened limb hands you
        // the wrong one at random.
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.front);
        const wrist = handlePointOf(figure, 'wristR', DRAWN);
        expect(hitTestJoint(figure, wrist, PICK, DRAWN)).toBe('wristR');
        const head = handlePointOf(figure, 'head', DRAWN);
        expect(hitTestJoint(figure, head, PICK, DRAWN)).toBe('head');
    });

    it('leaves few enough collisions across the whole library to matter', () => {
        // The number this replaces: 53% of pose-and-view combinations had a
        // pair of handles closer than the pick radius, and 41 of 216 had two
        // within half of it. Those are the ones you simply cannot separate.
        const rows = POSE_LIBRARY.flatMap((group) => group.poses).flatMap(
            (pose) => VIEW_PRESET_KEYS.map((view) => tightest(pose, view)),
        );
        const ambiguous = rows.filter((row) => row.worst < PICK).length;
        const unpickable = rows.filter((row) => row.worst < PICK / 2).length;
        expect(ambiguous / rows.length).toBeLessThan(0.3);
        expect(unpickable).toBeLessThan(12);
    });
});

describe('figureSolids', () => {
    // The only description of the body. The renderer draws it, the hit test
    // projects it, so one list is what keeps grab and sight from drifting.
    it('is two blocks, a waist, a head, and the limbs, sized off the body', () => {
        const figure = createFigure('standing', DIMS);
        const solids = figureSolids(figure);
        expect(solids.filter((s) => s.kind === 'block')).toHaveLength(2);
        const thigh = solids.find((s) => s.kind === 'capsule' && s.from === figure.joints.hipL);
        expect(thigh && thigh.kind === 'capsule' ? thigh.fromRadius : 0)
            .toBeCloseTo(MANIKIN.thigh * figureUnit(figure), 6);
    });

    it('scales every radius with the figure', () => {
        const base = createFigure('standing', DIMS);
        const big = scaleFigure(base, 2);
        const radii = (f: PoseFigure) => figureSolids(f)
            .map((s) => (s.kind === 'sphere' ? s.radius : s.kind === 'capsule' ? s.fromRadius : s.unit));
        const a = radii(base);
        const b = radii(big);
        for (let i = 0; i < a.length; i += 1) expect(b[i]).toBeCloseTo(a[i] * 2, 6);
    });

    it('is what the body hit test sees', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.front);
        const at = projectFigure(figure);
        // Chest centre, and the middle of a thigh.
        expect(hitTestBody(figure, { x: at.neck.x, y: at.neck.y + (at.hip.y - at.neck.y) * 0.3 })).toBe(true);
        expect(hitTestBody(figure, { x: (at.hipL.x + at.kneeL.x) / 2, y: (at.hipL.y + at.kneeL.y) / 2 })).toBe(true);
        // Well out beside the waist: paper.
        expect(hitTestBody(figure, { x: at.neck.x + figureUnit(figure) * 0.3, y: (at.neck.y + at.hip.y) / 2 })).toBe(false);
    });
});

describe('placeNewFigure', () => {
    it('puts the first figure in the middle', () => {
        expect(placeNewFigure([], DIMS)).toEqual({ x: 512, y: 512 });
    });

    it('never drops a figure on top of one already placed', () => {
        const figures: PoseFigure[] = [];
        for (let i = 0; i < 6; i += 1) {
            figures.push(createFigure('standing', DIMS, placeNewFigure(figures, DIMS)));
        }
        const centers = figures.map((figure) => {
            const bounds = figureBounds(figure);
            return { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
        });
        for (let i = 0; i < centers.length; i += 1) {
            for (let j = i + 1; j < centers.length; j += 1) {
                expect(Math.hypot(centers[i].x - centers[j].x, centers[i].y - centers[j].y)).toBeGreaterThan(1);
            }
        }
    });

    it('keeps cascading, inside the canvas, once every candidate is taken', () => {
        const crowd = Array.from({ length: 12 }, (_, i) => createFigure('standing', DIMS, { x: 40 * i, y: 40 * i }));
        const spot = placeNewFigure(crowd, DIMS);
        expect(spot.x).toBeGreaterThan(0);
        expect(spot.x).toBeLessThanOrEqual(DIMS.width);
        expect(spot.y).toBeLessThanOrEqual(DIMS.height);
    });
});

describe('nextFigureAt', () => {
    const stack = () => {
        const a = createFigure('standing', DIMS, { x: 500, y: 512 });
        const b = createFigure('standing', DIMS, { x: 520, y: 512 });
        return [a, b];
    };

    it('returns the top figure when nothing is selected', () => {
        const [a, b] = stack();
        expect(nextFigureAt([a, b], b.joints.hip, null)?.id).toBe(b.id);
    });

    it('walks down the pile on repeated clicks and wraps around', () => {
        const [a, b] = stack();
        const point = b.joints.hip;
        const first = nextFigureAt([a, b], point, null);
        expect(first?.id).toBe(b.id);
        const second = nextFigureAt([a, b], point, first?.id ?? null);
        expect(second?.id).toBe(a.id);
        expect(nextFigureAt([a, b], point, second?.id ?? null)?.id).toBe(b.id);
    });

    it('returns nothing on empty canvas space', () => {
        const [a, b] = stack();
        expect(nextFigureAt([a, b], { x: 5, y: 5 }, null)).toBeNull();
    });
});

describe('shades', () => {
    it('carries the shade through preset swaps', () => {
        const figure = createFigure('standing', DIMS, undefined, 2);
        expect(applyPreset(figure, 'walking', DIMS).shade).toBe(2);
    });

    it('hands the first three figures three different tones', () => {
        const figures: PoseFigure[] = [];
        for (let i = 0; i < 3; i += 1) {
            figures.push(createFigure('standing', DIMS, undefined, leastUsedShade(figures)));
        }
        expect(figures.map((figure) => figure.shade)).toEqual([0, 1, 2]);
    });

    it('reuses the freed tone after a delete instead of colliding', () => {
        const figures: PoseFigure[] = [];
        for (let i = 0; i < 3; i += 1) {
            figures.push(createFigure('standing', DIMS, undefined, leastUsedShade(figures)));
        }
        const remaining = [figures[0], figures[2]];
        expect(leastUsedShade(remaining)).toBe(1);
        const next = createFigure('standing', DIMS, undefined, leastUsedShade(remaining));
        expect(remaining.some((figure) => figure.shade === next.shade)).toBe(false);
    });

    it('treats a figure saved before shades existed as the first tone', () => {
        const legacy = { ...createFigure('standing', DIMS), shade: undefined };
        expect(leastUsedShade([legacy])).toBe(1);
    });
});

describe('the skeleton', () => {
    const boneLength = bone3;

    it('hangs every joint off the hip, the face included', () => {
        expect(subtreeOf('hip')).toHaveLength(JOINT_KEYS.length);
        expect(subtreeOf('shoulderL').sort()).toEqual(['elbowL', 'shoulderL', 'wristL']);
        // The face is an ordinary child: dragging the head takes it along,
        // with no new machinery.
        expect(subtreeOf('head').sort()).toEqual(['face', 'head']);
        expect(subtreeOf('face')).toEqual(['face']);
    });

    it('swings the limb below the joint and keeps every bone length', () => {
        const figure = createFigure('standing', DIMS);
        const before = { upper: boneLength(figure, 'shoulderL', 'elbowL'), fore: boneLength(figure, 'elbowL', 'wristL') };
        const at = projectFigure(figure);
        const swung = swingJoint(figure, 'elbowL', { x: at.elbowL.x - 200, y: at.elbowL.y - 40 });
        expect(boneLength(swung, 'shoulderL', 'elbowL')).toBeCloseTo(before.upper, 4);
        expect(boneLength(swung, 'elbowL', 'wristL')).toBeCloseTo(before.fore, 4);
        // The wrist came along; the shoulder it hangs from did not move.
        expect(swung.joints.wristL.x).not.toBeCloseTo(figure.joints.wristL.x, 1);
        // Rigid: the bone between them came along whole, in three dimensions.
        expect(boneLength(swung, 'elbowL', 'wristL')).toBeCloseTo(boneLength(figure, 'elbowL', 'wristL'), 4);
        expect(swung.joints.shoulderL).toEqual(figure.joints.shoulderL);
        expect(swung.joints.wristR).toEqual(figure.joints.wristR);
    });

    it('lays the bone in the picture plane once the pointer is past its reach', () => {
        // Beyond the silhouette of the bone's sphere there is no depth left to
        // give: the limb simply points at the pointer, exactly as it did when
        // the figure was flat.
        const figure = createFigure('standing', DIMS, undefined, 0, { yaw: 0, pitch: 0 });
        const at = projectFigure(figure);
        const swung = swingJoint(figure, 'elbowR', { x: at.shoulderR.x + 400, y: at.shoulderR.y });
        const shoulder = swung.joints.shoulderR;
        const elbow = swung.joints.elbowR;
        expect(elbow.z - shoulder.z).toBeCloseTo(0, 6);
        expect(Math.atan2(elbow.y - shoulder.y, elbow.x - shoulder.x)).toBeCloseTo(0, 6);
        expect(boneLength(swung, 'shoulderR', 'elbowR'))
            .toBeCloseTo(boneLength(figure, 'shoulderR', 'elbowR'), 4);
    });

    it('sends the bone out of the screen when the pointer is inside its reach', () => {
        // The one thing a flat figure could never say. Dragging a joint to
        // half the bone's projected length means the rest of it is pointing
        // at the viewer — there is no other length-preserving answer.
        const figure = createFigure('standing', DIMS, undefined, 0, { yaw: 0, pitch: 0 });
        const at = projectFigure(figure);
        const reach = boneLength(figure, 'shoulderR', 'elbowR');
        const near = { x: at.shoulderR.x + reach * 0.4, y: at.shoulderR.y };
        const swung = swingJoint(figure, 'elbowR', near);
        expect(swung.joints.elbowR.z - swung.joints.shoulderR.z).toBeGreaterThan(reach * 0.5);
        expect(boneLength(swung, 'shoulderR', 'elbowR')).toBeCloseTo(reach, 4);
        // ...and the same drag with Shift sends it behind the body instead.
        const away = swingJoint(figure, 'elbowR', near, { away: true });
        expect(away.joints.elbowR.z - away.joints.shoulderR.z).toBeLessThan(-reach * 0.5);
        expect(boneLength(away, 'shoulderR', 'elbowR')).toBeCloseTo(reach, 4);
    });

    it('moves the whole figure when the root is dragged', () => {
        const figure = createFigure('standing', DIMS);
        const at = projectFigure(figure);
        const swung = swingJoint(figure, 'hip', { x: at.hip.x + 30, y: at.hip.y - 15 });
        for (const key of JOINT_KEYS) {
            expect(swung.joints[key].x).toBeCloseTo(figure.joints[key].x + 30, 4);
            expect(swung.joints[key].y).toBeCloseTo(figure.joints[key].y - 15, 4);
        }
    });

    it('ignores a drag that lands on the parent, which gives no direction', () => {
        const figure = createFigure('standing', DIMS);
        expect(swingJoint(figure, 'kneeL', projectFigure(figure).hipL)).toBe(figure);
    });

    it('leaves proportions intact however far a pose is pushed', () => {
        let figure = createFigure('standing', DIMS);
        const before = JOINT_KEYS.map((key) => {
            const parent = JOINT_PARENT[key];
            return parent ? boneLength(figure, key, parent) : 0;
        });
        for (const key of ['elbowL', 'kneeR', 'shoulderR', 'wristL', 'head'] as JointKey[]) {
            figure = swingJoint(figure, key, { x: 900, y: 120 });
        }
        const after = JOINT_KEYS.map((key) => {
            const parent = JOINT_PARENT[key];
            return parent ? boneLength(figure, key, parent) : 0;
        });
        after.forEach((length, i) => expect(length).toBeCloseTo(before[i], 4));
    });
});

describe('the pose library', () => {
    const everyPose = POSE_LIBRARY.flatMap((group) => group.poses);
    const boneLengths = (figure: PoseFigure) => JOINT_KEYS.map((key) => {
        const parent = JOINT_PARENT[key];
        return parent ? bone3(figure, key, parent) : 0;
    });

    it('lists every pose once, in a group', () => {
        expect(new Set(everyPose).size).toBe(everyPose.length);
        expect(everyPose.length).toBeGreaterThanOrEqual(30);
    });

    it('builds every pose from the same bone lengths', () => {
        // Otherwise applying a pose would silently restretch the figure, which
        // is exactly what the skeleton exists to prevent.
        const reference = boneLengths(createFigure('standing', DIMS));
        for (const pose of everyPose) {
            boneLengths(createFigure(pose, DIMS)).forEach((length, i) => {
                expect(length).toBeCloseTo(reference[i], 3);
            });
        }
    });

    it('lets a crouch be shorter than a stand instead of stretching it', () => {
        const standing = figureBounds(createFigure('standing', DIMS)).height;
        expect(figureBounds(createFigure('crouching', DIMS)).height).toBeLessThan(standing * 0.92);
        expect(figureBounds(createFigure('lying', DIMS)).height).toBeLessThan(standing * 0.5);
    });

    it('stands every pose up except the ones that are meant to be horizontal', () => {
        // Bowing is no longer on the list: it used to fold sideways across the
        // screen because that was the only fold a flat figure had. It now
        // folds forward, out of the picture, which is what bowing is.
        const horizontal = new Set(['lying', 'lyingSide', 'prone', 'pushUp']);
        for (const pose of everyPose) {
            const figure = createFigure(pose, DIMS, undefined, 0, { yaw: 0, pitch: 0 });
            const rise = figure.joints.hip.y - figure.joints.neck.y;
            const run = Math.abs(figure.joints.neck.x - figure.joints.hip.x);
            expect(rise > run).toBe(!horizontal.has(pose));
        }
    });

    it('swaps a pose without resizing the person or moving the camera', () => {
        const figure = turnFigure(scaleFigure(createFigure('standing', DIMS), 0.6), 17, -4);
        for (const pose of everyPose) {
            const swapped = applyPreset(figure, pose, DIMS);
            expect(figureUnit(swapped)).toBeCloseTo(figureUnit(figure), 3);
            expect(figureTurn(swapped)).toEqual(figureTurn(figure));
        }
    });

    it('uses the third dimension in most of the library, not as a garnish', () => {
        // A pose whose joints are all at one depth is a flat pose wearing a
        // 3D data structure. The calibration pose is allowed to be one.
        const flat = everyPose.filter((pose) => {
            const figure = createFigure(pose, DIMS, undefined, 0, { yaw: 0, pitch: 0 });
            // The body only: the face marker points out of the screen in every
            // pose, so counting it would make the test vacuous.
            const zs = JOINT_KEYS.filter((key) => key !== 'face')
                .map((key) => figure.joints[key].z);
            return Math.max(...zs) - Math.min(...zs) < figureUnit(figure) * 0.05;
        });
        expect(flat).toEqual(['tPose']);
    });
});

describe('figure scale is the body, not the bounding box', () => {
    it('gives a lying figure the same limbs as a standing one', () => {
        // The lying pose has a fifth of the box height but the same bones.
        // Sizing anything off the box turns it into the stick figure the
        // manikin exists to avoid.
        const standing = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.front);
        const lying = createFigure('lying', DIMS, undefined, 0, VIEW_PRESETS.front);
        expect(figureBounds(lying).height).toBeLessThan(figureBounds(standing).height * 0.4);
        expect(figureUnit(lying)).toBeCloseTo(figureUnit(standing), 4);
        // Girth is within a few percent, not identical: what is left of the
        // difference is perspective, which is a thing about the camera rather
        // than about the body.
        // Measured on the solid itself: the thigh capsule's radius at the hip,
        // which is what "girth" is once the body is a list of solids.
        const thighGirth = (f: PoseFigure) => {
            const thigh = figureSolids(f).find((solid) => solid.kind === 'capsule'
                && solid.from === f.joints.hipL && solid.to === f.joints.kneeL);
            return thigh && thigh.kind === 'capsule' ? thigh.fromRadius : 0;
        };
        const ratio = thighGirth(lying) / thighGirth(standing);
        expect(Number.isFinite(ratio)).toBe(true);
        expect(ratio).toBeGreaterThan(0.9);
        expect(ratio).toBeLessThan(1.1);
    });

    it('keeps a lying figure as grabbable as a standing one', () => {
        const lying = createFigure('lying', DIMS);
        expect(hitTestBody(lying, lying.joints.kneeL)).toBe(true);
        const beside = { x: lying.joints.kneeL.x, y: lying.joints.kneeL.y - figureUnit(lying) * 0.02 };
        expect(hitTestBody(lying, beside)).toBe(true);
    });

    it('does not change thickness when a limb is swung', () => {
        const figure = createFigure('standing', DIMS);
        const swung = swingJoint(figure, 'elbowL', { x: 0, y: 0 });
        expect(figureUnit(swung)).toBeCloseTo(figureUnit(figure), 4);
    });
});

describe('clampScaleFactor', () => {
    it('stops a resize drag at the grabbable minimum', () => {
        const figure = createFigure('standing', DIMS);
        const factor = clampScaleFactor(figure, 0.0001);
        expect(figureUnit(scaleFigure(figure, factor))).toBeCloseTo(MIN_FIGURE_UNIT, 4);
    });

    it('holds every pose to the same physical minimum, not the same box', () => {
        // A bounding box would let a lying figure shrink to a fraction of the
        // size a standing one is held at.
        for (const pose of ['standing', 'lying', 'crouching'] as const) {
            const figure = createFigure(pose, DIMS);
            const smallest = scaleFigure(figure, clampScaleFactor(figure, 0.0001));
            expect(figureUnit(smallest)).toBeCloseTo(MIN_FIGURE_UNIT, 4);
        }
    });

    it('never turns a shrink into a growth on an already tiny figure', () => {
        const tiny = scaleFigure(createFigure('standing', DIMS), 0.01);
        expect(clampScaleFactor(tiny, 0.5)).toBeLessThanOrEqual(1);
        expect(figureBounds(scaleFigure(tiny, clampScaleFactor(tiny, 0.5))).height)
            .toBeLessThanOrEqual(figureBounds(tiny).height);
    });

    it('leaves a growth alone', () => {
        expect(clampScaleFactor(createFigure('standing', DIMS), 2)).toBe(2);
    });

    it('applies a pose to a tiny figure without inflating it', () => {
        const tiny = scaleFigure(createFigure('standing', DIMS), 0.03);
        expect(figureUnit(applyPreset(tiny, 'lying', DIMS))).toBeCloseTo(figureUnit(tiny), 4);
    });
});

describe('transformFigures', () => {
    it('moves and scales every joint together', () => {
        const figure = createFigure('standing', DIMS);
        const [moved] = transformFigures([figure], { scale: 0.5, dx: 10, dy: -4 });
        for (const key of JOINT_KEYS) {
            expect(moved.joints[key].x).toBeCloseTo(figure.joints[key].x * 0.5 + 10, 4);
            expect(moved.joints[key].y).toBeCloseTo(figure.joints[key].y * 0.5 - 4, 4);
        }
    });
});

describe('the camera', () => {
    it('projects nearer joints bigger and farther ones smaller', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, { yaw: 0, pitch: 0 });
        const projection = projectionOf(figure);
        const near = { ...figure.joints.hip, z: figure.joints.hip.z + figureUnit(figure) * 0.4 };
        const far = { ...figure.joints.hip, z: figure.joints.hip.z - figureUnit(figure) * 0.4 };
        expect(projectPointScale(near, projection)).toBeGreaterThan(1);
        expect(projectPointScale(far, projection)).toBeLessThan(1);
    });

    it('unprojects back to where it projected from', () => {
        const figure = createFigure('reaching', DIMS);
        const projection = projectionOf(figure);
        const at = projectFigure(figure);
        const back = unprojectPoint(at.wristR, figure.joints.wristR.z, projection);
        expect(back.x).toBeCloseTo(figure.joints.wristR.x, 6);
        expect(back.y).toBeCloseTo(figure.joints.wristR.y, 6);
    });

    it('aims at the torso, so posing a limb never nudges the rest of the figure', () => {
        // The camera used to be aimed at the centre of the joint cloud, which
        // every limb moves: dragging a wrist re-projected — and visibly slid —
        // the whole body.
        const figure = createFigure('standing', DIMS);
        const at = projectFigure(figure);
        const swung = swingJoint(figure, 'wristL', { x: at.wristL.x + 90, y: at.wristL.y - 120 });
        const after = projectFigure(swung);
        for (const key of ['head', 'neck', 'hip', 'kneeR', 'ankleR'] as JointKey[]) {
            expect(after[key].x).toBeCloseTo(at[key].x, 6);
            expect(after[key].y).toBeCloseTo(at[key].y, 6);
        }
    });

    it('turns without drift, however many times it is turned', () => {
        // Composing deltas onto the joints works roll into the body; deriving
        // them from the totals cannot.
        const figure = createFigure('standing', DIMS, undefined, 0, { yaw: 0, pitch: 0 });
        let walked = figure;
        for (let i = 0; i < 40; i += 1) walked = turnFigure(walked, 9, 1.5);
        for (let i = 0; i < 40; i += 1) walked = turnFigure(walked, -9, -1.5);
        expect(figureTurn(walked).yaw).toBeCloseTo(0, 6);
        expect(figureTurn(walked).pitch).toBeCloseTo(0, 6);
        for (const key of JOINT_KEYS) {
            expect(walked.joints[key].x).toBeCloseTo(figure.joints[key].x, 3);
            expect(walked.joints[key].z).toBeCloseTo(figure.joints[key].z, 3);
        }
    });

    it('keeps the figure where it was on the canvas while it turns', () => {
        const figure = createFigure('walking', DIMS);
        const before = figureBounds(figure);
        const turned = setFigureTurn(figure, VIEW_PRESETS.side);
        const after = figureBounds(turned);
        expect(after.x + after.width / 2).toBeCloseTo(before.x + before.width / 2, 4);
        expect(after.y + after.height / 2).toBeCloseTo(before.y + before.height / 2, 4);
        // ...and it is the same body, only seen from elsewhere.
        expect(figureUnit(turned)).toBeCloseTo(figureUnit(figure), 4);
    });

    it('goes all the way round but stops short of looking down its own axis', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, { yaw: 0, pitch: 0 });
        expect(figureTurn(turnFigure(figure, 200, 0)).yaw).toBeCloseTo(-160, 6);
        expect(figureTurn(turnFigure(figure, 0, 200)).pitch).toBe(MAX_VIEW_PITCH);
        expect(figureTurn(turnFigure(figure, 0, -200)).pitch).toBe(-MAX_VIEW_PITCH);
    });

    it('names the view when it is one and reports the angles when it is not', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, VIEW_PRESETS.side);
        expect(viewPresetOf(figure)).toBe('side');
        expect(viewPresetOf(turnFigure(figure, 13, 0))).toBeNull();
    });

    it('mirrors the view along with the body', () => {
        // Mirroring a figure turned 35° to its left leaves it turned 35° to
        // its right, and the readout has to say so.
        const figure = createFigure('standing', DIMS, undefined, 0, { yaw: 35, pitch: 8 });
        expect(figureTurn(flipFigure(figure))).toEqual({ yaw: -35, pitch: 8 });
    });

    it('puts the turn grip opposite the scale grip', () => {
        const figure = createFigure('standing', DIMS);
        const visual = figureVisualBounds(figure);
        const grip = turnHandlePoint(figure);
        expect(grip.x).toBeCloseTo(visual.x, 4);
        expect(grip.y).toBeCloseTo(visual.y + visual.height, 4);
        expect(isTurnHandleHit(figure, { x: grip.x - 4, y: grip.y + 4 }, 12)).toBe(true);
        expect(isScaleHandleHit(figure, grip, 12)).toBe(false);
    });

    it('scales depth with the figure, so shrinking never flattens it', () => {
        const figure = createFigure('reaching', DIMS);
        const spread = (f: PoseFigure) => Math.max(...JOINT_KEYS.map((key) => f.joints[key].z))
            - Math.min(...JOINT_KEYS.map((key) => f.joints[key].z));
        expect(spread(scaleFigure(figure, 0.5))).toBeCloseTo(spread(figure) * 0.5, 4);
        expect(spread(transformFigures([figure], { scale: 0.25, dx: 3, dy: 7 })[0]))
            .toBeCloseTo(spread(figure) * 0.25, 4);
    });
});
