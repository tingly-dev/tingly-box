import { projectFigure } from './camera';
import { HEAD_RADIUS_RATIO, JOINT_KEYS, figureUnit, type JointKey, type PoseFigure, type Rect } from './skeleton';
import { zOf, type Vec3 } from './vec3';
import type { Point, Size } from './types';

// --- bounds and transforms ---------------------------------------------------
//
// Everything outward-facing is measured on the *projected* figure: what can be
// grabbed, fitted and framed has to match what is on screen, not the world
// coordinates behind it.

export const figureBounds = (figure: PoseFigure): Rect => {
    const projected = projectFigure(figure);
    const xs = JOINT_KEYS.map((key) => projected[key].x);
    const ys = JOINT_KEYS.map((key) => projected[key].y);
    const minX = Math.min(...xs);
    const minY = Math.min(...ys);
    return { x: minX, y: minY, width: Math.max(...xs) - minX, height: Math.max(...ys) - minY };
};

// The pivot every transform in this module turns about: the centre of the
// projected joint bounds. Stated once, because `createFigure` depends on it
// agreeing with how presets are normalised.
export const figureCenter = (figure: PoseFigure): Point => {
    const bounds = figureBounds(figure);
    return { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
};

export const centerFigureAt = (figure: PoseFigure, point: Point): PoseFigure => {
    const center = figureCenter(figure);
    return translateFigure(figure, point.x - center.x, point.y - center.y);
};

const figurePadding = (figure: PoseFigure): number =>
    Math.max(figureUnit(figure) * HEAD_RADIUS_RATIO, 1);

// The head sticks out past the crown joint and every bone is a thick capsule,
// so the visual box is fatter than the joint box. Used for hit-testing the
// body and for placing the grips.
export const figureVisualBounds = (figure: PoseFigure): Rect => {
    const bounds = figureBounds(figure);
    const pad = figurePadding(figure);
    return {
        x: bounds.x - pad,
        y: bounds.y - pad,
        width: bounds.width + pad * 2,
        height: bounds.height + pad * 2,
    };
};

// Scaled and centred to sit inside `box` with a margin. The thumbnail grid
// needs this; keeping it here means the "visual bounds, not joint bounds"
// choice is made once, in the module that knows the difference.
export const fitFigureInto = (figure: PoseFigure, box: Size, pad = 0): PoseFigure => {
    const bounds = figureVisualBounds(figure);
    const factor = Math.min(
        (box.width - pad * 2) / Math.max(bounds.width, 1),
        (box.height - pad * 2) / Math.max(bounds.height, 1),
    );
    const scaled = scaleFigure(figure, factor);
    const scaledBounds = figureVisualBounds(scaled);
    return translateFigure(
        scaled,
        box.width / 2 - (scaledBounds.x + scaledBounds.width / 2),
        box.height / 2 - (scaledBounds.y + scaledBounds.height / 2),
    );
};

export const mapJoints = (figure: PoseFigure, fn: (point: Vec3) => Vec3): PoseFigure => {
    const joints = {} as Record<JointKey, Vec3>;
    for (const key of JOINT_KEYS) joints[key] = fn(figure.joints[key]);
    return { ...figure, joints };
};

export const translateFigure = (figure: PoseFigure, dx: number, dy: number): PoseFigure =>
    mapJoints(figure, (p) => ({ x: p.x + dx, y: p.y + dy, z: zOf(p) }));

// In `figureUnit` space, like every other size in this module: a bounding box
// would mean a different physical minimum per pose, letting a lying figure be
// shrunk to a fraction of the size a standing one is held at.
export const MIN_FIGURE_UNIT = 32;

const safeFactor = (factor: number): number => (Number.isFinite(factor) && factor > 0 ? factor : 1);

// The floor belongs to the *drag*, not to the transform: a resize gesture must
// not leave a figure too small to grab again, but scaling a figure as part of
// applying a pose or refitting a canvas has no business being second-guessed.
// Never above 1 — clamping an already-tiny figure would turn "make it smaller"
// into "make it bigger".
export const clampScaleFactor = (figure: PoseFigure, factor: number): number => {
    const safe = safeFactor(factor);
    if (safe >= 1) return safe;
    return Math.max(safe, Math.min(1, MIN_FIGURE_UNIT / figureUnit(figure)));
};

// Uniform scale about the figure's own centre, depth included: scaling only
// x and y would flatten the figure as it shrank and stretch it as it grew.
export const scaleFigure = (figure: PoseFigure, factor: number, origin?: Point): PoseFigure => {
    const pivot = origin ?? figureCenter(figure);
    const factorApplied = safeFactor(factor);
    return mapJoints(figure, (p) => ({
        x: pivot.x + (p.x - pivot.x) * factorApplied,
        y: pivot.y + (p.y - pivot.y) * factorApplied,
        z: zOf(p) * factorApplied,
    }));
};
