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
// The mannequin is three-dimensional: every joint carries a depth, each figure
// is seen through its own perspective camera, and the parts are drawn back to
// front. That is not polish. A flat figure can only ever say "the arm points
// left"; it cannot say "the arm points at you", cannot let one limb pass in
// front of the body, and hands the model a paper cut-out to copy. Depth is the
// difference between a pose diagram and a pose reference, and it is the one
// thing the first, flat version could not express at any amount of fiddling.
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
    | 'ankleL' | 'ankleR'
    // The detail tier. See DETAIL_JOINT_KEYS.
    | 'face' | 'handL' | 'handR' | 'toeL' | 'toeR';

// The body every pose in the library is written in, and the only joints a
// figure shows handles for by default. Fifteen is enough to say what a body is
// doing, and few enough to read at a glance.
export const CORE_JOINT_KEYS: readonly JointKey[] = [
    'head', 'neck', 'shoulderL', 'shoulderR', 'elbowL', 'elbowR', 'wristL', 'wristR',
    'hip', 'hipL', 'hipR', 'kneeL', 'kneeR', 'ankleL', 'ankleR',
];

// The five the mannequin used to *guess* from the rest of the body: which way
// the face looks, which way each hand points, which way each foot points.
//
// The tier stops here, and the boundary is not a matter of taste — it is
// exactly what a pose estimator's landmark set can fill (a nose, two ears, the
// finger landmarks, heels and toes; see `.design/pose-from-image.md`). A joint
// no photograph could ever fill would be a handle with no source: something
// the user must pose by hand for ever, on every figure, to get a body that
// still reads the same from three metres away. A spine segment is the obvious
// candidate and the obvious mistake — the landmark set has no spine.
export const DETAIL_JOINT_KEYS: readonly JointKey[] = ['face', 'handL', 'handR', 'toeL', 'toeR'];

export const JOINT_KEYS: readonly JointKey[] = [...CORE_JOINT_KEYS, ...DETAIL_JOINT_KEYS];

const DETAIL_SET = new Set<JointKey>(DETAIL_JOINT_KEYS);
export const isDetailJoint = (key: JointKey): boolean => DETAIL_SET.has(key);

// Joints live in world space: x/y are canvas pixels, z is depth in the same
// unit, positive toward the viewer. A `Vec3` is structurally a `CanvasPoint`
// with a depth, so every existing caller that only wanted x/y still compiles.
export interface Vec3 { x: number; y: number; z: number }

// Where the figure is being looked at from, as a memo of the rotation already
// baked into the joints. Keeping the total (rather than composing deltas onto
// the joints blind) is what makes turning drift-free and reversible, and what
// lets the toolbar show the angle as a number instead of an alias.
export interface FigureTurn { yaw: number; pitch: number }

export interface PoseFigure {
    id: string;
    joints: Record<JointKey, Vec3>;
    // Which tone this figure is drawn in. Assigned once, at creation, and
    // carried in the data: figures in a crowd have to stay told apart, and
    // colouring by list position would recolour everyone when one is deleted.
    // Optional so a sketch saved before shades existed still opens.
    shade?: number;
    // Optional for the same reason: a sketch saved flat opens facing front.
    turn?: FigureTurn;
    // Whether this figure is being posed at the detail tier: the five extra
    // joints are always *there* and always drive the drawing, but their
    // handles — and the ability to grab them — appear only when asked for, or
    // when something has actually set them (a photograph). Per figure, not per
    // canvas: one person in a crowd may need a hand posed while the others do
    // not, and turning detail on for all of them to fix one is noise.
    detail?: boolean;
}

// Which joints this figure answers to right now.
export const jointKeysOf = (figure: PoseFigure): readonly JointKey[] => (
    figure.detail ? JOINT_KEYS : CORE_JOINT_KEYS
);

// Turning the tier off does not throw the detail away — the joints stay where
// they were put, they just stop being grabbable. "Done" is not "locked"
// (principle 10), and a hand posed by hand or lifted from a photograph must
// survive a glance at the simpler view.
export const setFigureDetail = (figure: PoseFigure, detail: boolean): PoseFigure => ({
    ...completeFigure(figure),
    detail,
});

export interface Rect { x: number; y: number; width: number; height: number }

// The head's long radius: also the padding that keeps the visual box (and
// the grips on its corners) clear of the silhouette.
const HEAD_RADIUS_RATIO = 0.07;

// Every thickness in the manikin is a fraction of "how big is this person",
// and that number must NOT be the bounding box: a lying figure has the same
// body as a standing one but a fifth of the box, which would shrink its limbs
// into the stick figure this module exists to avoid. Nor can it be a projected
// length — a torso turned away from the camera is foreshortened to nothing and
// would take the whole body's thickness down with it. The torso bone measured
// *in 3D* is the one number no pose and no camera angle can change.
const TORSO_HEIGHT_RATIO = 0.36;

// --- three dimensions --------------------------------------------------------

// Depth is read defensively everywhere it is read: a sketch saved before the
// mannequin had a third dimension comes back with flat joints.
const zOf = (point: Vec3): number => point.z ?? 0;

const sub3 = (a: Vec3, b: Vec3): Vec3 => ({ x: a.x - b.x, y: a.y - b.y, z: zOf(a) - zOf(b) });
const add3 = (a: Vec3, b: Vec3): Vec3 => ({ x: a.x + b.x, y: a.y + b.y, z: zOf(a) + zOf(b) });
const mul3 = (a: Vec3, factor: number): Vec3 => ({ x: a.x * factor, y: a.y * factor, z: zOf(a) * factor });
const len3 = (a: Vec3): number => Math.hypot(a.x, a.y, zOf(a));
const dot3 = (a: Vec3, b: Vec3): number => a.x * b.x + a.y * b.y + zOf(a) * zOf(b);
const cross3 = (a: Vec3, b: Vec3): Vec3 => ({
    x: a.y * zOf(b) - zOf(a) * b.y,
    y: zOf(a) * b.x - a.x * zOf(b),
    z: a.x * b.y - a.y * b.x,
});
const norm3 = (a: Vec3): Vec3 => {
    const length = len3(a);
    return length < 1e-9 ? { x: 0, y: 0, z: 0 } : mul3(a, 1 / length);
};
const dist3 = (a: Vec3, b: Vec3): number => len3(sub3(a, b));

const rad = (degrees: number) => (degrees * Math.PI) / 180;

// Rodrigues: turn `point` about a unit `axis` through the origin.
const rotateAxis = (point: Vec3, axis: Vec3, angle: number): Vec3 => {
    const cos = Math.cos(angle);
    const sin = Math.sin(angle);
    return add3(
        add3(mul3(point, cos), mul3(cross3(axis, point), sin)),
        mul3(axis, dot3(axis, point) * (1 - cos)),
    );
};

// Yaw turns the body about its own vertical axis (the canvas y axis, which
// points down); pitch tips it about the horizontal screen axis, which is what
// "seen from above / below" means. Always composed in this fixed order from
// the totals, never accumulated onto the joints — alternating incremental
// rotations creep roll into the figure and a manikin with roll looks broken.
const rotateView = (point: Vec3, turn: FigureTurn): Vec3 => {
    const yaw = rad(turn.yaw);
    const pitch = rad(turn.pitch);
    const cy = Math.cos(yaw);
    const sy = Math.sin(yaw);
    const afterYaw = { x: point.x * cy + zOf(point) * sy, y: point.y, z: zOf(point) * cy - point.x * sy };
    const cp = Math.cos(pitch);
    const sp = Math.sin(pitch);
    return { x: afterYaw.x, y: afterYaw.y * cp - afterYaw.z * sp, z: afterYaw.y * sp + afterYaw.z * cp };
};

export const figureTurn = (figure: PoseFigure): FigureTurn => figure.turn ?? { yaw: 0, pitch: 0 };

export const figureUnit = (figure: PoseFigure): number => {
    // hip → neck, the bone itself, in three dimensions. The hip *line's*
    // midpoint drifts with the torso's lean (the stubs to hipL/hipR rotate
    // with it), which would make the same body measure differently lying down
    // than standing up.
    const torso = dist3(figure.joints.neck, figure.joints.hip);
    return Math.max(torso / TORSO_HEIGHT_RATIO, 1);
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
export const unprojectPoint = (point: CanvasPoint, z: number, projection: Projection): Vec3 => {
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

// --- the pose library --------------------------------------------------------
//
// Presets are normalised into a unit box (x across the figure's width, y from
// crown to ankles, z the same scale as y). They are starting points, not a pose
// picker standing between the user and the canvas: picking the tool drops the
// default figure straight onto the surface and the preset row only appears once
// one is selected, to swap a pose in place.

// A figure lands at 70% of the canvas height, centred. Big enough to read as
// the subject, small enough to leave room for the scene around it.
const FIGURE_HEIGHT_RATIO = 0.7;
// Width of the unit box relative to its height. Arms out to the side need
// more room than a body is wide.
const FIGURE_ASPECT = 0.45;

export type PosePresetKey =
    | 'standing' | 'contrapposto' | 'handsOnHips' | 'armsCrossed' | 'tPose' | 'armsUp' | 'armsBehind'
    | 'walking' | 'running' | 'jumping' | 'kicking' | 'reaching' | 'bowing'
    | 'throwing' | 'dancing' | 'climbing'
    | 'sitting' | 'sittingFloor' | 'crossLegged' | 'kneeling' | 'crouching' | 'hugKnees' | 'reclining'
    | 'lying' | 'lyingSide' | 'prone' | 'pushUp'
    | 'wave' | 'pointing' | 'thinking' | 'leaning' | 'shrug' | 'armsOpen' | 'lookingBack'
    | 'salute' | 'presenting';

type PresetPoints = Record<JointKey, readonly [number, number, number]>;

// Poses are declared as bone angles, not as coordinates. Two reasons: a table
// of forty-five joint numbers is unreadable and unmaintainable, and — since a
// drag rotates bones and never stretches them (see `swingJoint`) — every pose
// in the library has to be built from the same bone lengths or applying one
// would silently change the figure's proportions.
//
// A bone is one or two numbers: `-50` is the flat angle in the plane of the
// screen (0 points straight down, +90 to the screen's right, ±180 straight up),
// and `[-50, 30]` lifts that same bone 30° *out* of the screen toward the
// viewer (negative pushes it behind the body). Writing depth as a second
// number rather than a second table means the flat poses that were already
// right stay exactly as they were written, and reading "the upper arm is at
// -50, swung 30 toward the camera" is still something you can picture.
type Angle = number | readonly [number, number];

const angleOfBone = (angle: Angle): readonly [number, number] => (
    typeof angle === 'number' ? [angle, 0] : angle
);

interface PoseSpec {
    lean?: number;          // torso, from upright, in the plane of the screen
    bend?: number;          // torso, out of it: + folds the body toward the viewer
    twist?: number;         // shoulders against hips, about the body's own axis
    headTilt?: number;      // head, relative to the torso, in the screen plane
    headTurn?: number;      // head, about the body's vertical axis
    headNod?: number;       // head, out of the screen plane
    shoulderTilt?: number;
    hipTilt?: number;
    arms: { l: readonly [Angle, Angle]; r: readonly [Angle, Angle] };
    legs: { l: readonly [Angle, Angle]; r: readonly [Angle, Angle] };
}

// One skeleton for every pose, in arbitrary units — the result is normalised.
const BONE = {
    torso: 0.36, head: 0.11,
    shoulderSpan: 0.085, shoulderDrop: 0.035,
    upperArm: 0.155, foreArm: 0.145,
    hipSpan: 0.052, hipDrop: 0.022,
    thigh: 0.235, shin: 0.225,
    // The detail bones. Their lengths are chosen so that a figure whose detail
    // joints are still where the body put them draws as it did when the
    // renderer worked them out for itself — the tier adds control, not a
    // different-looking mannequin. The toe is the one exception: at the length
    // that reproduced the old foot exactly, its handle sat on top of the
    // ankle's and could not be grabbed. The foot is a little longer for it,
    // which it wanted to be anyway.
    face: 0.075, hand: 0.034, toe: 0.048,
} as const;

const ORIGIN: Vec3 = { x: 0, y: 0, z: 0 };

// Down is 0, so a spec reads the way a person describes a limb: "hanging" is 0.
// The second number swings the same bone out of the screen plane, which is
// what a flat angle can never say — and is exactly the information a pose
// reference is for.
const along = (angle: Angle, length: number): Vec3 => {
    const [flat, depth] = angleOfBone(angle);
    const planar = Math.cos(rad(depth)) * length;
    return {
        x: Math.sin(rad(flat)) * planar,
        y: Math.cos(rad(flat)) * planar,
        z: Math.sin(rad(depth)) * length,
    };
};

// Rotating one offset, rather than adding two angled vectors: the shoulder and
// hip stubs have to keep their length when the shoulder or hip line tilts, or
// the "same bones in every pose" guarantee quietly breaks for tilted poses.
const turned = (offset: Vec3, degrees: number): Vec3 =>
    rotateAxis(offset, { x: 0, y: 0, z: 1 }, rad(degrees));

// --- the detail joints, when nothing has set them ----------------------------
//
// One set of formulas, used by three callers: the preset builder below (so all
// thirty-six poses get them without a line of editing), `completeFigure` (so a
// sketch saved before the tier existed opens with them), and nothing else —
// the renderer now *reads* these joints instead of working them out, which is
// what makes them posable at all.

// Which way the body faces: across the shoulders, crossed with the spine.
export const bodyForwardOf = (joints: Pick<Record<JointKey, Vec3>, 'neck' | 'hip' | 'shoulderL' | 'shoulderR'>): Vec3 => {
    const forward = cross3(sub3(joints.neck, joints.hip), sub3(joints.shoulderR, joints.shoulderL));
    return len3(forward) < 1e-9 ? { x: 0, y: 0, z: 1 } : norm3(forward);
};

// The part of `forward` that survives once the component along `axis` is taken
// out — "point this the way the body faces, but keep it square to the limb it
// hangs off". A raised foot carries its toes round with it this way, and a
// tipped head keeps its face on the front of the skull.
const squareTo = (forward: Vec3, axis: Vec3): Vec3 => {
    const flat = sub3(forward, mul3(axis, dot3(forward, axis)));
    return len3(flat) > 1e-3 ? norm3(flat) : forward;
};

export const derivedDetailJoints = (
    joints: Record<JointKey, Vec3>,
    unit: number,
): Record<'face' | 'handL' | 'handR' | 'toeL' | 'toeR', Vec3> => {
    const forward = bodyForwardOf(joints);
    const headAxis = norm3(sub3(joints.head, joints.neck));
    const alongArm = (from: JointKey, to: JointKey): Vec3 => {
        const direction = sub3(joints[to], joints[from]);
        return len3(direction) < 1e-9 ? forward : norm3(direction);
    };
    const overFoot = (knee: JointKey, ankle: JointKey): Vec3 => {
        const shin = sub3(joints[ankle], joints[knee]);
        return squareTo(forward, len3(shin) < 1e-9 ? forward : norm3(shin));
    };
    return {
        face: add3(joints.head, mul3(squareTo(forward, headAxis), unit * BONE.face)),
        handL: add3(joints.wristL, mul3(alongArm('elbowL', 'wristL'), unit * BONE.hand)),
        handR: add3(joints.wristR, mul3(alongArm('elbowR', 'wristR'), unit * BONE.hand)),
        toeL: add3(joints.ankleL, mul3(overFoot('kneeL', 'ankleL'), unit * BONE.toe)),
        toeR: add3(joints.ankleR, mul3(overFoot('kneeR', 'ankleR'), unit * BONE.toe)),
    };
};

// A figure that predates the detail tier — or one built by hand — gets its
// five extra joints put where the body implies they are. Cheap, idempotent,
// and the reason nothing downstream has to cope with a missing joint.
export const completeFigure = (figure: PoseFigure): PoseFigure => {
    if (DETAIL_JOINT_KEYS.every((key) => figure.joints[key] !== undefined)) return figure;
    const joints = { ...figure.joints };
    for (const [key, point] of Object.entries(derivedDetailJoints(joints, figureUnit(figure)))) {
        if (joints[key as JointKey] === undefined) joints[key as JointKey] = point;
    }
    return { ...figure, joints };
};

const buildPose = (spec: PoseSpec): PresetPoints => {
    const hip = { ...ORIGIN };
    // The torso leans in the screen plane and bends out of it. Both are folded
    // into one direction so the neck cannot drift off the torso bone.
    const spine = along([(spec.lean ?? 0) + 180, -(spec.bend ?? 0)], BONE.torso);
    const neck = add3(hip, spine);
    // The torso's own axis, which the twist and the head's turn rotate about.
    const axis = norm3(spine);
    const head = add3(neck, along(
        [(spec.lean ?? 0) + (spec.headTilt ?? 0) + 180, -(spec.bend ?? 0) - (spec.headNod ?? 0)],
        BONE.head,
    ));

    const shoulderAngle = (spec.lean ?? 0) + (spec.shoulderTilt ?? 0);
    const twist = rad(spec.twist ?? 0);
    const shoulderStub = (side: number) => rotateAxis(
        turned({ x: side * BONE.shoulderSpan, y: BONE.shoulderDrop, z: 0 }, shoulderAngle),
        axis,
        twist,
    );
    const shoulderL = add3(neck, shoulderStub(-1));
    const shoulderR = add3(neck, shoulderStub(1));

    const hipAngle = (spec.lean ?? 0) + (spec.hipTilt ?? 0);
    const hipL = add3(hip, turned({ x: -BONE.hipSpan, y: BONE.hipDrop, z: 0 }, hipAngle));
    const hipR = add3(hip, turned({ x: BONE.hipSpan, y: BONE.hipDrop, z: 0 }, hipAngle));

    const elbowL = add3(shoulderL, along(spec.arms.l[0], BONE.upperArm));
    const elbowR = add3(shoulderR, along(spec.arms.r[0], BONE.upperArm));
    const kneeL = add3(hipL, along(spec.legs.l[0], BONE.thigh));
    const kneeR = add3(hipR, along(spec.legs.r[0], BONE.thigh));

    const raw = {
        hip, neck, head, shoulderL, shoulderR, hipL, hipR, elbowL, elbowR, kneeL, kneeR,
        wristL: add3(elbowL, along(spec.arms.l[1], BONE.foreArm)),
        wristR: add3(elbowR, along(spec.arms.r[1], BONE.foreArm)),
        ankleL: add3(kneeL, along(spec.legs.l[1], BONE.shin)),
        ankleR: add3(kneeR, along(spec.legs.r[1], BONE.shin)),
    } as Record<JointKey, Vec3>;
    // The head's turn is applied last: it rotates the skull about the torso's
    // axis without moving anything else, which is what "looking over your
    // shoulder" is.
    if (spec.headTurn) {
        raw.head = add3(neck, rotateAxis(sub3(raw.head, neck), axis, rad(spec.headTurn)));
    }
    // The detail joints are derived, never declared. That is what let the tier
    // be added without touching a single one of the thirty-six pose specs: a
    // preset says what the body is doing, and where the face and the toes go
    // follows from that until somebody says otherwise.
    Object.assign(raw, derivedDetailJoints(raw, BONE.torso / TORSO_HEIGHT_RATIO));

    // Into the unit box, at one scale shared by every pose — deliberately not
    // "stretch each pose to fill the box". Same bones, same body: a crouching
    // figure is genuinely shorter than a standing one, and swapping poses
    // never resizes the person.
    // Measured over the core joints only. A figure's size is its body: letting
    // the toes and the face push the unit box out would make every pose a
    // little smaller the day the detail tier was added, for no reason anyone
    // could see.
    const xs = CORE_JOINT_KEYS.map((key) => raw[key].x);
    const ys = CORE_JOINT_KEYS.map((key) => raw[key].y);
    const minY = Math.min(...ys);
    const midX = (Math.min(...xs) + Math.max(...xs)) / 2;
    const zs = CORE_JOINT_KEYS.map((key) => zOf(raw[key]));
    const midZ = (Math.min(...zs) + Math.max(...zs)) / 2;
    const points = {} as Record<JointKey, readonly [number, number, number]>;
    for (const key of JOINT_KEYS) {
        points[key] = [
            0.5 + (raw[key].x - midX) / (POSE_SCALE * FIGURE_ASPECT),
            (raw[key].y - minY) / POSE_SCALE,
            (zOf(raw[key]) - midZ) / POSE_SCALE,
        ];
    }
    return points;
};

// The height an upright figure occupies in raw units: crown to heel with the
// legs straight. Every pose is divided by this one number.
const POSE_SCALE = BONE.torso + BONE.head + BONE.hipDrop + BONE.thigh + BONE.shin;

const POSE_SPECS: Record<PosePresetKey, PoseSpec> = {
    standing: {
        arms: { l: [[-8, 8], [-6, 12]], r: [[8, 8], [6, 12]] },
        legs: { l: [-3, -2], r: [3, 2] },
    },
    contrapposto: {
        lean: 4, shoulderTilt: -4, hipTilt: 5, twist: -6,
        arms: { l: [[-10, 6], [-14, 10]], r: [[6, 10], [10, 14]] },
        legs: { l: [-1, 0], r: [9, 4] },
    },
    handsOnHips: {
        arms: { l: [[-52, -14], [26, 26]], r: [[52, -14], [-26, 26]] },
        legs: { l: [-5, -3], r: [5, 3] },
    },
    armsCrossed: {
        arms: { l: [[-38, -6], [62, 34]], r: [[38, -6], [-62, 34]] },
        legs: { l: [-4, -2], r: [4, 2] },
    },
    // The calibration pose: deliberately flat, so "arms out" is still the one
    // preset that shows the skeleton with nothing foreshortened.
    tPose: { arms: { l: [-90, -90], r: [90, 90] }, legs: { l: [-4, -3], r: [4, 3] } },
    armsUp: {
        arms: { l: [[-166, 12], [-176, 18]], r: [[166, 12], [176, 18]] },
        legs: { l: [-5, -4], r: [5, 4] },
    },
    armsBehind: {
        arms: { l: [[-14, -22], [34, -50]], r: [[14, -22], [-34, -50]] },
        legs: { l: [-6, -4], r: [6, 4] },
    },

    walking: {
        lean: 2,
        arms: { l: [[20, 18], [15, 20]], r: [[-20, -18], [-14, -20]] },
        legs: { l: [[-25, -14], [-12, -8]], r: [[25, 16], [14, 10]] },
    },
    running: {
        lean: -10, bend: 6,
        arms: { l: [[-28, 34], [-88, 50]], r: [[28, -30], [88, -40]] },
        legs: { l: [[-42, -40], [-74, -30]], r: [[38, 34], [24, 20]] },
    },
    jumping: {
        arms: { l: [[-158, 8], [-172, 10]], r: [[158, 8], [172, 10]] },
        legs: { l: [[-22, 26], [-48, 10]], r: [[22, 26], [48, 10]] },
    },
    kicking: {
        lean: 10, bend: -8,
        arms: { l: [[-34, 10], [-22, 14]], r: [[34, -10], [22, -14]] },
        legs: { l: [-4, -2], r: [[58, 42], [48, 30]] },
    },
    // Reaching *out of the picture* rather than off to one side: the whole
    // reason the mannequin has a third dimension.
    reaching: {
        lean: -8, bend: 8, headNod: 6,
        arms: { l: [[-12, 6], [-8, 10]], r: [[64, 58], [58, 68]] },
        legs: { l: [-6, -3], r: [8, 4] },
    },
    bowing: {
        lean: 10, bend: 44, headNod: 16,
        arms: { l: [[-6, 10], [-4, 12]], r: [[6, 10], [4, 12]] },
        legs: { l: [-3, 0], r: [3, 0] },
    },
    throwing: {
        lean: 8, twist: -26,
        arms: { l: [[-66, 42], [-58, 50]], r: [[148, -40], [118, -58]] },
        legs: { l: [[-16, -24], [-8, -14]], r: [[16, 26], [8, 12]] },
    },
    dancing: {
        lean: -10, shoulderTilt: -10, hipTilt: 12, twist: 12, headTilt: -8,
        arms: { l: [[-150, 20], [-168, 26]], r: [[60, -26], [86, -10]] },
        legs: { l: [[-10, 14], [-4, 8]], r: [[26, -10], [10, -6]] },
    },
    climbing: {
        bend: 10,
        arms: { l: [[-160, -24], [-172, -28]], r: [[30, -20], [70, -30]] },
        legs: { l: [[-6, -6], [-2, -4]], r: [[40, -30], [86, -26]] },
    },

    // Seated poses are where a flat figure fails hardest: a thigh coming
    // toward the camera is a *short* thigh, and there is no flat angle that
    // says that without breaking the bone length.
    sitting: {
        lean: 6,
        arms: { l: [[-10, 24], [16, 58]], r: [[10, 24], [-16, 58]] },
        legs: { l: [[-4, 84], [-2, 2]], r: [[4, 80], [2, 0]] },
    },
    sittingFloor: {
        lean: -14,
        arms: { l: [[-26, -30], [-10, -46]], r: [[26, -30], [10, -46]] },
        legs: { l: [[-6, 78], [-4, 74]], r: [[6, 74], [4, 70]] },
    },
    crossLegged: {
        lean: -2,
        arms: { l: [[-30, 18], [-4, 52]], r: [[30, 18], [4, 52]] },
        legs: { l: [[-74, 26], [76, 30]], r: [[74, 26], [-76, 30]] },
    },
    kneeling: {
        lean: 4,
        arms: { l: [[-14, 6], [-8, 10]], r: [[14, 6], [8, 10]] },
        legs: { l: [[4, -10], [6, -92]], r: [[10, 70], [6, 4]] },
    },
    crouching: {
        lean: -14, bend: 12,
        arms: { l: [[-18, 40], [18, 58]], r: [[18, 40], [-18, 58]] },
        legs: { l: [[-18, 58], [-8, -4]], r: [[18, 54], [8, -8]] },
    },
    hugKnees: {
        lean: -8, bend: 10, headNod: 4,
        arms: { l: [[-14, 48], [44, 46]], r: [[14, 48], [-44, 46]] },
        legs: { l: [[-10, 72], [-6, -30]], r: [[10, 70], [6, -32]] },
    },
    reclining: {
        lean: 26, bend: -8, hipTilt: -8, headTilt: -8,
        arms: { l: [[-40, -34], [-24, -56]], r: [[34, 34], [62, 30]] },
        legs: { l: [[26, 52], [-18, -10]], r: [[44, 26], [34, 10]] },
    },

    lying: {
        lean: 88,
        arms: { l: [[86, 10], [88, 12]], r: [[94, 10], [96, 12]] },
        legs: { l: [[94, 6], [92, 4]], r: [[84, 6], [82, 4]] },
    },
    lyingSide: {
        lean: 86, headTilt: 8,
        arms: { l: [[80, -30], [70, -46]], r: [[96, 30], [84, 46]] },
        legs: { l: [[92, -24], [74, -34]], r: [[88, 22], [70, 30]] },
    },
    prone: {
        lean: 86, bend: -12, headTilt: -12, headNod: -12,
        arms: { l: [[70, -40], [26, -30]], r: [[110, -40], [154, -30]] },
        legs: { l: [[92, -10], [96, -40]], r: [[86, -10], [82, -40]] },
    },
    pushUp: {
        lean: 76, bend: -6,
        arms: { l: [[-10, -70], [-8, -72]], r: [[10, -70], [8, -72]] },
        legs: { l: [[92, -8], [90, -8]], r: [[88, -8], [86, -8]] },
    },

    wave: {
        headTilt: -4,
        arms: { l: [[-8, 8], [-6, 10]], r: [[148, 14], [172, 26]] },
        legs: { l: [-4, -2], r: [4, 2] },
    },
    // Pointing at the viewer, not off to the side.
    pointing: {
        headTurn: -8,
        arms: { l: [[-10, 6], [-8, 8]], r: [[56, 62], [48, 78]] },
        legs: { l: [-4, -2], r: [6, 3] },
    },
    thinking: {
        lean: -3, headTilt: 6, headNod: 6,
        arms: { l: [[-26, -4], [64, 30]], r: [[26, 6], [-140, 44]] },
        legs: { l: [-4, -2], r: [4, 2] },
    },
    leaning: {
        lean: -12, twist: 8,
        arms: { l: [[-20, -14], [-10, -18]], r: [[16, 8], [10, 12]] },
        legs: { l: [[-6, 0], [0, 0]], r: [[11, 6], [6, 4]] },
    },
    shrug: {
        headNod: -6,
        arms: { l: [[-40, 12], [-96, 40]], r: [[40, 12], [96, 40]] },
        legs: { l: [-5, -3], r: [5, 3] },
    },
    armsOpen: {
        bend: -4,
        arms: { l: [[-104, 26], [-96, 34]], r: [[104, 26], [96, 34]] },
        legs: { l: [-6, -3], r: [6, 3] },
    },
    lookingBack: {
        lean: 4, twist: 18, headTurn: 62,
        arms: { l: [[-10, -8], [-8, -12]], r: [[10, -8], [8, -12]] },
        legs: { l: [[-6, -8], [-4, -6]], r: [[8, 6], [4, 4]] },
    },
    salute: {
        arms: { l: [[-8, 4], [-6, 6]], r: [[36, 20], [-150, 52]] },
        legs: { l: [-2, -1], r: [2, 1] },
    },
    presenting: {
        lean: 3, headTurn: -14,
        arms: { l: [[-10, 6], [-8, 8]], r: [[86, 30], [78, 40]] },
        legs: { l: [-5, -3], r: [7, 4] },
    },
};

const POSE_PRESETS: Record<PosePresetKey, PresetPoints> = Object.fromEntries(
    (Object.keys(POSE_SPECS) as PosePresetKey[]).map((key) => [key, buildPose(POSE_SPECS[key])]),
) as Record<PosePresetKey, PresetPoints>;

// Grouped the way someone looks for a pose — by what the body is doing, not by
// how the data was authored.
export const POSE_LIBRARY: readonly { group: string; poses: readonly PosePresetKey[] }[] = [
    { group: 'standing', poses: ['standing', 'contrapposto', 'handsOnHips', 'armsCrossed', 'tPose', 'armsUp', 'armsBehind'] },
    { group: 'motion', poses: ['walking', 'running', 'jumping', 'kicking', 'reaching', 'bowing', 'throwing', 'dancing', 'climbing'] },
    { group: 'seated', poses: ['sitting', 'sittingFloor', 'crossLegged', 'kneeling', 'crouching', 'hugKnees', 'reclining'] },
    { group: 'lying', poses: ['lying', 'lyingSide', 'prone', 'pushUp'] },
    { group: 'gesture', poses: ['wave', 'pointing', 'thinking', 'leaning', 'shrug', 'armsOpen', 'lookingBack', 'salute', 'presenting'] },
];

// Where the figure is looked at from. A camera angle is not a pose — the same
// body seen from the side is the same body — so these are a separate control,
// not more entries in the pose grid, and the numbers are shown rather than
// hidden behind names like "dynamic".
export type ViewPresetKey = 'front' | 'threeQuarter' | 'side' | 'back' | 'above' | 'below';

export const VIEW_PRESETS: Record<ViewPresetKey, FigureTurn> = {
    front: { yaw: 0, pitch: 0 },
    threeQuarter: { yaw: 35, pitch: 8 },
    side: { yaw: 82, pitch: 4 },
    back: { yaw: 180, pitch: 0 },
    above: { yaw: 28, pitch: 34 },
    below: { yaw: 28, pitch: -30 },
};

export const VIEW_PRESET_KEYS: readonly ViewPresetKey[] = [
    'front', 'threeQuarter', 'side', 'back', 'above', 'below',
];

// A new figure does not land dead-on. A front elevation is the one angle at
// which a three-dimensional pose looks exactly like the flat one it replaced:
// every limb foreshortened to nothing, no overlap, no volume. A gentle
// three-quarter is what an artist reaches for and what shows, at a glance,
// that this figure can be turned.
export const DEFAULT_VIEW: FigureTurn = { yaw: 22, pitch: 6 };

let figureCounter = 0;

export const createFigure = (
    preset: PosePresetKey,
    dims: CanvasDimensions,
    center?: CanvasPoint,
    shade = 0,
    view: FigureTurn = DEFAULT_VIEW,
): PoseFigure => {
    const height = Math.min(dims.height * FIGURE_HEIGHT_RATIO, dims.width / FIGURE_ASPECT);
    const width = height * FIGURE_ASPECT;
    const cx = center?.x ?? dims.width / 2;
    const cy = center?.y ?? dims.height / 2;
    const points = POSE_PRESETS[preset];
    const joints = {} as Record<JointKey, Vec3>;
    for (const key of JOINT_KEYS) {
        const [nx, ny, nz] = points[key];
        joints[key] = { x: cx + (nx - 0.5) * width, y: cy + (ny - 0.5) * height, z: nz * height };
    }
    figureCounter += 1;
    const figure = setFigureTurn({ id: `figure-${Date.now()}-${figureCounter}`, joints, shade }, view);
    // Centre on the joints, not on the nominal unit box: no preset fills the
    // box exactly (a crown sits below its top edge, a seated figure leans to
    // one side), and every later transform pivots on the joint bounds. Making
    // the two agree here is what lets a pose be swapped in place without the
    // figure drifting.
    return centerFigureAt(figure, { x: cx, y: cy });
};

// --- turning the figure ------------------------------------------------------
//
// The joints stay the single source of truth in world space; `turn` is a memo
// of the rotation already applied to them. Re-deriving the joints from the
// *totals* on every change is what makes the gesture drift-free: composing
// yaw and pitch deltas onto the joints one drag-frame at a time slowly works
// roll into the body, and a manikin leaning out of the picture plane for no
// reason reads as broken rather than as posed.

const unrotateView = (point: Vec3, turn: FigureTurn): Vec3 => {
    const pitch = rad(turn.pitch);
    const cp = Math.cos(pitch);
    const sp = Math.sin(pitch);
    const afterPitch = { x: point.x, y: point.y * cp + zOf(point) * sp, z: zOf(point) * cp - point.y * sp };
    const yaw = rad(turn.yaw);
    const cy = Math.cos(yaw);
    const sy = Math.sin(yaw);
    return {
        x: afterPitch.x * cy - afterPitch.z * sy,
        y: afterPitch.y,
        z: afterPitch.z * cy + afterPitch.x * sy,
    };
};

// Beyond this the figure is seen straight down its own axis and there is
// nothing left to read. Yaw has no such limit — all the way round is a back
// view, which is a legitimate reference.
export const MAX_VIEW_PITCH = 78;

const wrapYaw = (yaw: number): number => {
    const wrapped = ((yaw + 180) % 360 + 360) % 360 - 180;
    return Object.is(wrapped, -0) ? 0 : wrapped;
};

export const setFigureTurn = (figure: PoseFigure, turn: FigureTurn): PoseFigure => {
    const next: FigureTurn = {
        yaw: wrapYaw(turn.yaw),
        pitch: Math.max(-MAX_VIEW_PITCH, Math.min(MAX_VIEW_PITCH, turn.pitch)),
    };
    const current = figureTurn(figure);
    const anchor = projectionOf(figure).anchor;
    const joints = {} as Record<JointKey, Vec3>;
    for (const key of JOINT_KEYS) {
        const body = unrotateView(sub3(figure.joints[key], anchor), current);
        joints[key] = add3(anchor, rotateView(body, next));
    }
    // Turning is a camera move, so the figure has to stay where it was on the
    // canvas: rotating about the joint centre alone slides it, because the
    // projected silhouette is not centred on that point.
    const before = figureCenter(figure);
    return centerFigureAt({ ...figure, joints, turn: next }, before);
};

export const turnFigure = (figure: PoseFigure, deltaYaw: number, deltaPitch: number): PoseFigure => {
    const current = figureTurn(figure);
    return setFigureTurn(figure, { yaw: current.yaw + deltaYaw, pitch: current.pitch + deltaPitch });
};

// Which named view a figure is at, or null when it has been turned by hand.
// The toolbar shows the name when there is one and the two angles when there
// is not, rather than pretending an arbitrary angle is a preset.
export const viewPresetOf = (figure: PoseFigure): ViewPresetKey | null => {
    const turn = figureTurn(figure);
    for (const key of VIEW_PRESET_KEYS) {
        const preset = VIEW_PRESETS[key];
        if (Math.abs(wrapYaw(turn.yaw - preset.yaw)) < 0.5 && Math.abs(turn.pitch - preset.pitch) < 0.5) {
            return key;
        }
    }
    return null;
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
    const centers = existing.map((figure) => figureCenter(figure));
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

// Swaps the pose while keeping the figure where it is, roughly how big it is,
// and which way it is being looked at: re-entry (principle 10) applies inside
// the dialog too, and the camera is not part of the pose (principle 4) — it
// would be a nasty surprise for picking "sitting" to also spin the model round.
export const applyPreset = (figure: PoseFigure, preset: PosePresetKey, dims: CanvasDimensions): PoseFigure => {
    const fresh = createFigure(preset, dims, figureCenter(figure), figure.shade, figureTurn(figure));
    // Matched on `figureUnit`, the body, not on the bounding box: box height
    // changes with the pose (a crouch is shorter than a stand), so matching
    // boxes would resize the person every time the pose changed.
    return { ...scaleFigure(fresh, figureUnit(figure) / figureUnit(fresh)), id: figure.id };
};

// --- the skeleton ------------------------------------------------------------
//
// Joints are not loose points. Every one hangs off a parent, rooted at the
// hip, so dragging a shoulder brings the whole arm with it and a limb cannot
// be stretched into rubber: a drag rotates the bone about its parent — in
// three dimensions — and carries everything below it rigidly, which is what
// the wooden joint does.

export const JOINT_PARENT: Record<JointKey, JointKey | null> = {
    hip: null,
    hipL: 'hip', hipR: 'hip', neck: 'hip',
    kneeL: 'hipL', ankleL: 'kneeL',
    kneeR: 'hipR', ankleR: 'kneeR',
    shoulderL: 'neck', shoulderR: 'neck', head: 'neck',
    elbowL: 'shoulderL', wristL: 'elbowL',
    elbowR: 'shoulderR', wristR: 'elbowR',
    // The detail tier hangs off the core exactly like everything else, which
    // is the point: dragging a wrist takes its hand along, dragging the head
    // takes the face, and no new machinery is needed to pose them.
    face: 'head', handL: 'wristL', handR: 'wristR', toeL: 'ankleL', toeR: 'ankleR',
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

export interface SwingOptions {
    // Send the bone behind the body instead of in front of it. A pointer on
    // screen names a *circle* of 3D positions for the joint, not a point: the
    // same pixel is an arm reaching toward you and an arm reaching away. The
    // drag keeps whichever side the limb is already on, and this is how you
    // say the other one.
    away?: boolean;
}

// Forward kinematics in three dimensions: swing `key` toward the pointer about
// its parent and take its subtree along. The bone keeps its length, so what
// the drag actually chooses is a direction — and where the projected pointer
// falls *inside* the bone's circle, the rest of the bone has to be pointing
// out of the screen. That is foreshortening, and it is produced by dragging
// rather than by a second control.
export const swingJoint = (
    figure: PoseFigure,
    key: JointKey,
    target: CanvasPoint,
    options: SwingOptions = {},
): PoseFigure => {
    const parentKey = JOINT_PARENT[key];
    const projection = projectionOf(figure);
    if (!parentKey) {
        const here = projectPoint(figure.joints[key], projection);
        return translateFigure(figure, target.x - here.x, target.y - here.y);
    }
    const pivot = figure.joints[parentKey];
    const from = figure.joints[key];
    const length = dist3(from, pivot);
    if (length < 1e-6) return figure;

    // The pointer read on the world plane through the parent: the joint has to
    // end up on a sphere about the parent, and that plane cuts it through the
    // middle, where a pixel of movement means the least ambiguity.
    const onPlane = unprojectPoint(target, zOf(pivot), projection);
    let dx = onPlane.x - pivot.x;
    let dy = onPlane.y - pivot.y;
    const planar = Math.hypot(dx, dy);
    if (planar < 1e-6) return figure;
    let dz: number;
    if (planar >= length) {
        // Past the silhouette of the sphere: the bone lies in the picture
        // plane and simply points at the pointer, exactly as it did when the
        // figure was flat.
        const shrink = length / planar;
        dx *= shrink;
        dy *= shrink;
        dz = 0;
    } else {
        const depth = Math.sqrt(Math.max(length * length - planar * planar, 0));
        const side = options.away ? -1 : (Math.sign(zOf(from) - zOf(pivot)) || 1);
        dz = side * depth;
    }

    const before = norm3(sub3(from, pivot));
    const after = norm3({ x: dx, y: dy, z: dz });
    const axisRaw = cross3(before, after);
    const sin = len3(axisRaw);
    const cos = Math.max(-1, Math.min(1, dot3(before, after)));
    // Parallel: nothing to do. Anti-parallel: the rotation axis is undefined,
    // so any perpendicular one will do — the limb is being folded straight
    // back on itself either way.
    let axis: Vec3;
    let angle: number;
    if (sin < 1e-9) {
        if (cos > 0) return figure;
        axis = norm3(Math.abs(before.x) < 0.9
            ? cross3(before, { x: 1, y: 0, z: 0 })
            : cross3(before, { x: 0, y: 0, z: 1 }));
        angle = Math.PI;
    } else {
        axis = mul3(axisRaw, 1 / sin);
        angle = Math.atan2(sin, cos);
    }

    const joints = { ...figure.joints };
    for (const member of SUBTREES[key]) {
        joints[member] = add3(pivot, rotateAxis(sub3(joints[member], pivot), axis, angle));
    }
    return { ...figure, joints };
};

// The escape hatch (Alt): move one joint on its own, at the depth it already
// has. Proportions are the skeleton's job, not a law.
export const moveJoint = (figure: PoseFigure, key: JointKey, point: CanvasPoint): PoseFigure => ({
    ...figure,
    joints: {
        ...figure.joints,
        [key]: unprojectPoint(point, zOf(figure.joints[key]), projectionOf(figure)),
    },
});

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

// Thumbnails: fitted to the tile, but never *inflated* past the size a
// standing figure gets in the same tile. Every thickness scales with the body
// (`figureUnit`), so letting a crouch grow until its small box fills the tile
// makes it 40% thicker than the stand beside it — and a grid where the manikin
// changes build from cell to cell reads as thirty different people rather than
// as one person in thirty poses.
export const fitFigureIntoTile = (figure: PoseFigure, box: CanvasDimensions, pad = 0): PoseFigure => {
    const fitted = fitFigureInto(figure, box, pad);
    const cap = figureUnit(fitFigureInto(createFigure('standing', box, undefined, 0, figureTurn(figure)), box, pad));
    const unit = figureUnit(fitted);
    if (unit <= cap) return fitted;
    const center = figureCenter(fitted);
    return centerFigureAt(scaleFigure(fitted, cap / unit), center);
};

const mapJoints = (figure: PoseFigure, fn: (point: Vec3) => Vec3): PoseFigure => {
    const joints = {} as Record<JointKey, Vec3>;
    for (const key of JOINT_KEYS) joints[key] = fn(figure.joints[key]);
    return { ...figure, joints };
};

export const translateFigure = (figure: PoseFigure, dx: number, dy: number): PoseFigure =>
    mapJoints(figure, (p) => ({ x: p.x + dx, y: p.y + dy, z: zOf(p) }));

// Used when a saved sketch is re-opened on a differently sized canvas. Goes
// through the same uniform transform as the strokes, so the drawing and the
// figures standing in it stay in register. Depth rides the same scale — it is
// measured in canvas pixels like everything else, and leaving it behind would
// flatten a reopened sketch.
// Also the door every foreign figure comes through, so it is where a sketch
// saved before the detail tier gets its five extra joints — even when the
// transform itself is a no-op.
export const transformFigures = (
    figures: readonly PoseFigure[],
    transform: CanvasTransform,
): PoseFigure[] => figures.map((figure) => {
    const whole = completeFigure(figure);
    return isIdentityTransform(transform) ? whole : mapJoints(whole, (p) => ({
        ...applyTransform(p, transform),
        z: zOf(p) * transform.scale,
    }));
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

// Uniform scale about the figure's own centre, depth included: scaling only
// x and y would flatten the figure as it shrank and stretch it as it grew.
export const scaleFigure = (figure: PoseFigure, factor: number, origin?: CanvasPoint): PoseFigure => {
    const pivot = origin ?? figureCenter(figure);
    const factorApplied = safeFactor(factor);
    return mapJoints(figure, (p) => ({
        x: pivot.x + (p.x - pivot.x) * factorApplied,
        y: pivot.y + (p.y - pivot.y) * factorApplied,
        z: zOf(p) * factorApplied,
    }));
};

// Mirroring the coordinates is enough: the bone list is symmetric, so no
// left/right relabelling is needed for the figure to render correctly. The
// recorded view flips with it — mirroring a figure turned 35° to its left
// leaves it turned 35° to its right, and the toolbar has to say so.
export const flipFigure = (figure: PoseFigure): PoseFigure => {
    const bounds = figureBounds(figure);
    const axis = bounds.x + bounds.width / 2;
    const turn = figureTurn(figure);
    const mirrored = mapJoints(figure, (p) => ({ x: axis * 2 - p.x, y: p.y, z: zOf(p) }));
    return { ...mirrored, turn: { yaw: wrapYaw(-turn.yaw), pitch: turn.pitch } };
};

// --- hit testing -------------------------------------------------------------

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

// Joints are grabbed where they are drawn — the projected position — and the
// nearest one to the camera wins a tie, which is the one the pointer is
// actually over.
export const hitTestJoint = (figure: PoseFigure, point: CanvasPoint, radius: number): JointKey | null => {
    const projected = projectFigure(figure);
    let best: JointKey | null = null;
    let bestDistance = radius;
    let bestDepth = -Infinity;
    // Only the joints this figure is showing. Grabbing a detail joint you
    // cannot see — and that sits right on top of a wrist or a head — would
    // make the simple tier quietly harder to use than no tier at all.
    for (const key of jointKeysOf(figure)) {
        const joint = projected[key];
        const distance = Math.hypot(point.x - joint.x, point.y - joint.y);
        if (distance > bestDistance) continue;
        if (distance < bestDistance - 1e-9 || joint.depth > bestDepth) {
            best = key;
            bestDistance = distance;
            bestDepth = joint.depth;
        }
    }
    return best;
};

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

// Bottom-left, opposite the scale grip: turning the figure is the other thing
// you do to the whole body, and it gets the same kind of control rather than a
// mode to enter. Drag sideways to spin the body, up and down to raise or drop
// the camera.
export const turnHandlePoint = (figure: PoseFigure): CanvasPoint => {
    const bounds = figureVisualBounds(figure);
    return { x: bounds.x, y: bounds.y + bounds.height };
};

export const isTurnHandleHit = (figure: PoseFigure, point: CanvasPoint, radius: number): boolean => {
    const handle = turnHandlePoint(figure);
    return Math.hypot(point.x - handle.x, point.y - handle.y) <= radius;
};

// How far the body turns per pixel dragged. A full turn in rather less than a
// canvas width: the useful range is the first 45°, and a slow handle makes
// finding it a chore.
export const TURN_DEGREES_PER_PIXEL = 0.55;

// --- the wooden manikin ------------------------------------------------------
//
// Everything below is derived from the fifteen joints; there are no extra
// handles. The shape follows an artist's wooden manikin rather than a flat
// pictogram: a peg neck under an egg head, a chest and a pelvis as two
// separate volumes joined at the waist, visible ball joints, and tapered limb
// segments. That is not decoration — the chest takes its angle from the
// shoulder line and the pelvis from the hip line, so dragging one shoulder
// twists the torso and the figure reads as having a front and a back.
//
// Every shape here is already projected: its position is where it lands on
// screen, its girth is scaled by how near it is to the camera, and its extent
// along the body is the *projected* distance, which is what makes a thigh
// pointing at the viewer come out short and fat instead of long and thin.

export interface Ellipse { center: CanvasPoint; radiusX: number; radiusY: number; angle: number }
export interface Segment { from: CanvasPoint; to: CanvasPoint; fromRadius: number; toRadius: number }
export interface Ball { center: CanvasPoint; radius: number }

export type ShapeTone = 'body' | 'joint' | 'head';

// A drawable with the depth it is at, so the renderer can sort. Parts are
// grouped rather than sorted one by one: a body is a handful of solids that
// overlap each other, and a chest, its waist ball and its pelvis have a
// drawing order that comes from anatomy, not from depth.
// `tint` lightens a shape above its tone without adding a fourth colour to
// every shade table — used for the one surface that is not a volume, the flat
// plane of the face.
export type FigureShape =
    | { kind: 'ellipse'; ellipse: Ellipse; tone: ShapeTone; depth: number; tint?: number }
    | { kind: 'segment'; segment: Segment; tone: ShapeTone; depth: number; tint?: number }
    | { kind: 'ball'; ball: Ball; tone: ShapeTone; depth: number; tint?: number };

export interface FigureCluster {
    key: 'torso' | 'head' | 'armL' | 'armR' | 'legL' | 'legR';
    depth: number;
    // Fixed for the torso, where the order encodes anatomy (hip balls under
    // the pelvis); by depth inside a limb, where a fold really does put the
    // forearm in front of the upper arm.
    order: 'authored' | 'depth';
    shapes: FigureShape[];
}

export interface FigureParts {
    head: Ellipse;
    // The flat plane of the face, or null when the figure is looking away.
    // A wooden manikin has no features, and without this a back view is
    // pixel-for-pixel a front view — which makes "turn it round" a control
    // that does nothing a model could ever read.
    face: Ellipse | null;
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
    // The same shapes again, grouped and carrying their depth: hit testing
    // wants them by name, the renderer wants them in order. One geometry,
    // two views of it — never two tables.
    clusters: FigureCluster[];
}

const lerp3 = (a: Vec3, b: Vec3, t: number): Vec3 => ({
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
    z: zOf(a) + (zOf(b) - zOf(a)) * t,
});
const mid3 = (a: Vec3, b: Vec3): Vec3 => lerp3(a, b, 0.5);
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
    // A body is not a cut-out: seen from the side the chest and pelvis are as
    // deep as they are wide, so their on-screen width can never fall below
    // this however far the shoulders foreshorten.
    chestDepth: 0.070, pelvisDepth: 0.062,
} as const;

export const figureParts = (figure: PoseFigure): FigureParts => {
    const joints = figure.joints;
    const projection = projectionOf(figure);
    const at = projectFigure(figure);
    const unit = figureUnit(figure);
    // Girth scales with nearness; length is whatever the projection says.
    const u = (ratio: number, scale: number) => unit * ratio * scale;
    const place = (point: Vec3) => projectPoint(point, projection);

    const shoulderMid = place(mid3(joints.shoulderL, joints.shoulderR));
    const hipMid = place(mid3(joints.hipL, joints.hipR));
    const shoulderSpan = spanOf(at.shoulderL, at.shoulderR);
    const hipSpan = spanOf(at.hipL, at.hipR);
    const torso = Math.max(spanOf(at.neck, hipMid), 1);
    const torsoScale = (at.neck.scale + hipMid.scale) / 2;

    // Chest and pelvis each take their own angle: the chest from the shoulder
    // line, the pelvis from the hip line. Twist one and only that volume turns.
    const chestCenter = place(lerp3(mid3(joints.shoulderL, joints.shoulderR), mid3(joints.hipL, joints.hipR), 0.26));
    // The pelvis straddles the hip line and the waist ball bridges it to the
    // chest, the way the two turned halves of a manikin meet at their pin.
    const pelvisCenter = place(lerp3(joints.neck, mid3(joints.hipL, joints.hipR), 0.96));
    const waistCenter = place(lerp3(joints.neck, mid3(joints.hipL, joints.hipR), 0.72));

    const limbSpec = [
        ['shoulderL', 'elbowL', R.upperArm, R.elbow],
        ['shoulderR', 'elbowR', R.upperArm, R.elbow],
        ['elbowL', 'wristL', R.foreArm, R.wrist],
        ['elbowR', 'wristR', R.foreArm, R.wrist],
        ['hipL', 'kneeL', R.thigh, R.knee],
        ['hipR', 'kneeR', R.thigh, R.knee],
        ['kneeL', 'ankleL', R.shin, R.ankle],
        ['kneeR', 'ankleR', R.shin, R.ankle],
    ] as const;

    const limbs: Segment[] = limbSpec.map(([from, to, fromRatio, toRatio]) => ({
        from: at[from],
        to: at[to],
        fromRadius: u(fromRatio, at[from].scale),
        toRadius: u(toRatio, at[to].scale),
    }));

    const ballSpec = [
        ['shoulderL', R.shoulder], ['shoulderR', R.shoulder],
        ['elbowL', R.elbow], ['elbowR', R.elbow],
        ['kneeL', R.knee], ['kneeR', R.knee],
        ['ankleL', R.ankle], ['ankleR', R.ankle],
    ] as const;
    const balls: Ball[] = ballSpec.map(([key, ratio]) => ({
        center: at[key],
        radius: u(ratio, at[key].scale),
    }));

    // Hands, feet and the plane of the face are no longer worked out here.
    // They hang off `handL/R`, `toeL/R` and `face`, which the preset builder
    // fills with exactly these directions — so a figure nobody has touched
    // draws identically, and one whose toes have been turned draws the turn.
    const hands: Ellipse[] = ([['wristL', 'handL'], ['wristR', 'handR']] as const).map(([wrist, hand]) => {
        const scale = at[wrist].scale;
        return {
            center: place(lerp3(joints[wrist], joints[hand], 0.55)),
            radiusX: u(R.handLong, scale),
            radiusY: u(R.handWide, scale),
            angle: angleOf(at[wrist], at[hand]),
        };
    });

    // A foot points where its toes point. The projected length is the
    // foreshortening: a foot pointing at the camera is a short foot, and it
    // has to be, or a figure walking toward you grows skis.
    const feet: Ellipse[] = ([['ankleL', 'toeL'], ['ankleR', 'toeR']] as const).map(([ankle, toe]) => {
        const scale = at[ankle].scale;
        const center = place(lerp3(joints[ankle], joints[toe], 0.55));
        const reach = Math.max(spanOf(at[ankle], center), u(R.footWide, scale) * 0.7);
        return {
            center,
            radiusX: reach + u(R.footWide, scale) * 0.5,
            radiusY: u(R.footWide, scale),
            angle: angleOf(at[ankle], center),
        };
    });

    // The facial plane sits on the front of the skull and is simply absent
    // once it has turned away. It is lighter rather than darker: the light is
    // in front, so the flat of the face is the part of the head that catches
    // it — and a dark patch on a head reads as a mask, which is not what we
    // want a model to paint.
    const facing = norm3(sub3(joints.face, joints.head));
    const facingCamera = zOf(facing);
    const face: Ellipse | null = facingCamera > 0.06 ? (() => {
        const center = place(add3(joints.head, mul3(facing, unit * R.headWide * 0.42)));
        return {
            center,
            radiusX: u(R.headWide, center.scale) * 0.66 * facingCamera,
            radiusY: u(R.headLong, center.scale) * 0.62,
            angle: angleOf(at.neck, at.head) - Math.PI / 2,
        };
    })() : null;

    // Drawn under the chest so the figure never comes apart: the chest and
    // pelvis rotate on their own axes, and a hard shoulder drag would
    // otherwise swing the ribcage out from under the neck and leave a gap.
    const spine: Segment = {
        from: at.neck,
        to: hipMid,
        fromRadius: u(0.045, at.neck.scale),
        toRadius: u(0.055, hipMid.scale),
    };
    const head: Ellipse = {
        center: at.head,
        radiusX: u(R.headWide, at.head.scale),
        radiusY: u(R.headLong, at.head.scale),
        angle: angleOf(at.neck, at.head) - Math.PI / 2,
    };
    const neck: Segment = {
        from: at.neck,
        to: at.head,
        fromRadius: u(R.neck, at.neck.scale),
        toRadius: u(R.neck, at.head.scale),
    };
    const chest: Ellipse = {
        center: chestCenter,
        // Transverse axis on the shoulder line, long axis down the torso:
        // drag one shoulder and the chest rotates with it, while the pelvis
        // keeps the hip line's own angle. That difference is the twist. The
        // floor is the body's own depth, so turning to the side narrows the
        // ribcage to a torso seen edge-on and no further.
        radiusX: Math.max(shoulderSpan * 0.46, u(R.chestDepth, chestCenter.scale)),
        // Floored for the same reason as the width: a torso seen end-on is a
        // short torso, not a flat one.
        radiusY: Math.max(torso * 0.33, u(0.052, chestCenter.scale)),
        angle: angleOf(at.shoulderL, at.shoulderR),
    };
    const pelvis: Ellipse = {
        // Wide enough to reach past the hip balls and short enough to sit
        // between them: a narrower or taller ellipse hangs below the hips
        // as a droplet instead of reading as a pelvis.
        center: pelvisCenter,
        radiusX: Math.max(hipSpan * 0.5 + u(R.hipBall, pelvisCenter.scale) * 0.9, u(R.pelvisDepth, pelvisCenter.scale)),
        radiusY: Math.max(torso * 0.16, u(0.030, pelvisCenter.scale)),
        angle: angleOf(at.hipL, at.hipR),
    };
    const waist: Ball = { center: waistCenter, radius: torso * 0.085 };
    // Kept apart from the rest because they are drawn *under* the pelvis:
    // a hip ball on top of the block reads as a buttock, while one behind
    // it shows only where the thigh comes out, which is what the wooden
    // joint actually looks like.
    const hipBalls: Ball[] = [
        { center: at.hipL, radius: u(R.hipBall, at.hipL.scale) },
        { center: at.hipR, radius: u(R.hipBall, at.hipR.scale) },
    ];

    const mean = (...depths: number[]) => depths.reduce((sum, d) => sum + d, 0) / depths.length;

    const limbCluster = (
        key: FigureCluster['key'],
        ball: Ball,
        upper: Segment,
        joint: Ball,
        lower: Segment,
        end: Ball | null,
        tip: Ellipse,
        depths: readonly [number, number, number],
    ): FigureCluster => ({
        key,
        order: 'depth',
        depth: mean(...depths),
        shapes: [
            { kind: 'segment', segment: upper, tone: 'body', depth: mean(depths[0], depths[1]) },
            { kind: 'segment', segment: lower, tone: 'body', depth: mean(depths[1], depths[2]) },
            { kind: 'ellipse', ellipse: tip, tone: 'body', depth: depths[2] },
            { kind: 'ball', ball, tone: 'joint', depth: depths[0] },
            { kind: 'ball', ball: joint, tone: 'joint', depth: depths[1] },
            ...(end ? [{ kind: 'ball' as const, ball: end, tone: 'joint' as const, depth: depths[2] }] : []),
        ],
    });

    const clusters: FigureCluster[] = [
        {
            key: 'torso',
            order: 'authored',
            depth: mean(at.neck.depth, hipMid.depth),
            shapes: [
                { kind: 'segment', segment: spine, tone: 'body', depth: mean(at.neck.depth, hipMid.depth) },
                { kind: 'ball', ball: hipBalls[0], tone: 'joint', depth: at.hipL.depth },
                { kind: 'ball', ball: hipBalls[1], tone: 'joint', depth: at.hipR.depth },
                { kind: 'ellipse', ellipse: chest, tone: 'body', depth: chestCenter.depth },
                { kind: 'ellipse', ellipse: pelvis, tone: 'body', depth: pelvisCenter.depth },
                { kind: 'ball', ball: waist, tone: 'joint', depth: waistCenter.depth },
            ],
        },
        {
            key: 'head',
            order: 'authored',
            depth: at.head.depth,
            shapes: [
                { kind: 'segment', segment: neck, tone: 'body', depth: mean(at.neck.depth, at.head.depth) },
                { kind: 'ellipse', ellipse: head, tone: 'head', depth: at.head.depth },
                ...(face ? [{ kind: 'ellipse' as const, ellipse: face, tone: 'head' as const, tint: 0.34, depth: at.head.depth }] : []),
            ],
        },
        limbCluster('armL', balls[0], limbs[0], balls[2], limbs[2], null, hands[0],
            [at.shoulderL.depth, at.elbowL.depth, at.wristL.depth]),
        limbCluster('armR', balls[1], limbs[1], balls[3], limbs[3], null, hands[1],
            [at.shoulderR.depth, at.elbowR.depth, at.wristR.depth]),
        limbCluster('legL', hipBalls[0], limbs[4], balls[4], limbs[6], balls[6], feet[0],
            [at.hipL.depth, at.kneeL.depth, at.ankleL.depth]),
        limbCluster('legR', hipBalls[1], limbs[5], balls[5], limbs[7], balls[7], feet[1],
            [at.hipR.depth, at.kneeR.depth, at.ankleR.depth]),
    ];

    return { head, face, neck, spine, chest, waist, pelvis, limbs, balls, hipBalls, hands, feet, clusters };
};

// --- drawing -----------------------------------------------------------------
//
// Three tones per figure, no wood: the body, the joint balls a step darker so
// the articulation reads, and the head a step lighter so it does not merge into
// the chest. Neutral grey — the manikin's structure is the message, and a wood
// colour only invites the model to paint a wooden doll.
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

// One fixed light, up and to the left and in front of the figure — the studio
// default, and the one every artist's reference photo already uses. The first
// version deliberately had none: flat colour, structure only. That was right
// for a flat figure, where a gradient could only have been decoration. It is
// wrong for this one. On a body that has depth, the shading *is* structure:
// which way a volume turns is exactly what the light says and what the
// silhouette cannot.
const LIGHT = { x: -0.42, y: -0.72, z: 0.55 };
const LIGHT_SCREEN = (() => {
    const length = Math.hypot(LIGHT.x, LIGHT.y);
    return { x: LIGHT.x / length, y: LIGHT.y / length };
})();
const SHADOW_TINT = '#4d535b';
const HIGHLIGHT_TINT = '#ffffff';

// Accepts both spellings this module produces: the authored `#rrggbb` tones
// and the `rgb()` a previous mix returned, so shading steps can be composed.
const parseColor = (color: string): readonly [number, number, number] => {
    if (color.startsWith('#')) {
        return [
            parseInt(color.slice(1, 3), 16),
            parseInt(color.slice(3, 5), 16),
            parseInt(color.slice(5, 7), 16),
        ];
    }
    const parts = color.replace(/[^0-9.,-]/g, '').split(',');
    return [Number(parts[0]) || 0, Number(parts[1]) || 0, Number(parts[2]) || 0];
};

const mixColor = (from: string, to: string, amount: number): string => {
    const a = parseColor(from);
    const b = parseColor(to);
    const t = Math.max(0, Math.min(1, amount));
    const channel = (i: number) => Math.round(a[i] + (b[i] - a[i]) * t);
    return `rgb(${channel(0)}, ${channel(1)}, ${channel(2)})`;
};

// Aerial perspective, in miniature: the far side of a body goes a touch
// darker, the near side a touch lighter. Small enough that nobody reads it as
// a colour, big enough that two crossed limbs are never ambiguous.
const DEPTH_TINT = 0.11;

const shadeOfShape = (base: string, depth: number, unit: number): string => {
    const t = Math.max(-1, Math.min(1, depth / Math.max(unit * 0.5, 1)));
    return t >= 0
        ? mixColor(base, HIGHLIGHT_TINT, t * DEPTH_TINT)
        : mixColor(base, SHADOW_TINT, -t * DEPTH_TINT);
};

const LIT_AMOUNT = 0.30;
const DARK_AMOUNT = 0.32;

const shapeFill = (
    ctx: CanvasRenderingContext2D,
    shape: FigureShape,
    tone: FigureTone,
    unit: number,
): string | CanvasGradient => {
    const hex = shape.tint ? mixColor(tone[shape.tone], HIGHLIGHT_TINT, shape.tint) : tone[shape.tone];
    const base = shadeOfShape(hex, shape.depth, unit);
    // Lit and dark are mixed from the untinted hex — the depth-tinted base is
    // already an `rgb()` string, which `mixColor` does not parse — and then
    // carried to the same depth so all three stops agree.
    const litColor = shadeOfShape(mixColor(hex, HIGHLIGHT_TINT, LIT_AMOUNT), shape.depth, unit);
    const darkColor = shadeOfShape(mixColor(hex, SHADOW_TINT, DARK_AMOUNT), shape.depth, unit);

    if (shape.kind === 'segment') {
        const { from, to, fromRadius, toRadius } = shape.segment;
        const dx = to.x - from.x;
        const dy = to.y - from.y;
        const length = Math.hypot(dx, dy);
        const radius = Math.max(fromRadius, toRadius);
        if (length < 1e-6 || radius < 0.5) return base;
        // Across the limb, never along it: a cylinder is lit on one side and
        // dark on the other, and a gradient running down its length would
        // read as the limb fading out.
        let px = -dy / length;
        let py = dx / length;
        if (px * LIGHT_SCREEN.x + py * LIGHT_SCREEN.y < 0) { px = -px; py = -py; }
        const cx = (from.x + to.x) / 2;
        const cy = (from.y + to.y) / 2;
        const gradient = ctx.createLinearGradient(
            cx + px * radius, cy + py * radius,
            cx - px * radius, cy - py * radius,
        );
        gradient.addColorStop(0, litColor);
        gradient.addColorStop(0.46, base);
        gradient.addColorStop(1, darkColor);
        return gradient;
    }

    const center = shape.kind === 'ball' ? shape.ball.center : shape.ellipse.center;
    const radius = shape.kind === 'ball'
        ? shape.ball.radius
        : Math.max(shape.ellipse.radiusX, shape.ellipse.radiusY);
    if (radius < 0.5) return base;
    const gradient = ctx.createRadialGradient(
        center.x + LIGHT_SCREEN.x * radius * 0.45,
        center.y + LIGHT_SCREEN.y * radius * 0.45,
        radius * 0.05,
        center.x + LIGHT_SCREEN.x * radius * 0.1,
        center.y + LIGHT_SCREEN.y * radius * 0.1,
        radius * 1.3,
    );
    gradient.addColorStop(0, litColor);
    gradient.addColorStop(0.42, base);
    gradient.addColorStop(1, darkColor);
    return gradient;
};

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

const fillBall = (ctx: CanvasRenderingContext2D, ball: Ball, grow = 0): void => {
    ctx.beginPath();
    ctx.arc(ball.center.x, ball.center.y, Math.max(ball.radius + grow, 0.5), 0, Math.PI * 2);
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

const fillShape = (ctx: CanvasRenderingContext2D, shape: FigureShape, grow = 0): void => {
    if (shape.kind === 'segment') fillSegment(ctx, shape.segment, grow);
    else if (shape.kind === 'ball') fillBall(ctx, shape.ball, grow);
    else fillEllipse(ctx, shape.ellipse, grow);
};

const RIM_RATIO = 0.006;

// Back to front, one body part at a time. The rim is drawn per group rather
// than once for the whole figure, and that is the change depth forced: a
// single flat rim pass welds an arm crossing the chest into the chest, which
// is the one thing the near arm has to not do. Inside a group the rim is still
// one flat union, so a limb has no seam where its own segments meet.
export const drawFigure = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    options: { selected?: boolean } = {},
): void => {
    const parts = figureParts(figure);
    const tone = toneFor(figure, options.selected === true);
    const unit = figureUnit(figure);
    const rim = Math.max(unit * RIM_RATIO, 1);
    ctx.save();
    ctx.globalCompositeOperation = 'source-over';

    const clusters = [...parts.clusters].sort((a, b) => a.depth - b.depth);
    for (const cluster of clusters) {
        ctx.fillStyle = tone.rim;
        for (const shape of cluster.shapes) fillShape(ctx, shape, rim);
        const ordered = cluster.order === 'depth'
            ? [...cluster.shapes].sort((a, b) => a.depth - b.depth)
            : cluster.shapes;
        for (const shape of ordered) {
            ctx.fillStyle = shapeFill(ctx, shape, tone, unit);
            fillShape(ctx, shape);
        }
    }
    ctx.restore();
};

// Handles are overlay-only: they are drawn on the interaction layer, never on
// the surface that is exported, so the model never sees the blue dots.
export const drawFigureHandles = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    handleRadius: number,
): void => {
    const projected = projectFigure(figure);
    ctx.save();
    ctx.globalCompositeOperation = 'source-over';
    ctx.lineWidth = Math.max(1, handleRadius * 0.35);
    // Nearer joints get bigger dots. It costs nothing and it means the depth
    // of a pose is legible from the handles alone, before anything is dragged.
    const keys = jointKeysOf(figure);
    const ordered = [...keys].sort((a, b) => projected[a].depth - projected[b].depth);
    for (const key of ordered) {
        const joint = projected[key];
        const detail = isDetailJoint(key);
        // Detail handles are smaller and hollow. They are secondary by
        // construction — a body is posed at the shoulders and hips, and the
        // face and toes are a second pass — so they must not compete with the
        // fifteen that matter for reading the pose (principle 9).
        const radius = handleRadius * (detail ? 0.62 : 1) * Math.max(0.7, Math.min(1.4, joint.scale));
        ctx.fillStyle = detail ? HANDLE_STROKE : HANDLE_FILL;
        ctx.strokeStyle = detail ? HANDLE_FILL : HANDLE_STROKE;
        ctx.beginPath();
        ctx.arc(joint.x, joint.y, radius, 0, Math.PI * 2);
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
    // The turn grip is a ring, not a square: a different gesture deserves a
    // different shape, and a ring is what "spin this" looks like everywhere.
    const turn = turnHandlePoint(figure);
    ctx.beginPath();
    ctx.arc(turn.x, turn.y, handleRadius, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(turn.x, turn.y, handleRadius * 0.4, 0, Math.PI * 2);
    ctx.strokeStyle = HANDLE_FILL;
    ctx.stroke();
    ctx.restore();
};
