// A posable mannequin that lives on the sketch canvas.
//
// Our image path is OpenAI-compatible generations/edits (plus Codex's native
// images protocol); none of it accepts pose conditioning, so a figure can only
// ever be *pixels in a reference image*. That is exactly what a sketch already
// is, which is why this is a tool inside the sketch canvas rather than a fourth
// reference-image source: the figure is a way to start a drawing, not a new
// kind of input.
//
// It is drawn as a solid grey mannequin rather than a stick figure on purpose:
// a wooden artist's doll reads as "a body in this pose", while thin line art
// reads as line art the model may faithfully reproduce in the result.
//
// Geometry here is pure and unit-tested; the canvas calls live at the bottom.

import type { CanvasDimensions, CanvasPoint } from './sketchCanvas';

export type JointKey =
    | 'head' | 'neck'
    | 'shoulderL' | 'shoulderR'
    | 'elbowL' | 'elbowR'
    | 'wristL' | 'wristR'
    | 'hip' | 'hipL' | 'hipR'
    | 'kneeL' | 'kneeR'
    | 'ankleL' | 'ankleR';

export const JOINT_KEYS: readonly JointKey[] = [
    'head', 'neck', 'shoulderL', 'shoulderR', 'elbowL', 'elbowR', 'wristL', 'wristR',
    'hip', 'hipL', 'hipR', 'kneeL', 'kneeR', 'ankleL', 'ankleR',
];

export interface PoseFigure {
    id: string;
    joints: Record<JointKey, CanvasPoint>;
}

export interface Rect { x: number; y: number; width: number; height: number }

// Bones are drawn as round-capped strokes; `width` is a fraction of the
// figure's current bounding-box height, so a figure keeps its proportions at
// any scale without storing one.
interface Bone { from: JointKey; to: JointKey; width: number }

export const BONES: readonly Bone[] = [
    { from: 'neck', to: 'hip', width: 0.105 },
    { from: 'shoulderL', to: 'shoulderR', width: 0.075 },
    { from: 'hipL', to: 'hipR', width: 0.075 },
    { from: 'neck', to: 'shoulderL', width: 0.06 },
    { from: 'neck', to: 'shoulderR', width: 0.06 },
    { from: 'hip', to: 'hipL', width: 0.06 },
    { from: 'hip', to: 'hipR', width: 0.06 },
    { from: 'shoulderL', to: 'elbowL', width: 0.045 },
    { from: 'shoulderR', to: 'elbowR', width: 0.045 },
    { from: 'elbowL', to: 'wristL', width: 0.036 },
    { from: 'elbowR', to: 'wristR', width: 0.036 },
    { from: 'hipL', to: 'kneeL', width: 0.062 },
    { from: 'hipR', to: 'kneeR', width: 0.062 },
    { from: 'kneeL', to: 'ankleL', width: 0.048 },
    { from: 'kneeR', to: 'ankleR', width: 0.048 },
    { from: 'neck', to: 'head', width: 0.04 },
];

export const HEAD_RADIUS_RATIO = 0.062;

// Presets are normalised into a unit box (x across the figure's width, y from
// crown to ankles). They are starting points, not a pose picker standing
// between the user and the canvas: picking the tool drops the default figure
// straight onto the surface and the preset row only appears once one is
// selected, to swap a pose in place.
export type PosePresetKey = 'standing' | 'walking' | 'sitting' | 'armsUp';

type PresetPoints = Record<JointKey, readonly [number, number]>;

export const POSE_PRESETS: Record<PosePresetKey, PresetPoints> = {
    standing: {
        head: [0.50, 0.055], neck: [0.50, 0.145],
        shoulderL: [0.34, 0.185], shoulderR: [0.66, 0.185],
        elbowL: [0.28, 0.320], elbowR: [0.72, 0.320],
        wristL: [0.24, 0.455], wristR: [0.76, 0.455],
        hip: [0.50, 0.505], hipL: [0.40, 0.525], hipR: [0.60, 0.525],
        kneeL: [0.40, 0.735], kneeR: [0.60, 0.735],
        ankleL: [0.40, 0.960], ankleR: [0.60, 0.960],
    },
    walking: {
        head: [0.50, 0.055], neck: [0.50, 0.145],
        shoulderL: [0.34, 0.185], shoulderR: [0.66, 0.185],
        elbowL: [0.30, 0.325], elbowR: [0.74, 0.310],
        wristL: [0.38, 0.440], wristR: [0.80, 0.435],
        hip: [0.50, 0.505], hipL: [0.42, 0.525], hipR: [0.58, 0.525],
        kneeL: [0.30, 0.700], kneeR: [0.68, 0.720],
        ankleL: [0.22, 0.935], ankleR: [0.80, 0.955],
    },
    sitting: {
        head: [0.34, 0.070], neck: [0.36, 0.160],
        shoulderL: [0.28, 0.200], shoulderR: [0.46, 0.200],
        elbowL: [0.28, 0.340], elbowR: [0.48, 0.340],
        wristL: [0.42, 0.450], wristR: [0.60, 0.450],
        hip: [0.40, 0.565], hipL: [0.34, 0.585], hipR: [0.48, 0.585],
        kneeL: [0.78, 0.605], kneeR: [0.86, 0.630],
        ankleL: [0.76, 0.930], ankleR: [0.86, 0.955],
    },
    armsUp: {
        head: [0.50, 0.100], neck: [0.50, 0.190],
        shoulderL: [0.34, 0.230], shoulderR: [0.66, 0.230],
        elbowL: [0.26, 0.110], elbowR: [0.74, 0.110],
        wristL: [0.22, 0.010], wristR: [0.78, 0.010],
        hip: [0.50, 0.545], hipL: [0.40, 0.565], hipR: [0.60, 0.565],
        kneeL: [0.40, 0.760], kneeR: [0.60, 0.760],
        ankleL: [0.40, 0.965], ankleR: [0.60, 0.965],
    },
};

// A figure lands at 70% of the canvas height, centred. Big enough to read as
// the subject, small enough to leave room for the scene around it.
export const FIGURE_HEIGHT_RATIO = 0.7;
// Width of the unit box relative to its height. Arms out to the side need
// more room than a body is wide.
export const FIGURE_ASPECT = 0.45;

let figureCounter = 0;

export const createFigure = (
    preset: PosePresetKey,
    dims: CanvasDimensions,
    center?: CanvasPoint,
): PoseFigure => {
    const height = Math.min(dims.height * FIGURE_HEIGHT_RATIO, dims.width / FIGURE_ASPECT);
    const width = height * FIGURE_ASPECT;
    const cx = center?.x ?? dims.width / 2;
    const cy = center?.y ?? dims.height / 2;
    const points = POSE_PRESETS[preset];
    const joints = {} as Record<JointKey, CanvasPoint>;
    for (const key of JOINT_KEYS) {
        const [nx, ny] = points[key];
        joints[key] = { x: cx + (nx - 0.5) * width, y: cy + (ny - 0.5) * height };
    }
    figureCounter += 1;
    const figure = { id: `figure-${Date.now()}-${figureCounter}`, joints };
    // Centre on the joints, not on the nominal unit box: no preset fills the
    // box exactly (a crown sits below its top edge, a seated figure leans to
    // one side), and every later transform pivots on the joint bounds. Making
    // the two agree here is what lets a pose be swapped in place without the
    // figure drifting.
    const bounds = figureBounds(figure);
    return translateFigure(figure, cx - (bounds.x + bounds.width / 2), cy - (bounds.y + bounds.height / 2));
};

// Swaps the pose while keeping the figure where it is and roughly how big it
// is: re-entry (principle 10) applies inside the dialog too.
export const applyPreset = (figure: PoseFigure, preset: PosePresetKey, dims: CanvasDimensions): PoseFigure => {
    const bounds = figureBounds(figure);
    const fresh = createFigure(preset, dims, {
        x: bounds.x + bounds.width / 2,
        y: bounds.y + bounds.height / 2,
    });
    const freshBounds = figureBounds(fresh);
    const factor = freshBounds.height > 0 ? bounds.height / freshBounds.height : 1;
    return { ...scaleFigure(fresh, factor), id: figure.id };
};

export const figureBounds = (figure: PoseFigure): Rect => {
    const points = JOINT_KEYS.map((key) => figure.joints[key]);
    const xs = points.map((p) => p.x);
    const ys = points.map((p) => p.y);
    const minX = Math.min(...xs);
    const minY = Math.min(...ys);
    return { x: minX, y: minY, width: Math.max(...xs) - minX, height: Math.max(...ys) - minY };
};

// The head sticks out past the crown joint and every bone is a thick capsule,
// so the visual box is fatter than the joint box. Used for hit-testing the
// body and for placing the scale handle.
export const figurePadding = (figure: PoseFigure): number =>
    Math.max(figureBounds(figure).height * HEAD_RADIUS_RATIO, 1);

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

const mapJoints = (figure: PoseFigure, fn: (point: CanvasPoint) => CanvasPoint): PoseFigure => {
    const joints = {} as Record<JointKey, CanvasPoint>;
    for (const key of JOINT_KEYS) joints[key] = fn(figure.joints[key]);
    return { ...figure, joints };
};

export const translateFigure = (figure: PoseFigure, dx: number, dy: number): PoseFigure =>
    mapJoints(figure, (p) => ({ x: p.x + dx, y: p.y + dy }));

export const moveJoint = (figure: PoseFigure, key: JointKey, point: CanvasPoint): PoseFigure => ({
    ...figure,
    joints: { ...figure.joints, [key]: point },
});

export const MIN_FIGURE_HEIGHT = 24;

// Uniform scale about the figure's own centre, clamped so a figure can never
// be shrunk into an invisible dot the user then cannot grab.
export const scaleFigure = (figure: PoseFigure, factor: number, origin?: CanvasPoint): PoseFigure => {
    const bounds = figureBounds(figure);
    const pivot = origin ?? { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
    const safe = Number.isFinite(factor) && factor > 0 ? factor : 1;
    const clamped = bounds.height * safe < MIN_FIGURE_HEIGHT && safe < 1
        ? Math.max(MIN_FIGURE_HEIGHT / Math.max(bounds.height, 1), 1e-3)
        : safe;
    return mapJoints(figure, (p) => ({
        x: pivot.x + (p.x - pivot.x) * clamped,
        y: pivot.y + (p.y - pivot.y) * clamped,
    }));
};

// Mirroring the coordinates is enough: the bone list is symmetric, so no
// left/right relabelling is needed for the figure to render correctly.
export const flipFigure = (figure: PoseFigure): PoseFigure => {
    const bounds = figureBounds(figure);
    const axis = bounds.x + bounds.width / 2;
    return mapJoints(figure, (p) => ({ x: axis * 2 - p.x, y: p.y }));
};

export const distanceToSegment = (point: CanvasPoint, a: CanvasPoint, b: CanvasPoint): number => {
    const vx = b.x - a.x;
    const vy = b.y - a.y;
    const lengthSq = vx * vx + vy * vy;
    if (lengthSq === 0) return Math.hypot(point.x - a.x, point.y - a.y);
    const t = Math.max(0, Math.min(1, ((point.x - a.x) * vx + (point.y - a.y) * vy) / lengthSq));
    return Math.hypot(point.x - (a.x + vx * t), point.y - (a.y + vy * t));
};

export const hitTestJoint = (figure: PoseFigure, point: CanvasPoint, radius: number): JointKey | null => {
    let best: JointKey | null = null;
    let bestDistance = radius;
    for (const key of JOINT_KEYS) {
        const joint = figure.joints[key];
        const distance = Math.hypot(point.x - joint.x, point.y - joint.y);
        if (distance <= bestDistance) {
            best = key;
            bestDistance = distance;
        }
    }
    return best;
};

// True when the point is on the mannequin's silhouette (any bone capsule or
// the head), which is what "grab the body and move it" means.
export const hitTestBody = (figure: PoseFigure, point: CanvasPoint, tolerance = 0): boolean => {
    const height = figureBounds(figure).height;
    const head = figure.joints.head;
    if (Math.hypot(point.x - head.x, point.y - head.y) <= height * HEAD_RADIUS_RATIO + tolerance) return true;
    return BONES.some((bone) => distanceToSegment(point, figure.joints[bone.from], figure.joints[bone.to])
        <= (height * bone.width) / 2 + tolerance);
};

// Bottom-right of the visual box: the familiar corner grip, so scaling does
// not need a slider in the toolbar.
export const scaleHandlePoint = (figure: PoseFigure): CanvasPoint => {
    const bounds = figureVisualBounds(figure);
    return { x: bounds.x + bounds.width, y: bounds.y + bounds.height };
};

export const isScaleHandleHit = (figure: PoseFigure, point: CanvasPoint, radius: number): boolean => {
    const handle = scaleHandlePoint(figure);
    return Math.hypot(point.x - handle.x, point.y - handle.y) <= radius;
};

// --- canvas rendering --------------------------------------------------------

export const FIGURE_FILL = '#9ca3af';
export const FIGURE_SELECTED_FILL = '#8b97a8';
const HANDLE_FILL = '#2563eb';
const HANDLE_STROKE = '#ffffff';

export const drawFigure = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    options: { selected?: boolean } = {},
): void => {
    const height = figureBounds(figure).height;
    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.globalCompositeOperation = 'source-over';
    ctx.strokeStyle = options.selected ? FIGURE_SELECTED_FILL : FIGURE_FILL;
    for (const bone of BONES) {
        const from = figure.joints[bone.from];
        const to = figure.joints[bone.to];
        ctx.lineWidth = Math.max(1, height * bone.width);
        ctx.beginPath();
        ctx.moveTo(from.x, from.y);
        ctx.lineTo(to.x, to.y);
        ctx.stroke();
    }
    ctx.fillStyle = options.selected ? FIGURE_SELECTED_FILL : FIGURE_FILL;
    ctx.beginPath();
    ctx.arc(figure.joints.head.x, figure.joints.head.y, Math.max(1, height * HEAD_RADIUS_RATIO), 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
};

// Handles are overlay-only: they are drawn on the interaction layer, never on
// the surface that is exported, so the model never sees the blue dots.
export const drawFigureHandles = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    handleRadius: number,
): void => {
    ctx.save();
    ctx.globalCompositeOperation = 'source-over';
    ctx.lineWidth = Math.max(1, handleRadius * 0.35);
    for (const key of JOINT_KEYS) {
        const joint = figure.joints[key];
        ctx.fillStyle = HANDLE_FILL;
        ctx.strokeStyle = HANDLE_STROKE;
        ctx.beginPath();
        ctx.arc(joint.x, joint.y, handleRadius, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
    }
    const handle = scaleHandlePoint(figure);
    ctx.fillStyle = HANDLE_STROKE;
    ctx.strokeStyle = HANDLE_FILL;
    ctx.beginPath();
    ctx.rect(handle.x - handleRadius, handle.y - handleRadius, handleRadius * 2, handleRadius * 2);
    ctx.fill();
    ctx.stroke();
    ctx.restore();
};
