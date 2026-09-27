// Each figure carries its own perspective camera, aimed at the middle of its
// torso. Everything outward-facing — grabbing, fitting, drawing — is measured
// on the projected figure, and the 3D renderer builds the very same camera.
import { JOINT_KEYS, TORSO_HEIGHT_RATIO, figureUnit, type FigureTurn, type JointKey, type PoseFigure } from './skeleton';
import { rad, zOf, type Vec3 } from './vec3';
import type { Point } from './types';

// Yaw turns the body about its own vertical axis (the canvas y axis, which
// points down); pitch tips it about the horizontal screen axis, which is what
// "seen from above / below" means. Always composed in this fixed order from
// the totals, never accumulated onto the joints — alternating incremental
// rotations creep roll into the figure and a manikin with roll looks broken.
export const rotateView = (point: Vec3, turn: FigureTurn): Vec3 => {
    const yaw = rad(turn.yaw);
    const pitch = rad(turn.pitch);
    const cy = Math.cos(yaw);
    const sy = Math.sin(yaw);
    const afterYaw = { x: point.x * cy + zOf(point) * sy, y: point.y, z: zOf(point) * cy - point.x * sy };
    const cp = Math.cos(pitch);
    const sp = Math.sin(pitch);
    return { x: afterYaw.x, y: afterYaw.y * cp - afterYaw.z * sp, z: afterYaw.y * sp + afterYaw.z * cp };
};


// How far the eye sits from the figure, in figure units (body heights). This
// is the lens: perspective comes from distance, not from focal length, and the
// projection keeps the torso's depth at scale 1 whatever the distance — so
// moving the camera in is a dolly-zoom. The figure stays the size it was on the
// canvas and only the perspective changes: near limbs balloon and far ones
// shrink at a wide angle, everything flattens toward a silhouette at a long
// one. That is exactly the choice a photographer is making with a lens, and
// the part of it that matters for a pose reference.
//
// The default is the distance every figure was drawn at before lenses
// existed: low enough that a limb reaching toward the camera visibly grows,
// high enough that a mannequin never goes fish-eye.
export const DEFAULT_LENS_DISTANCE = 3.4;

export type LensKey = 'ultraWide' | 'wide' | 'standard' | 'tele' | 'flat';

export interface Lens { key: LensKey; distance: number }

// Ordered near → far. The near end stops where `MAX_NEAR_RATIO` would start
// clamping a hand held out at arm's length; the far end is as good as
// orthographic — past it nothing changes that anyone could see.
export const LENSES: readonly Lens[] = [
    { key: 'ultraWide', distance: 1.3 },
    { key: 'wide', distance: 2.1 },
    { key: 'standard', distance: DEFAULT_LENS_DISTANCE },
    { key: 'tele', distance: 6 },
    { key: 'flat', distance: 14 },
];

export const MIN_LENS_DISTANCE = LENSES[0].distance;
export const MAX_LENS_DISTANCE = LENSES[LENSES.length - 1].distance;

export const figureLens = (figure: PoseFigure): number => figure.lens ?? DEFAULT_LENS_DISTANCE;

// Which named lens a figure has, or null for a distance set some other way.
export const lensOf = (figure: PoseFigure): LensKey | null =>
    LENSES.find((lens) => Math.abs(lens.distance - figureLens(figure)) < 1e-6)?.key ?? null;

// A joint dragged almost into the lens would project to infinity. Clamped well
// before that: the figure stays on the canvas whatever the pose.
const MAX_NEAR_RATIO = 0.55;

export interface Projection { anchor: Vec3; distance: number }

// Everything is projected about the figure's own centre, so a figure carries
// its camera with it: moving one across the canvas must not re-render it as if
// it were now off to the side of a shared lens, and two figures side by side
// are two references, not one photograph.
//
// The camera is aimed at the middle of the torso, deliberately not at the
// centre of the joint cloud. The joint cloud moves whenever any limb moves, so
// aiming at it would re-project — and visibly nudge — the entire figure every
// time a wrist was dragged. Neck and hip are the two joints no limb can shift.
export const projectionOf = (figure: PoseFigure): Projection => {
    const neck = figure.joints.neck;
    const hip = figure.joints.hip;
    return {
        anchor: {
            x: (neck.x + hip.x) / 2,
            y: (neck.y + hip.y) / 2,
            z: (zOf(neck) + zOf(hip)) / 2,
        },
        distance: figureUnit(figure) * figureLens(figure),
    };
};

// How much nearer things grow. 1 at the figure's own depth.
export const perspectiveAt = (z: number, projection: Projection): number => {
    const near = Math.min(z - projection.anchor.z, projection.distance * MAX_NEAR_RATIO);
    return projection.distance / (projection.distance - near);
};

export interface ProjectedPoint { x: number; y: number; depth: number; scale: number }

export const projectPoint = (point: Vec3, projection: Projection): ProjectedPoint => {
    const scale = perspectiveAt(zOf(point), projection);
    return {
        x: projection.anchor.x + (point.x - projection.anchor.x) * scale,
        y: projection.anchor.y + (point.y - projection.anchor.y) * scale,
        depth: zOf(point) - projection.anchor.z,
        scale,
    };
};

// The inverse, at a known depth: where on the world plane through `z` does this
// pointer sit? Dragging happens in screen pixels and has to land in the world.
export const unprojectPoint = (point: Point, z: number, projection: Projection): Vec3 => {
    const scale = perspectiveAt(z, projection);
    return {
        x: projection.anchor.x + (point.x - projection.anchor.x) / scale,
        y: projection.anchor.y + (point.y - projection.anchor.y) / scale,
        z,
    };
};

export type ProjectedJoints = Record<JointKey, ProjectedPoint>;

export const projectFigure = (figure: PoseFigure): ProjectedJoints => {
    const projection = projectionOf(figure);
    const out = {} as ProjectedJoints;
    for (const key of JOINT_KEYS) out[key] = projectPoint(figure.joints[key], projection);
    return out;
};
