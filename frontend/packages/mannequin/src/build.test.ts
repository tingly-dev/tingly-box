import { describe, expect, it } from 'vitest';
import {
    applyPreset,
    createFigure,
    figureBuild,
    figureSolids,
    figureUnit,
    FIGURE_BUILDS,
    JOINT_KEYS,
    JOINT_PARENT,
    LEGACY_BUILD,
    POSE_LIBRARY,
    setFigureBuild,
    swingJoint,
    transformFigures,
    VIEW_PRESETS,
    type JointKey,
    type PoseFigure,
} from './index';
import { figureFromLandmarks, landmarksFromFigure } from './landmarks';
import { dist3, norm3, sub3, dot3 } from './vec3';

const DIMS = { width: 1024, height: 1024 };
const everyPose = POSE_LIBRARY.flatMap((group) => group.poses);
const span = (figure: PoseFigure, l: JointKey, r: JointKey) => dist3(figure.joints[l], figure.joints[r]);
const directionOf = (figure: PoseFigure, key: JointKey) =>
    norm3(sub3(figure.joints[key], figure.joints[JOINT_PARENT[key]!]));

describe('two builds, one library', () => {
    it('builds every pose on both skeletons', () => {
        for (const build of FIGURE_BUILDS) {
            for (const pose of everyPose) {
                const figure = createFigure(pose, DIMS, undefined, 0, undefined, build);
                expect(figureBuild(figure)).toBe(build);
                for (const key of JOINT_KEYS) {
                    expect(Number.isFinite(figure.joints[key].z), `${build} ${pose} ${key}`).toBe(true);
                }
            }
        }
    });

    it('differs at the girdles and nowhere else', () => {
        const female = createFigure('tPose', DIMS, undefined, 0, VIEW_PRESETS.front, 'female');
        const male = createFigure('tPose', DIMS, undefined, 0, VIEW_PRESETS.front, 'male');
        // Same person-size, so swapping build never resizes anyone.
        expect(figureUnit(female)).toBeCloseTo(figureUnit(male), 6);
        // Male: shoulders clearly wider than hips. Female: the two lines close
        // to equal, shoulders narrower and hips wider than his.
        expect(span(male, 'shoulderL', 'shoulderR') / span(male, 'hipL', 'hipR')).toBeGreaterThan(1.5);
        expect(span(female, 'shoulderL', 'shoulderR') / span(female, 'hipL', 'hipR')).toBeLessThan(1.35);
        expect(span(female, 'shoulderL', 'shoulderR')).toBeLessThan(span(male, 'shoulderL', 'shoulderR'));
        expect(span(female, 'hipL', 'hipR')).toBeGreaterThan(span(male, 'hipL', 'hipR'));
        for (const [a, b] of [['shoulderL', 'elbowL'], ['elbowL', 'wristL'], ['hipL', 'kneeL'], ['kneeL', 'ankleL'], ['hip', 'neck']] as const) {
            expect(dist3(female.joints[a], female.joints[b])).toBeCloseTo(dist3(male.joints[a], male.joints[b]), 6);
        }
    });

    it('gives the female build a bust and glutes and the male build neither', () => {
        const count = (build: 'female' | 'male') => figureSolids(createFigure('standing', DIMS, undefined, 0, undefined, build))
            .filter((solid) => solid.kind === 'sphere').length;
        expect(count('female') - count('male')).toBe(4);
    });

    it('puts the bust on the front of the chest, whichever way the body lies', () => {
        for (const pose of ['standing', 'bowing', 'allFours', 'lying'] as const) {
            const figure = createFigure(pose, DIMS, undefined, 0, undefined, 'female');
            const male = figureSolids(createFigure(pose, DIMS, undefined, 0, undefined, 'male'));
            // Spheres in list order: waist, head, then the bust pair.
            const extra = figureSolids(figure).filter((solid) => solid.kind === 'sphere').slice(2, 4);
            expect(male.length).toBeGreaterThan(0);
            const J = figure.joints;
            const chest = { x: (J.neck.x + J.hip.x) / 2, y: (J.neck.y + J.hip.y) / 2, z: (J.neck.z + J.hip.z) / 2 };
            const across = norm3(sub3(J.shoulderR, J.shoulderL));
            const up = norm3(sub3(J.neck, J.hip));
            const front = norm3({ x: up.y * across.z - up.z * across.y, y: up.z * across.x - up.x * across.z, z: up.x * across.y - up.y * across.x });
            for (const solid of extra) {
                if (solid.kind !== 'sphere') continue;
                expect(dot3(sub3(solid.center, chest), front), pose).toBeGreaterThan(0);
            }
        }
    });

    it('lays face-down poses with the chest to the floor and supine ones with it to the sky', () => {
        // The floor is screen-down (+y). Front = across × up's partner, as in
        // `bodyForwardOf`: measured on the female build because her bust makes
        // a wrong answer visible at a glance.
        const frontY = (pose: 'allFours' | 'prone' | 'proneOnElbows' | 'pushUp' | 'lying') => {
            const J = createFigure(pose, DIMS, undefined, 0, VIEW_PRESETS.front, 'female').joints;
            const up = sub3(J.neck, J.hip);
            const across = sub3(J.shoulderR, J.shoulderL);
            return up.z * across.x - up.x * across.z;
        };
        for (const pose of ['allFours', 'prone', 'proneOnElbows', 'pushUp'] as const) expect(frontY(pose), pose).toBeGreaterThan(0);
        expect(frontY('lying')).toBeLessThan(0);
    });

    it('bows forward, not backward', () => {
        // `bend` once had its sign flipped, which made the bow a back-bend that
        // looked identical from the front.
        const J = createFigure('bowing', DIMS, undefined, 0, VIEW_PRESETS.front).joints;
        const up = sub3(J.neck, J.hip);
        const across = sub3(J.shoulderR, J.shoulderL);
        const front = norm3({ x: up.y * across.z - up.z * across.y, y: up.z * across.x - up.x * across.z, z: up.x * across.y - up.y * across.x });
        // The neck has travelled toward where the hips' front used to be: the
        // face of a bowing body points at the floor, so front is mostly +y.
        expect(front.y).toBeGreaterThan(0.4);
    });
});

describe('changing build', () => {
    it('keeps the pose, the size and the place, and changes only the girdles', () => {
        const before = swingJoint(createFigure('kneeUp', DIMS, undefined, 0, undefined, 'female'), 'wristR', { x: 700, y: 300 });
        const after = setFigureBuild(before, 'male');
        expect(figureBuild(after)).toBe('male');
        expect(after.id).toBe(before.id);
        expect(figureUnit(after)).toBeCloseTo(figureUnit(before), 4);
        expect(dist3(after.joints.hip, before.joints.hip)).toBeLessThan(1e-6);
        for (const key of ['elbowL', 'wristL', 'elbowR', 'wristR', 'kneeL', 'ankleL', 'kneeR', 'ankleR', 'neck', 'head'] as const) {
            expect(dot3(directionOf(after, key), directionOf(before, key)), key).toBeGreaterThan(0.999);
        }
        expect(span(after, 'shoulderL', 'shoulderR')).toBeGreaterThan(span(before, 'shoulderL', 'shoulderR'));
        expect(span(after, 'hipL', 'hipR')).toBeLessThan(span(before, 'hipL', 'hipR'));
    });

    it('round-trips back to the same figure', () => {
        const figure = createFigure('sideSit', DIMS, undefined, 0, undefined, 'female');
        const back = setFigureBuild(setFigureBuild(figure, 'male'), 'female');
        for (const key of JOINT_KEYS) expect(dist3(back.joints[key], figure.joints[key]), key).toBeLessThan(figureUnit(figure) * 0.002);
    });

    it('is a no-op on the build a figure already is', () => {
        const figure = createFigure('standing', DIMS, undefined, 0, undefined, 'male');
        expect(setFigureBuild(figure, 'male')).toBe(figure);
    });

    it('survives a preset swap, a reopen and a photograph import', () => {
        const figure = createFigure('standing', DIMS, undefined, 2, undefined, 'male');
        expect(figureBuild(applyPreset(figure, 'seiza', DIMS))).toBe('male');
        expect(figureBuild(transformFigures([figure], { scale: 0.5, dx: 3, dy: 4 })[0])).toBe('male');
        const imported = figureFromLandmarks(landmarksFromFigure(createFigure('running', DIMS), { width: 1, height: 1 }), {
            frame: { width: 1, height: 1 }, reference: figure,
        });
        expect(imported && figureBuild(imported.figure)).toBe('male');
    });

    it('opens a sketch saved before builds existed as the build its skeleton is', () => {
        const saved = createFigure('tPose', DIMS, undefined, 0, VIEW_PRESETS.front, LEGACY_BUILD);
        const { build: _drop, ...legacy } = saved;
        expect(figureBuild(legacy)).toBe(LEGACY_BUILD);
        // ...and nothing about its skeleton is out of step with that build.
        expect(span(legacy, 'shoulderL', 'shoulderR')).toBeCloseTo(span(saved, 'shoulderL', 'shoulderR'), 6);
    });
});
