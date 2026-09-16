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


// How far the eye sits from the figure, in figure units. Low enough that a
// limb reaching toward the camera visibly grows and a turned torso visibly
// foreshortens; high enough that a mannequin never goes fish-eye. Perspective
// is the whole point: an orthographic projection of a 3D pose still reads flat.
const VIEW_DISTANCE_RATIO = 3.4;
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
        distance: figureUnit(figure) * VIEW_DISTANCE_RATIO,
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
