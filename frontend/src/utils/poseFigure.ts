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

// The hit-testing skeleton. The manikin is drawn from `figureParts` further
// down; these segments are the coarse silhouette used to answer "did the
// pointer land on the body?", with `width` a fraction of the figure's current
// bounding-box height so proportions survive scaling.
interface Bone { from: JointKey; to: JointKey; width: number }

export const BONES: readonly Bone[] = [
    { from: 'neck', to: 'hip', width: 0.105 },
    { from: 'shoulderL', to: 'shoulderR', width: 0.080 },
    { from: 'hipL', to: 'hipR', width: 0.082 },
    { from: 'neck', to: 'shoulderL', width: 0.062 },
    { from: 'neck', to: 'shoulderR', width: 0.062 },
    { from: 'hip', to: 'hipL', width: 0.070 },
    { from: 'hip', to: 'hipR', width: 0.070 },
    { from: 'shoulderL', to: 'elbowL', width: 0.062 },
    { from: 'shoulderR', to: 'elbowR', width: 0.062 },
    { from: 'elbowL', to: 'wristL', width: 0.046 },
    { from: 'elbowR', to: 'wristR', width: 0.046 },
    { from: 'hipL', to: 'kneeL', width: 0.086 },
    { from: 'hipR', to: 'kneeR', width: 0.086 },
    { from: 'kneeL', to: 'ankleL', width: 0.062 },
    { from: 'kneeR', to: 'ankleR', width: 0.062 },
    { from: 'neck', to: 'head', width: 0.042 },
];

// The head's long radius: also the padding that keeps the visual box (and
// the scale grip on its corner) clear of the silhouette.
export const HEAD_RADIUS_RATIO = 0.07;

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

// --- the wooden manikin ------------------------------------------------------
//
// Everything below is derived from the fifteen joints; there are no extra
// handles. The shape follows an artist's wooden manikin rather than a flat
// pictogram: a peg neck under an egg head, a chest and a pelvis as two
// separate volumes joined at the waist, visible ball joints, and tapered limb
// segments. That is not decoration — the chest takes its angle from the
// shoulder line and the pelvis from the hip line, so dragging one shoulder
// twists the torso and the figure reads as having a front and a back, which a
// row of uniform capsules never does.

export interface Ellipse { center: CanvasPoint; radiusX: number; radiusY: number; angle: number }
export interface Segment { from: CanvasPoint; to: CanvasPoint; fromRadius: number; toRadius: number }
export interface Ball { center: CanvasPoint; radius: number }

export interface FigureParts {
    head: Ellipse;
    neck: Segment;
    spine: Segment;
    chest: Ellipse;
    waist: Ball;
    pelvis: Ellipse;
    limbs: Segment[];
    balls: Ball[];
    hipBalls: Ball[];
    hands: Ellipse[];
    feet: Ellipse[];
}

const midpoint = (a: CanvasPoint, b: CanvasPoint): CanvasPoint => ({ x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 });
const lerp = (a: CanvasPoint, b: CanvasPoint, t: number): CanvasPoint => ({
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
});
const angleOf = (a: CanvasPoint, b: CanvasPoint): number => Math.atan2(b.y - a.y, b.x - a.x);
const spanOf = (a: CanvasPoint, b: CanvasPoint): number => Math.hypot(b.x - a.x, b.y - a.y);

// Radii as fractions of the figure's height, so proportions survive scaling.
const R = {
    headLong: 0.070, headWide: 0.056,
    neck: 0.021,
    shoulder: 0.034, elbow: 0.026, wrist: 0.019,
    hipBall: 0.036, knee: 0.033, ankle: 0.023,
    upperArm: 0.031, foreArm: 0.023, thigh: 0.043, shin: 0.031,
    handLong: 0.034, handWide: 0.024,
    footLong: 0.033, footWide: 0.020,
} as const;

export const figureParts = (figure: PoseFigure): FigureParts => {
    const joints = figure.joints;
    const height = Math.max(figureBounds(figure).height, 1);
    const u = (ratio: number) => height * ratio;

    const shoulderMid = midpoint(joints.shoulderL, joints.shoulderR);
    const hipMid = midpoint(joints.hipL, joints.hipR);
    const shoulderSpan = spanOf(joints.shoulderL, joints.shoulderR);
    const hipSpan = spanOf(joints.hipL, joints.hipR);
    const torso = Math.max(spanOf(joints.neck, hipMid), 1);

    // Chest and pelvis each take their own angle: the chest from the shoulder
    // line, the pelvis from the hip line. Twist one and only that volume turns.
    // Anchored under the shoulders, not on the neck-hip line: the chest turns
    // with the shoulder line, so its centre has to travel with them or a big
    // shoulder drag slides the ribcage out from under the neck.
    const chestCenter = lerp(shoulderMid, hipMid, 0.26);
    // The pelvis straddles the hip line and the waist ball bridges it to the
    // chest, the way the two turned halves of a manikin meet at their pin.
    const pelvisCenter = lerp(joints.neck, hipMid, 0.96);
    const waistCenter = lerp(joints.neck, hipMid, 0.72);

    const limbs: Segment[] = [
        { from: joints.shoulderL, to: joints.elbowL, fromRadius: u(R.upperArm), toRadius: u(R.elbow) },
        { from: joints.shoulderR, to: joints.elbowR, fromRadius: u(R.upperArm), toRadius: u(R.elbow) },
        { from: joints.elbowL, to: joints.wristL, fromRadius: u(R.foreArm), toRadius: u(R.wrist) },
        { from: joints.elbowR, to: joints.wristR, fromRadius: u(R.foreArm), toRadius: u(R.wrist) },
        { from: joints.hipL, to: joints.kneeL, fromRadius: u(R.thigh), toRadius: u(R.knee) },
        { from: joints.hipR, to: joints.kneeR, fromRadius: u(R.thigh), toRadius: u(R.knee) },
        { from: joints.kneeL, to: joints.ankleL, fromRadius: u(R.shin), toRadius: u(R.ankle) },
        { from: joints.kneeR, to: joints.ankleR, fromRadius: u(R.shin), toRadius: u(R.ankle) },
    ];

    const balls: Ball[] = [
        { center: joints.shoulderL, radius: u(R.shoulder) },
        { center: joints.shoulderR, radius: u(R.shoulder) },
        { center: joints.elbowL, radius: u(R.elbow) },
        { center: joints.elbowR, radius: u(R.elbow) },
        { center: joints.kneeL, radius: u(R.knee) },
        { center: joints.kneeR, radius: u(R.knee) },
        { center: joints.ankleL, radius: u(R.ankle) },
        { center: joints.ankleR, radius: u(R.ankle) },
    ];

    // A manikin's mitten hand continues the forearm; its block foot sits
    // across the shin, so a bent leg carries its foot around with it.
    const hands: Ellipse[] = ([['elbowL', 'wristL'], ['elbowR', 'wristR']] as const).map(([from, to]) => {
        const direction = angleOf(joints[from], joints[to]);
        return {
            center: {
                x: joints[to].x + Math.cos(direction) * u(R.handLong) * 0.55,
                y: joints[to].y + Math.sin(direction) * u(R.handLong) * 0.55,
            },
            radiusX: u(R.handLong),
            radiusY: u(R.handWide),
            angle: direction,
        };
    });

    const feet: Ellipse[] = ([['kneeL', 'ankleL'], ['kneeR', 'ankleR']] as const).map(([from, to]) => {
        const direction = angleOf(joints[from], joints[to]);
        return {
            center: {
                x: joints[to].x + Math.cos(direction) * u(R.footWide) * 0.9,
                y: joints[to].y + Math.sin(direction) * u(R.footWide) * 0.9,
            },
            radiusX: u(R.footLong),
            radiusY: u(R.footWide),
            angle: direction + Math.PI / 2,
        };
    });

    return {
        // Drawn under the chest so the figure never comes apart: the chest and
        // pelvis rotate on their own axes, and a hard shoulder drag would
        // otherwise swing the ribcage out from under the neck and leave a gap.
        spine: {
            from: joints.neck,
            to: hipMid,
            fromRadius: u(0.045),
            toRadius: u(0.055),
        },
        head: {
            center: joints.head,
            radiusX: u(R.headWide),
            radiusY: u(R.headLong),
            angle: angleOf(joints.neck, joints.head) - Math.PI / 2,
        },
        neck: {
            from: joints.neck,
            to: joints.head,
            fromRadius: u(R.neck),
            toRadius: u(R.neck),
        },
        chest: {
            center: chestCenter,
            // Transverse axis on the shoulder line, long axis down the torso:
            // drag one shoulder and the chest rotates with it, while the
            // pelvis keeps the hip line's own angle. That difference is the
            // twist, and it is the whole reason the torso is two volumes.
            radiusX: Math.max(shoulderSpan * 0.46, u(0.06)),
            radiusY: torso * 0.33,
            angle: angleOf(joints.shoulderL, joints.shoulderR),
        },
        waist: { center: waistCenter, radius: torso * 0.085 },
        pelvis: {
            // Wide enough to reach past the hip balls and short enough to sit
            // between them: a narrower or taller ellipse hangs below the hips
            // as a droplet instead of reading as a pelvis.
            center: pelvisCenter,
            radiusX: Math.max(hipSpan * 0.5 + u(R.hipBall) * 0.9, u(0.06)),
            radiusY: torso * 0.16,
            angle: angleOf(joints.hipL, joints.hipR),
        },
        limbs,
        balls,
        // Kept apart from the rest because they are drawn *under* the pelvis:
        // a hip ball on top of the block reads as a buttock, while one behind
        // it shows only where the thigh comes out, which is what the wooden
        // joint actually looks like.
        hipBalls: [
            { center: joints.hipL, radius: u(R.hipBall) },
            { center: joints.hipR, radius: u(R.hipBall) },
        ],
        hands,
        feet,
    };
};

// Three tones, no gradients: the body, the joint balls a shade darker so the
// articulation reads, and the head a shade lighter so it does not merge into
// the chest. Neutral grey, not wood — the manikin's structure is the message,
// and a wood colour only invites the model to paint a wooden doll.
export const FIGURE_FILL = '#aeb2b6';
export const FIGURE_JOINT_FILL = '#8f9398';
export const FIGURE_HEAD_FILL = '#b9bdc1';
export const FIGURE_SELECTED_FILL = '#a2abbd';
export const FIGURE_SELECTED_JOINT_FILL = '#828da3';
export const FIGURE_SELECTED_HEAD_FILL = '#adb5c5';
const HANDLE_FILL = '#2563eb';
const HANDLE_STROKE = '#ffffff';

const fillEllipse = (ctx: CanvasRenderingContext2D, ellipse: Ellipse): void => {
    ctx.beginPath();
    ctx.ellipse(ellipse.center.x, ellipse.center.y, Math.max(ellipse.radiusX, 0.5), Math.max(ellipse.radiusY, 0.5), ellipse.angle, 0, Math.PI * 2);
    ctx.fill();
};

// A tapered capsule: the quad between the two end circles plus the circles
// themselves. Close enough to a turned wooden limb, and it degrades to a
// plain capsule when the radii match.
const fillSegment = (ctx: CanvasRenderingContext2D, segment: Segment): void => {
    const dx = segment.to.x - segment.from.x;
    const dy = segment.to.y - segment.from.y;
    const length = Math.hypot(dx, dy);
    if (length > 1e-6) {
        const px = -dy / length;
        const py = dx / length;
        ctx.beginPath();
        ctx.moveTo(segment.from.x + px * segment.fromRadius, segment.from.y + py * segment.fromRadius);
        ctx.lineTo(segment.to.x + px * segment.toRadius, segment.to.y + py * segment.toRadius);
        ctx.lineTo(segment.to.x - px * segment.toRadius, segment.to.y - py * segment.toRadius);
        ctx.lineTo(segment.from.x - px * segment.fromRadius, segment.from.y - py * segment.fromRadius);
        ctx.closePath();
        ctx.fill();
    }
    ctx.beginPath();
    ctx.arc(segment.from.x, segment.from.y, Math.max(segment.fromRadius, 0.5), 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(segment.to.x, segment.to.y, Math.max(segment.toRadius, 0.5), 0, Math.PI * 2);
    ctx.fill();
};

export const drawFigure = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    options: { selected?: boolean } = {},
): void => {
    const parts = figureParts(figure);
    const selected = options.selected === true;
    ctx.save();
    ctx.globalCompositeOperation = 'source-over';

    // Body first, joints over it, head last: the drawing order is what makes
    // the balls read as articulation rather than as lumps under the limbs.
    ctx.fillStyle = selected ? FIGURE_SELECTED_FILL : FIGURE_FILL;
    fillSegment(ctx, parts.neck);
    fillSegment(ctx, parts.spine);
    for (const limb of parts.limbs) fillSegment(ctx, limb);
    for (const hand of parts.hands) fillEllipse(ctx, hand);
    for (const foot of parts.feet) fillEllipse(ctx, foot);

    const jointFill = selected ? FIGURE_SELECTED_JOINT_FILL : FIGURE_JOINT_FILL;
    const bodyFill = selected ? FIGURE_SELECTED_FILL : FIGURE_FILL;
    const fillBall = (ball: Ball) => {
        ctx.beginPath();
        ctx.arc(ball.center.x, ball.center.y, Math.max(ball.radius, 0.5), 0, Math.PI * 2);
        ctx.fill();
    };

    ctx.fillStyle = jointFill;
    for (const ball of parts.hipBalls) fillBall(ball);

    ctx.fillStyle = bodyFill;
    fillEllipse(ctx, parts.chest);
    fillEllipse(ctx, parts.pelvis);

    ctx.fillStyle = jointFill;
    for (const ball of [parts.waist, ...parts.balls]) fillBall(ball);

    ctx.fillStyle = selected ? FIGURE_SELECTED_HEAD_FILL : FIGURE_HEAD_FILL;
    fillEllipse(ctx, parts.head);
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
