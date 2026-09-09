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

import {
    applyTransform,
    isIdentityTransform,
    type CanvasDimensions,
    type CanvasPoint,
    type CanvasTransform,
} from './sketchCanvas';

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
    // Which tone this figure is drawn in. Assigned once, at creation, and
    // carried in the data: figures in a crowd have to stay told apart, and
    // colouring by list position would recolour everyone when one is deleted.
    // Optional so a sketch saved before shades existed still opens.
    shade?: number;
}

export interface Rect { x: number; y: number; width: number; height: number }

// The head's long radius: also the padding that keeps the visual box (and
// the scale grip on its corner) clear of the silhouette.
const HEAD_RADIUS_RATIO = 0.07;

// Every thickness in the manikin is a fraction of "how big is this person",
// and that number must NOT be the bounding box: a lying figure has the same
// body as a standing one but a fifth of the box, which would shrink its limbs
// into the stick figure this module exists to avoid. The torso bone is the
// one measure a pose cannot change (`swingJoint` rotates, never stretches),
// so scale is read from it and converted back to the standing height the
// ratios were authored against.
const TORSO_HEIGHT_RATIO = 0.36;

export const figureUnit = (figure: PoseFigure): number => {
    // hip → neck, the bone itself. The hip *line's* midpoint drifts with the
    // torso's lean (the stubs to hipL/hipR rotate with it), which would make
    // the same body measure differently lying down than standing up.
    const torso = Math.hypot(
        figure.joints.neck.x - figure.joints.hip.x,
        figure.joints.neck.y - figure.joints.hip.y,
    );
    return Math.max(torso / TORSO_HEIGHT_RATIO, 1);
};

// Presets are normalised into a unit box (x across the figure's width, y from
// crown to ankles). They are starting points, not a pose picker standing
// between the user and the canvas: picking the tool drops the default figure
// straight onto the surface and the preset row only appears once one is
// selected, to swap a pose in place.
// A figure lands at 70% of the canvas height, centred. Big enough to read as
// the subject, small enough to leave room for the scene around it.
const FIGURE_HEIGHT_RATIO = 0.7;
// Width of the unit box relative to its height. Arms out to the side need
// more room than a body is wide.
const FIGURE_ASPECT = 0.45;

export type PosePresetKey =
    | 'standing' | 'contrapposto' | 'handsOnHips' | 'armsCrossed' | 'tPose' | 'armsUp'
    | 'walking' | 'running' | 'jumping' | 'kicking' | 'reaching' | 'bowing'
    | 'sitting' | 'sittingFloor' | 'kneeling' | 'crouching' | 'lying'
    | 'wave' | 'pointing' | 'thinking' | 'leaning';

type PresetPoints = Record<JointKey, readonly [number, number]>;

// Poses are declared as bone angles, not as coordinates. Two reasons: a table
// of thirty joint positions is unreadable and unmaintainable, and — since a
// drag now rotates bones and never stretches them (see `swingJoint`) — every
// pose in the library has to be built from the same bone lengths or applying
// one would silently change the figure's proportions.
//
// Angles are degrees for the bone itself, not relative to its parent: 0 points
// straight down, +90 to the right of the screen, ±180 straight up. `lean` is
// the odd one out — it tilts the torso's top, so a negative lean leans the
// body toward the side the limbs are reaching. Reading
// "the upper arm is at -50" is something you can picture; "the elbow is at
// (0.28, 0.32)" is not.
interface PoseSpec {
    lean?: number;          // torso, from upright
    headTilt?: number;      // head, relative to the torso
    shoulderTilt?: number;  // shoulder line, from horizontal
    hipTilt?: number;
    arms: { l: readonly [number, number]; r: readonly [number, number] };
    legs: { l: readonly [number, number]; r: readonly [number, number] };
}

// One skeleton for every pose, in arbitrary units — the result is normalised.
const BONE = {
    torso: 0.36, head: 0.11,
    shoulderSpan: 0.085, shoulderDrop: 0.035,
    upperArm: 0.155, foreArm: 0.145,
    hipSpan: 0.052, hipDrop: 0.022,
    thigh: 0.235, shin: 0.225,
} as const;

const ORIGIN: CanvasPoint = { x: 0, y: 0 };

const rotatePoint = (point: CanvasPoint, pivot: CanvasPoint, angle: number): CanvasPoint => {
    const cos = Math.cos(angle);
    const sin = Math.sin(angle);
    const dx = point.x - pivot.x;
    const dy = point.y - pivot.y;
    return { x: pivot.x + dx * cos - dy * sin, y: pivot.y + dx * sin + dy * cos };
};

const rad = (degrees: number) => (degrees * Math.PI) / 180;
// Down is 0, so a spec reads the way a person describes a limb: "hanging" is 0.
const along = (degrees: number, length: number): CanvasPoint => ({
    x: Math.sin(rad(degrees)) * length,
    y: Math.cos(rad(degrees)) * length,
});
const plus = (point: CanvasPoint, delta: CanvasPoint): CanvasPoint => ({ x: point.x + delta.x, y: point.y + delta.y });
// Rotating one offset, rather than adding two angled vectors: the shoulder and
// hip stubs have to keep their length when the shoulder or hip line tilts, or
// the "same bones in every pose" guarantee quietly breaks for tilted poses.
const turned = (offset: CanvasPoint, degrees: number): CanvasPoint =>
    rotatePoint(offset, ORIGIN, rad(degrees));

const buildPose = (spec: PoseSpec): PresetPoints => {
    const lean = spec.lean ?? 0;
    const hip = { x: 0, y: 0 };
    const neck = plus(hip, along(lean + 180, BONE.torso));
    const head = plus(neck, along(lean + (spec.headTilt ?? 0) + 180, BONE.head));

    const shoulderAngle = lean + (spec.shoulderTilt ?? 0);
    const shoulderL = plus(neck, turned({ x: -BONE.shoulderSpan, y: BONE.shoulderDrop }, shoulderAngle));
    const shoulderR = plus(neck, turned({ x: BONE.shoulderSpan, y: BONE.shoulderDrop }, shoulderAngle));

    const hipAngle = lean + (spec.hipTilt ?? 0);
    const hipL = plus(hip, turned({ x: -BONE.hipSpan, y: BONE.hipDrop }, hipAngle));
    const hipR = plus(hip, turned({ x: BONE.hipSpan, y: BONE.hipDrop }, hipAngle));

    const elbowL = plus(shoulderL, along(spec.arms.l[0], BONE.upperArm));
    const elbowR = plus(shoulderR, along(spec.arms.r[0], BONE.upperArm));
    const kneeL = plus(hipL, along(spec.legs.l[0], BONE.thigh));
    const kneeR = plus(hipR, along(spec.legs.r[0], BONE.thigh));

    const raw: Record<JointKey, CanvasPoint> = {
        hip, neck, head, shoulderL, shoulderR, hipL, hipR, elbowL, elbowR, kneeL, kneeR,
        wristL: plus(elbowL, along(spec.arms.l[1], BONE.foreArm)),
        wristR: plus(elbowR, along(spec.arms.r[1], BONE.foreArm)),
        ankleL: plus(kneeL, along(spec.legs.l[1], BONE.shin)),
        ankleR: plus(kneeR, along(spec.legs.r[1], BONE.shin)),
    };

    // Into the unit box, at one scale shared by every pose — deliberately not
    // "stretch each pose to fill the box". Same bones, same body: a crouching
    // figure is genuinely shorter than a standing one, and swapping poses
    // never resizes the person.
    const xs = JOINT_KEYS.map((key) => raw[key].x);
    const ys = JOINT_KEYS.map((key) => raw[key].y);
    const minY = Math.min(...ys);
    const midX = (Math.min(...xs) + Math.max(...xs)) / 2;
    const points = {} as Record<JointKey, readonly [number, number]>;
    for (const key of JOINT_KEYS) {
        points[key] = [
            0.5 + (raw[key].x - midX) / (POSE_SCALE * FIGURE_ASPECT),
            (raw[key].y - minY) / POSE_SCALE,
        ];
    }
    return points;
};

// The height an upright figure occupies in raw units: crown to heel with the
// legs straight. Every pose is divided by this one number.
const POSE_SCALE = BONE.torso + BONE.head + BONE.hipDrop + BONE.thigh + BONE.shin;

const POSE_SPECS: Record<PosePresetKey, PoseSpec> = {
    standing: { arms: { l: [-8, -6], r: [8, 6] }, legs: { l: [-3, -2], r: [3, 2] } },
    contrapposto: {
        lean: 4, shoulderTilt: -4, hipTilt: 5,
        arms: { l: [-10, -14], r: [6, 10] }, legs: { l: [-1, 0], r: [9, 4] },
    },
    handsOnHips: { arms: { l: [-50, 25], r: [50, -25] }, legs: { l: [-5, -3], r: [5, 3] } },
    armsCrossed: { arms: { l: [-38, 62], r: [38, -62] }, legs: { l: [-4, -2], r: [4, 2] } },
    tPose: { arms: { l: [-90, -90], r: [90, 90] }, legs: { l: [-4, -3], r: [4, 3] } },
    armsUp: { arms: { l: [-168, -178], r: [168, 178] }, legs: { l: [-5, -4], r: [5, 4] } },

    walking: { lean: 2, arms: { l: [20, 15], r: [-20, -14] }, legs: { l: [-25, -12], r: [25, 14] } },
    running: { lean: -10, arms: { l: [-28, -88], r: [28, 88] }, legs: { l: [-42, -74], r: [38, 24] } },
    jumping: { arms: { l: [-158, -172], r: [158, 172] }, legs: { l: [-22, -48], r: [22, 48] } },
    kicking: { lean: 10, arms: { l: [-34, -22], r: [34, 22] }, legs: { l: [-4, -2], r: [72, 62] } },
    reaching: { lean: -8, headTilt: -6, arms: { l: [-12, -8], r: [122, 132] }, legs: { l: [-6, -3], r: [8, 4] } },
    bowing: { lean: -46, headTilt: -12, arms: { l: [-6, -4], r: [6, 4] }, legs: { l: [-3, 0], r: [3, 0] } },

    sitting: { lean: 6, arms: { l: [12, 42], r: [16, 46] }, legs: { l: [84, 4], r: [78, 1] } },
    sittingFloor: { lean: -12, arms: { l: [-26, -10], r: [26, 10] }, legs: { l: [76, 82], r: [70, 76] } },
    kneeling: { lean: 4, arms: { l: [-14, -8], r: [14, 8] }, legs: { l: [12, 96], r: [74, 8] } },
    crouching: { lean: -16, arms: { l: [24, 58], r: [28, 62] }, legs: { l: [64, 2], r: [56, -3] } },
    lying: { lean: 88, arms: { l: [86, 88], r: [94, 96] }, legs: { l: [94, 92], r: [84, 82] } },

    wave: { headTilt: -4, arms: { l: [-8, -6], r: [148, 172] }, legs: { l: [-4, -2], r: [4, 2] } },
    pointing: { arms: { l: [-10, -8], r: [95, 95] }, legs: { l: [-4, -2], r: [6, 3] } },
    thinking: { lean: -3, headTilt: 6, arms: { l: [-26, 64], r: [26, -140] }, legs: { l: [-4, -2], r: [4, 2] } },
    leaning: { lean: -12, arms: { l: [-20, -10], r: [16, 10] }, legs: { l: [-6, 0], r: [11, 6] } },
};

const POSE_PRESETS: Record<PosePresetKey, PresetPoints> = Object.fromEntries(
    (Object.keys(POSE_SPECS) as PosePresetKey[]).map((key) => [key, buildPose(POSE_SPECS[key])]),
) as Record<PosePresetKey, PresetPoints>;

// Grouped the way someone looks for a pose — by what the body is doing, not by
// how the data was authored.
export const POSE_LIBRARY: readonly { group: string; poses: readonly PosePresetKey[] }[] = [
    { group: 'standing', poses: ['standing', 'contrapposto', 'handsOnHips', 'armsCrossed', 'tPose', 'armsUp'] },
    { group: 'motion', poses: ['walking', 'running', 'jumping', 'kicking', 'reaching', 'bowing'] },
    { group: 'seated', poses: ['sitting', 'sittingFloor', 'kneeling', 'crouching', 'lying'] },
    { group: 'gesture', poses: ['wave', 'pointing', 'thinking', 'leaning'] },
];

let figureCounter = 0;

export const createFigure = (
    preset: PosePresetKey,
    dims: CanvasDimensions,
    center?: CanvasPoint,
    shade = 0,
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
    const figure = { id: `figure-${Date.now()}-${figureCounter}`, joints, shade };
    // Centre on the joints, not on the nominal unit box: no preset fills the
    // box exactly (a crown sits below its top edge, a seated figure leans to
    // one side), and every later transform pivots on the joint bounds. Making
    // the two agree here is what lets a pose be swapped in place without the
    // figure drifting.
    return centerFigureAt(figure, { x: cx, y: cy });
};

// Where the next figure should land. Dropping every one at the canvas centre
// stacks them exactly on top of each other, which looks like nothing happened
// and leaves the buried figure unreachable. Candidates walk outward from the
// centre; the first one clear of the figures already placed wins, and when a
// crowd has taken them all the figure cascades so it is at least grabbable.
const PLACEMENT_CANDIDATES: readonly (readonly [number, number])[] = [
    [0.5, 0.5], [0.26, 0.5], [0.74, 0.5], [0.38, 0.44], [0.62, 0.56],
    [0.14, 0.46], [0.86, 0.54], [0.5, 0.38], [0.5, 0.62],
];

export const placeNewFigure = (existing: readonly PoseFigure[], dims: CanvasDimensions): CanvasPoint => {
    const centers = existing.map((figure) => {
        const bounds = figureBounds(figure);
        return { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
    });
    const clearance = Math.min(dims.width, dims.height) * 0.12;
    for (const [fx, fy] of PLACEMENT_CANDIDATES) {
        const candidate = { x: dims.width * fx, y: dims.height * fy };
        if (centers.every((center) => Math.hypot(center.x - candidate.x, center.y - candidate.y) > clearance)) {
            return candidate;
        }
    }
    const step = clearance * 0.8;
    const overflow = existing.length - PLACEMENT_CANDIDATES.length + 1;
    return {
        x: Math.min(dims.width * 0.9, dims.width * 0.5 + step * overflow),
        y: Math.min(dims.height * 0.9, dims.height * 0.5 + step * overflow),
    };
};

// Every figure whose body is under the point, bottom of the stack first.
const figuresAt = (
    figures: readonly PoseFigure[],
    point: CanvasPoint,
    tolerance = 0,
): PoseFigure[] => figures.filter((figure) => hitTestBody(figure, point, tolerance));

// Clicking a pile of overlapping figures walks down it instead of always
// returning the top one, which would leave anything underneath unreachable.
export const nextFigureAt = (
    figures: readonly PoseFigure[],
    point: CanvasPoint,
    selectedId: string | null,
    tolerance = 0,
): PoseFigure | null => {
    const hits = figuresAt(figures, point, tolerance);
    if (hits.length === 0) return null;
    const index = hits.findIndex((figure) => figure.id === selectedId);
    if (index < 0) return hits[hits.length - 1];
    return hits[(index + hits.length - 1) % hits.length];
};

// Swaps the pose while keeping the figure where it is and roughly how big it
// is: re-entry (principle 10) applies inside the dialog too.
export const applyPreset = (figure: PoseFigure, preset: PosePresetKey, dims: CanvasDimensions): PoseFigure => {
    const fresh = createFigure(preset, dims, figureCenter(figure), figure.shade);
    // Matched on `figureUnit`, the body, not on the bounding box: box height
    // changes with the pose (a crouch is shorter than a stand), so matching
    // boxes would resize the person every time the pose changed.
    return { ...scaleFigure(fresh, figureUnit(figure) / figureUnit(fresh)), id: figure.id };
};

// --- the skeleton ------------------------------------------------------------
//
// Joints are not loose points. Every one hangs off a parent, rooted at the
// hip, so dragging a shoulder brings the whole arm with it and a limb cannot
// be stretched into rubber: a drag rotates the bone about its parent and
// carries everything below it rigidly, which is what the wooden joint does.

export const JOINT_PARENT: Record<JointKey, JointKey | null> = {
    hip: null,
    hipL: 'hip', hipR: 'hip', neck: 'hip',
    kneeL: 'hipL', ankleL: 'kneeL',
    kneeR: 'hipR', ankleR: 'kneeR',
    shoulderL: 'neck', shoulderR: 'neck', head: 'neck',
    elbowL: 'shoulderL', wristL: 'elbowL',
    elbowR: 'shoulderR', wristR: 'elbowR',
};

// The joint plus everything hanging off it.
export const subtreeOf = (key: JointKey): JointKey[] => JOINT_KEYS.filter((candidate) => {
    let walk: JointKey | null = candidate;
    while (walk) {
        if (walk === key) return true;
        walk = JOINT_PARENT[walk];
    }
    return false;
});

const SUBTREES = Object.fromEntries(JOINT_KEYS.map((key) => [key, subtreeOf(key)])) as Record<JointKey, JointKey[]>;

// Forward kinematics: swing `key` toward `target` about its parent and take
// its subtree along. Bone lengths never change, so a figure keeps its
// proportions however hard it is posed. Dragging the root moves the figure.
export const swingJoint = (figure: PoseFigure, key: JointKey, target: CanvasPoint): PoseFigure => {
    const parentKey = JOINT_PARENT[key];
    if (!parentKey) {
        return translateFigure(figure, target.x - figure.joints[key].x, target.y - figure.joints[key].y);
    }
    const pivot = figure.joints[parentKey];
    const from = figure.joints[key];
    const beforeLength = Math.hypot(from.x - pivot.x, from.y - pivot.y);
    const afterLength = Math.hypot(target.x - pivot.x, target.y - pivot.y);
    // A pointer sitting exactly on the parent has no direction to give.
    if (beforeLength < 1e-6 || afterLength < 1e-6) return figure;
    const delta = Math.atan2(target.y - pivot.y, target.x - pivot.x)
        - Math.atan2(from.y - pivot.y, from.x - pivot.x);
    const joints = { ...figure.joints };
    for (const member of SUBTREES[key]) joints[member] = rotatePoint(joints[member], pivot, delta);
    return { ...figure, joints };
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
// The pivot every transform in this module turns about: the centre of the
// *joint* bounds, not of the visual box. Stated once, because `createFigure`
// depends on it agreeing with how presets are normalised.
export const figureCenter = (figure: PoseFigure): CanvasPoint => {
    const bounds = figureBounds(figure);
    return { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
};

export const centerFigureAt = (figure: PoseFigure, point: CanvasPoint): PoseFigure => {
    const center = figureCenter(figure);
    return translateFigure(figure, point.x - center.x, point.y - center.y);
};

const figurePadding = (figure: PoseFigure): number =>
    Math.max(figureUnit(figure) * HEAD_RADIUS_RATIO, 1);

// Scaled and centred to sit inside `box` with a margin. The thumbnail grid
// needs this; keeping it here means the "visual bounds, not joint bounds"
// choice is made once, in the module that knows the difference.
export const fitFigureInto = (figure: PoseFigure, box: CanvasDimensions, pad = 0): PoseFigure => {
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

// Used when a saved sketch is re-opened on a differently sized canvas. Goes
// through the same uniform transform as the strokes, so the drawing and the
// figures standing in it stay in register.
export const transformFigures = (
    figures: readonly PoseFigure[],
    transform: CanvasTransform,
): PoseFigure[] => (isIdentityTransform(transform)
    ? figures as PoseFigure[]
    : figures.map((figure) => mapJoints(figure, (p) => applyTransform(p, transform))));

export const moveJoint = (figure: PoseFigure, key: JointKey, point: CanvasPoint): PoseFigure => ({
    ...figure,
    joints: { ...figure.joints, [key]: point },
});

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

// Uniform scale about the figure's own centre.
export const scaleFigure = (figure: PoseFigure, factor: number, origin?: CanvasPoint): PoseFigure => {
    const pivot = origin ?? figureCenter(figure);
    const factorApplied = safeFactor(factor);
    return mapJoints(figure, (p) => ({
        x: pivot.x + (p.x - pivot.x) * factorApplied,
        y: pivot.y + (p.y - pivot.y) * factorApplied,
    }));
};

// Mirroring the coordinates is enough: the bone list is symmetric, so no
// left/right relabelling is needed for the figure to render correctly.
export const flipFigure = (figure: PoseFigure): PoseFigure => {
    const bounds = figureBounds(figure);
    const axis = bounds.x + bounds.width / 2;
    return mapJoints(figure, (p) => ({ x: axis * 2 - p.x, y: p.y }));
};

// Where the point projects onto the segment (`t`), and how far it is from it.
// A tapered limb needs both: its half-width at the point of closest approach
// is the radius interpolated at that same `t`.
const projectOnSegment = (point: CanvasPoint, a: CanvasPoint, b: CanvasPoint): { t: number; distance: number } => {
    const vx = b.x - a.x;
    const vy = b.y - a.y;
    const lengthSq = vx * vx + vy * vy;
    if (lengthSq === 0) return { t: 0, distance: Math.hypot(point.x - a.x, point.y - a.y) };
    const t = Math.max(0, Math.min(1, ((point.x - a.x) * vx + (point.y - a.y) * vy) / lengthSq));
    return { t, distance: Math.hypot(point.x - (a.x + vx * t), point.y - (a.y + vy * t)) };
};

export const distanceToSegment = (point: CanvasPoint, a: CanvasPoint, b: CanvasPoint): number =>
    projectOnSegment(point, a, b).distance;

const insideSegment = (point: CanvasPoint, segment: Segment, tolerance: number): boolean => {
    const { t, distance } = projectOnSegment(point, segment.from, segment.to);
    return distance <= segment.fromRadius + (segment.toRadius - segment.fromRadius) * t + tolerance;
};

// The point taken back into the ellipse's own frame, where "inside" is the
// unit circle.
const insideEllipse = (point: CanvasPoint, ellipse: Ellipse, tolerance: number): boolean => {
    const cos = Math.cos(-ellipse.angle);
    const sin = Math.sin(-ellipse.angle);
    const dx = point.x - ellipse.center.x;
    const dy = point.y - ellipse.center.y;
    const x = (dx * cos - dy * sin) / Math.max(ellipse.radiusX + tolerance, 1e-6);
    const y = (dx * sin + dy * cos) / Math.max(ellipse.radiusY + tolerance, 1e-6);
    return x * x + y * y <= 1;
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
// True when the point is on the mannequin's silhouette, which is what "grab
// the body and move it" means. Tested against `figureParts` — the shapes that
// are actually drawn — rather than against a second table of limb widths: two
// tables of the same physical fact drift, and then what you can grab stops
// matching what you can see.
export const hitTestBody = (figure: PoseFigure, point: CanvasPoint, tolerance = 0): boolean => {
    const parts = figureParts(figure);
    if ([parts.head, parts.chest, parts.pelvis, ...parts.hands, ...parts.feet]
        .some((ellipse) => insideEllipse(point, ellipse, tolerance))) return true;
    if ([parts.waist, ...parts.hipBalls, ...parts.balls]
        .some((ball) => Math.hypot(point.x - ball.center.x, point.y - ball.center.y) <= ball.radius + tolerance)) {
        return true;
    }
    return [parts.neck, parts.spine, ...parts.limbs]
        .some((segment) => insideSegment(point, segment, tolerance));
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

const lerp = (a: CanvasPoint, b: CanvasPoint, t: number): CanvasPoint => ({
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
});
const midOf = (a: CanvasPoint, b: CanvasPoint): CanvasPoint => lerp(a, b, 0.5);
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
    const unit = figureUnit(figure);
    const u = (ratio: number) => unit * ratio;

    const shoulderMid = midOf(joints.shoulderL, joints.shoulderR);
    const hipMid = midOf(joints.hipL, joints.hipR);
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

// Three tones per figure, no gradients: the body, the joint balls a step
// darker so the articulation reads, and the head a step lighter so it does not
// merge into the chest. Neutral grey, not wood — the manikin's structure is
// the message, and a wood colour only invites the model to paint a wooden doll.
//
// The three shades exist for crowds: two figures in one grey merge into a
// single blob where they overlap, and the model then has no way to tell how
// many people are in the frame. Lightness only, so they still read as the same
// material, and the selected figure keeps its own cool tint on top of this.
// `rim` is a hairline round the silhouette. Without it a figure dissolves
// into white paper at its edges and two overlapping figures merge into one
// shape; with it each body reads as a separate solid object.
export interface FigureTone { body: string; joint: string; head: string; rim: string }

const FIGURE_SHADES: readonly FigureTone[] = [
    { body: '#aeb2b6', joint: '#8f9398', head: '#b9bdc1', rim: '#7c8085' },
    { body: '#8d9298', joint: '#70757b', head: '#9aa0a6', rim: '#5d6268' },
    { body: '#c0c5ca', joint: '#a2a7ad', head: '#ccd0d4', rim: '#8e9399' },
];

const SELECTED_TONE: FigureTone = { body: '#a2abbd', joint: '#828da3', head: '#adb5c5', rim: '#6d7789' };

// Which tone the next figure should wear. Least-used rather than "one past
// the count": after a delete, counting the list hands out a shade another
// figure is already wearing, which is the collision shades exist to prevent.
// Ties go to the lowest index, so the first three figures still read 0, 1, 2.
export const leastUsedShade = (figures: readonly PoseFigure[]): number => {
    const counts = FIGURE_SHADES.map(
        (_, index) => figures.filter((figure) => (figure.shade ?? 0) === index).length,
    );
    return counts.indexOf(Math.min(...counts));
};

const toneFor = (figure: PoseFigure, selected = false): FigureTone => (selected
    ? SELECTED_TONE
    : FIGURE_SHADES[(figure.shade ?? 0) % FIGURE_SHADES.length]);
const HANDLE_FILL = '#2563eb';
const HANDLE_STROKE = '#ffffff';

// Every shape is drawn twice: once grown by the rim width in the rim colour,
// once at its true size in the body colour. Because the first pass is one flat
// colour, the seams between parts vanish and what is left is a single outline
// around the whole figure — no silhouette union to compute.
const fillEllipse = (ctx: CanvasRenderingContext2D, ellipse: Ellipse, grow = 0): void => {
    ctx.beginPath();
    ctx.ellipse(
        ellipse.center.x,
        ellipse.center.y,
        Math.max(ellipse.radiusX + grow, 0.5),
        Math.max(ellipse.radiusY + grow, 0.5),
        ellipse.angle,
        0,
        Math.PI * 2,
    );
    ctx.fill();
};

// A tapered capsule: the quad between the two end circles plus the circles
// themselves. Close enough to a turned wooden limb, and it degrades to a
// plain capsule when the radii match.
const fillSegment = (ctx: CanvasRenderingContext2D, segment: Segment, grow = 0): void => {
    const fromRadius = segment.fromRadius + grow;
    const toRadius = segment.toRadius + grow;
    const dx = segment.to.x - segment.from.x;
    const dy = segment.to.y - segment.from.y;
    const length = Math.hypot(dx, dy);
    if (length > 1e-6) {
        const px = -dy / length;
        const py = dx / length;
        ctx.beginPath();
        ctx.moveTo(segment.from.x + px * fromRadius, segment.from.y + py * fromRadius);
        ctx.lineTo(segment.to.x + px * toRadius, segment.to.y + py * toRadius);
        ctx.lineTo(segment.to.x - px * toRadius, segment.to.y - py * toRadius);
        ctx.lineTo(segment.from.x - px * fromRadius, segment.from.y - py * fromRadius);
        ctx.closePath();
        ctx.fill();
    }
    ctx.beginPath();
    ctx.arc(segment.from.x, segment.from.y, Math.max(fromRadius, 0.5), 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(segment.to.x, segment.to.y, Math.max(toRadius, 0.5), 0, Math.PI * 2);
    ctx.fill();
};

const RIM_RATIO = 0.006;

export const drawFigure = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    options: { selected?: boolean } = {},
): void => {
    const parts = figureParts(figure);
    const tone = toneFor(figure, options.selected === true);
    const rim = Math.max(figureUnit(figure) * RIM_RATIO, 1);
    ctx.save();
    ctx.globalCompositeOperation = 'source-over';

    const fillBall = (ball: Ball, grow = 0) => {
        ctx.beginPath();
        ctx.arc(ball.center.x, ball.center.y, Math.max(ball.radius + grow, 0.5), 0, Math.PI * 2);
        ctx.fill();
    };

    // Pass one: the whole figure grown by the rim, in one flat colour.
    ctx.fillStyle = tone.rim;
    fillSegment(ctx, parts.neck, rim);
    fillSegment(ctx, parts.spine, rim);
    for (const limb of parts.limbs) fillSegment(ctx, limb, rim);
    for (const hand of parts.hands) fillEllipse(ctx, hand, rim);
    for (const foot of parts.feet) fillEllipse(ctx, foot, rim);
    for (const ball of parts.hipBalls) fillBall(ball, rim);
    fillEllipse(ctx, parts.chest, rim);
    fillEllipse(ctx, parts.pelvis, rim);
    for (const ball of [parts.waist, ...parts.balls]) fillBall(ball, rim);
    fillEllipse(ctx, parts.head, rim);

    // Pass two, at true size. Body first, joints over it, head last: the
    // drawing order is what makes the balls read as articulation rather than
    // as lumps under the limbs.
    ctx.fillStyle = tone.body;
    fillSegment(ctx, parts.neck);
    fillSegment(ctx, parts.spine);
    for (const limb of parts.limbs) fillSegment(ctx, limb);
    for (const hand of parts.hands) fillEllipse(ctx, hand);
    for (const foot of parts.feet) fillEllipse(ctx, foot);

    ctx.fillStyle = tone.joint;
    for (const ball of parts.hipBalls) fillBall(ball);

    ctx.fillStyle = tone.body;
    fillEllipse(ctx, parts.chest);
    fillEllipse(ctx, parts.pelvis);

    ctx.fillStyle = tone.joint;
    fillBall(parts.waist);
    for (const ball of parts.balls) fillBall(ball);

    ctx.fillStyle = tone.head;
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
