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
    // Where the face looks. See below.
    | 'face';

// Sixteen joints. Fifteen of them are the body; the sixteenth is the face.
//
// The face earns its place because it is the one thing the body genuinely
// cannot imply: everything else — which way a hand points, which way a foot
// points — follows from the limb it hangs off closely enough that nobody would
// pose it twice, but a head can be looking anywhere at all, and "looking over
// the shoulder" is a pose, not a detail. There was briefly a tier with hands
// and toes in it too; it was cut. A joint nobody would ever drag is a handle
// with no purpose, and five of them are noise (principle 9).
export const JOINT_KEYS: readonly JointKey[] = [
    'head', 'neck', 'shoulderL', 'shoulderR', 'elbowL', 'elbowR', 'wristL', 'wristR',
    'hip', 'hipL', 'hipR', 'kneeL', 'kneeR', 'ankleL', 'ankleR',
    'face',
];

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
}

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
// Width of the unit box relative to its height. Arms out to the side need more
// room than a body is wide — and the box has to hold the *drawn* figure, not
// just its joints: the deltoid and the shoulder ball both sit outboard of the
// shoulder joint, so a pose with its arms straight up is wider than its
// skeleton. Changing this only changes how much room is reserved; bone lengths
// are unaffected, because the same factor divides out when a preset is
// normalised (see `POSE_SCALE * FIGURE_ASPECT` below).
const FIGURE_ASPECT = 0.48;

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
    // Wide enough that the shoulder joint sits *on* the deltoid corner and the
    // hip joint on the pelvis's lower corner. Tucked inside the body instead,
    // a limb reads as hanging off a shelf, and every raised arm cuts a notch.
    shoulderSpan: 0.115, shoulderDrop: 0.035,
    upperArm: 0.155, foreArm: 0.145,
    hipSpan: 0.070, hipDrop: 0.022,
    thigh: 0.235, shin: 0.225,
    // Far enough in front of the skull that the handle clears the head's.
    face: 0.075,
} as const;

const ORIGIN: Vec3 = { x: 0, y: 0, z: 0 };

// How far the neck leans out of the torso's axis, toward the body's front.
const NECK_FORWARD = 7;

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

// --- which way things point --------------------------------------------------

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

// Where the face points when nobody has said: square to the skull, facing the
// way the body faces. Used by the preset builder (so all thirty-six poses get
// a face without declaring one) and by `completeFigure`.
export const derivedFace = (joints: Record<JointKey, Vec3>, unit: number): Vec3 => add3(
    joints.head,
    mul3(squareTo(bodyForwardOf(joints), norm3(sub3(joints.head, joints.neck))), unit * BONE.face),
);

// A figure saved before the face joint existed — or one built by hand — gets
// it put where the body implies. Cheap, idempotent, and the reason nothing
// downstream has to cope with a missing joint.
export const completeFigure = (figure: PoseFigure): PoseFigure => {
    // Reopened sketches go through the rig too. A sketch saved before the rig
    // existed can hold a pose the model no longer allows, and the honest thing
    // is to bring it into range on open rather than to keep a second, laxer
    // set of rules alive for old data.
    if (figure.joints.face !== undefined) return constrainFigure(figure);
    const joints = { ...figure.joints, face: derivedFace(figure.joints, figureUnit(figure)) };
    return constrainFigure({ ...figure, joints });
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
    // The neck is a cylinder that angles forward, not a vertical peg — one of
    // the few things the art-school construction is explicit about. Applied
    // here rather than in every pose spec, so it costs no preset a line.
    raw.head = add3(neck, rotateAxis(
        sub3(raw.head, neck),
        norm3(cross3(sub3(raw.head, neck), bodyForwardOf(raw))),
        rad(-NECK_FORWARD),
    ));
    // The head's turn is applied last: it rotates the skull about the torso's
    // axis without moving anything else, which is what "looking over your
    // shoulder" is.
    if (spec.headTurn) {
        raw.head = add3(neck, rotateAxis(sub3(raw.head, neck), axis, rad(spec.headTurn)));
    }
    // The face is derived, never declared — which is what let it be added
    // without touching a single one of the thirty-six pose specs. `headTurn`
    // still turns the skull; the face follows it.
    raw.face = derivedFace(raw, BONE.torso / TORSO_HEIGHT_RATIO);

    // Into the unit box, at one scale shared by every pose — deliberately not
    // "stretch each pose to fill the box". Same bones, same body: a crouching
    // figure is genuinely shorter than a standing one, and swapping poses
    // never resizes the person.
    // Measured over the body, not the face: a figure's size is its body, and
    // letting the face marker push the unit box out would make every pose a
    // little smaller for no reason anyone could see.
    const body = JOINT_KEYS.filter((key) => key !== 'face');
    const xs = body.map((key) => raw[key].x);
    const ys = body.map((key) => raw[key].y);
    const minY = Math.min(...ys);
    const midX = (Math.min(...xs) + Math.max(...xs)) / 2;
    const zs = body.map((key) => zOf(raw[key]));
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
    // Through the rig like everything else: a preset is a starting point
    // written by hand, and a hand writing forty-five angles gets some of them
    // wrong. Legalising here means the library cannot drift out of what the
    // model allows, and a new preset cannot be authored impossible.
    const figure = constrainFigure(
        setFigureTurn({ id: `figure-${Date.now()}-${figureCounter}`, joints, shade }, view),
    );
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
    // An ordinary child, so dragging the head takes the face with it and no
    // new machinery is needed to aim it.
    face: 'head',
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
    // The drag proposes, the rig disposes. Cheap enough to run on every frame,
    // and running it here is what makes an impossible pose unreachable rather
    // than merely unwritten.
    return constrainFigure({ ...figure, joints });
};

// --- the rig -----------------------------------------------------------------
//
// Until this existed the mannequin had no idea what any of its joints *were*.
// A pose was a table of absolute angles and a drag was a free rotation in three
// dimensions, so nothing anywhere could tell an elbow that it is a hinge. Both
// the authored library and the user could therefore produce a body no body can
// make, and that — not the drawing — is why a handful of poses read as wrong
// however carefully they were shaded.
//
// The rig is enforced rather than stored: `constrainFigure` takes any set of
// joints and returns the nearest legal one. That keeps the saved format a plain
// list of points (so every sketch ever saved still opens) while making the
// model, not the data, the authority on what a body can do. Every way a pose
// can enter — a preset, a drag, a reopened sketch, a photograph — goes through
// it, so there is no path that can smuggle an impossible pose in.

interface BodyFrame { up: Vec3; across: Vec3 }

// The body's own axes. `across` is the hinge reference: a knee and an elbow
// both swing in a plane whose normal lies along it.
const frameOf = (joints: Record<JointKey, Vec3>, left: JointKey, right: JointKey): BodyFrame | null => {
    const up = norm3(sub3(joints.neck, joints.hip));
    const span = sub3(joints[right], joints[left]);
    if (len3(up) < 1e-6 || len3(span) < 1e-6) return null;
    // Orthogonalised against `up` so a shrugged shoulder or a cocked pelvis
    // does not tilt the plane a hinge is allowed to swing in.
    const across = norm3(sub3(span, mul3(up, dot3(span, up))));
    if (len3(across) < 1e-6) return null;
    return { up, across };
};

// A hinge: one bone swinging against another in a single plane, on one side
// only. `side` is the sign of the bend normal along `across`, and it is the
// whole difference between a knee and an elbow — they bend opposite ways.
//
// Both signs were taken from poses that are known-good by eye rather than
// derived: the canvas y axis points *down*, which quietly flips the handedness
// of every cross product, and a hand-derived sign for this was wrong the first
// time. `sitting` is the reference for the knee and `handsOnHips` for the
// elbow; if either sign is ever in doubt, check it against those two rather
// than against a diagram.
interface Hinge {
    joint: JointKey;
    root: JointKey;
    tip: JointKey;
    side: JointKey;          // the frame's left marker
    other: JointKey;         // ...and its right one
    maxBend: number;         // degrees
    normalSign: number;
    // How far the parent bone may twist about its own axis, which is what
    // rotates the plane the hinge swings in. Generous for an arm — the humerus
    // really does rotate that far — and tight for a leg, where a femur turned
    // ninety degrees is the thing that made a cross-legged knee fold sideways.
    maxTwist: number;        // degrees
}

const HINGES: readonly Hinge[] = [
    { joint: 'elbowL', root: 'shoulderL', tip: 'wristL', side: 'shoulderL', other: 'shoulderR', maxBend: 148, normalSign: 1, maxTwist: 85 },
    { joint: 'elbowR', root: 'shoulderR', tip: 'wristR', side: 'shoulderL', other: 'shoulderR', maxBend: 148, normalSign: 1, maxTwist: 85 },
    { joint: 'kneeL', root: 'hipL', tip: 'ankleL', side: 'hipL', other: 'hipR', maxBend: 150, normalSign: -1, maxTwist: 45 },
    { joint: 'kneeR', root: 'hipR', tip: 'ankleR', side: 'hipL', other: 'hipR', maxBend: 150, normalSign: -1, maxTwist: 45 },
];

// Below seven degrees of bend a limb is straight enough that its bend plane is
// numerical noise, and constraining it would only jitter a straight arm. Note
// the sense: this is a cosine, so *larger* means *straighter*.
const HINGE_DEADBAND = Math.cos(7 * Math.PI / 180);

const DEG = Math.PI / 180;

// A ball joint: how far a bone may swing from the body's "down", and how far
// it may cross the midline. Deliberately loose — a shoulder really does reach
// almost anywhere, and the job here is to stop a limb folding through the
// torso, not to police choreography.
interface Cone {
    joint: JointKey;
    root: JointKey;
    side: JointKey;
    other: JointKey;
    maxFromDown: number;     // degrees away from the body's own "straight down"
    maxCrossing: number;     // degrees past the midline, toward the other side
}

const CONES: readonly Cone[] = [
    { joint: 'kneeL', root: 'hipL', side: 'hipL', other: 'hipR', maxFromDown: 135, maxCrossing: 30 },
    { joint: 'kneeR', root: 'hipR', side: 'hipL', other: 'hipR', maxFromDown: 135, maxCrossing: 30 },
];

// Swing `direction` about `axis` until its component along `toward` is `want`,
// taking the nearer of the two solutions. `direction` must be perpendicular to
// `axis`; the reachable range is the length of `toward`'s own perpendicular
// part, so `want` is clamped into it first.
const swingAbout = (direction: Vec3, axis: Vec3, toward: Vec3, want: number): Vec3 => {
    const flat = sub3(toward, mul3(axis, dot3(toward, axis)));
    const reach = len3(flat);
    if (reach < 1e-6) return direction;
    const basis = norm3(flat);
    const side = cross3(axis, basis);
    const target = Math.max(-reach, Math.min(reach, want)) / reach;
    const here = Math.atan2(dot3(direction, side), dot3(direction, basis));
    const solved = Math.acos(Math.max(-1, Math.min(1, target)));
    // Two angles give the same component; keep whichever is the smaller move,
    // so a correction never flips a limb across the body.
    const angle = Math.abs(((here - solved + Math.PI * 3) % (Math.PI * 2)) - Math.PI)
        <= Math.abs(((here + solved + Math.PI * 3) % (Math.PI * 2)) - Math.PI)
        ? solved : -solved;
    return add3(mul3(basis, Math.cos(angle)), mul3(side, Math.sin(angle)));
};

const constrainHinge = (joints: Record<JointKey, Vec3>, hinge: Hinge): void => {
    const frame = frameOf(joints, hinge.side, hinge.other);
    if (!frame) return;
    const upper = sub3(joints[hinge.joint], joints[hinge.root]);
    const lower = sub3(joints[hinge.tip], joints[hinge.joint]);
    const length = len3(lower);
    if (len3(upper) < 1e-6 || length < 1e-6) return;
    const u = norm3(upper);
    const d = norm3(lower);
    const cos = Math.max(-1, Math.min(1, dot3(u, d)));
    if (cos > HINGE_DEADBAND) return;

    const raw = cross3(u, d);
    if (len3(raw) < 1e-9) return;
    let normal = norm3(raw);

    // The plane first: the bend normal has to lie on the joint's own side of
    // the body, and no further round than the parent bone can twist.
    const flat = sub3(frame.across, mul3(u, dot3(frame.across, u)));
    const reach = len3(flat);
    if (reach > 1e-6) {
        const along = dot3(normal, frame.across);
        const least = Math.cos(hinge.maxTwist * DEG) * reach;
        const wrongSide = Math.sign(along) !== hinge.normalSign;
        if (wrongSide || Math.abs(along) < least) {
            normal = swingAbout(normal, u, frame.across, hinge.normalSign * least);
        }
    }

    // ...then the amount, which a hinge caps in both directions.
    const bend = Math.acos(cos);
    const allowed = Math.min(bend, hinge.maxBend * DEG);
    const toward = norm3(cross3(normal, u));
    if (len3(toward) < 1e-6) return;
    const aimed = add3(mul3(u, Math.cos(allowed)), mul3(toward, Math.sin(allowed)));
    joints[hinge.tip] = add3(joints[hinge.joint], mul3(norm3(aimed), length));
};

const constrainCone = (joints: Record<JointKey, Vec3>, cone: Cone): void => {
    const frame = frameOf(joints, cone.side, cone.other);
    if (!frame) return;
    const bone = sub3(joints[cone.joint], joints[cone.root]);
    const length = len3(bone);
    if (length < 1e-6) return;
    let d = norm3(bone);
    const down = mul3(frame.up, -1);

    // Away from "down" — a thigh that has swung past this is not lifted, it is
    // dislocated.
    const lift = Math.acos(Math.max(-1, Math.min(1, dot3(d, down))));
    const cap = cone.maxFromDown * DEG;
    if (lift > cap) {
        const sideways = sub3(d, mul3(down, dot3(d, down)));
        if (len3(sideways) > 1e-6) {
            d = norm3(add3(mul3(down, Math.cos(cap)), mul3(norm3(sideways), Math.sin(cap))));
        }
    }

    // ...and across the midline. `outward` is the way this limb's own side
    // lies, so a negative component is the leg crossing the body.
    const outward = cone.joint.endsWith('L')
        ? mul3(frame.across, -1)
        : frame.across;
    const crossing = dot3(d, outward);
    const limit = -Math.sin(cone.maxCrossing * DEG);
    if (crossing < limit) {
        const rest = sub3(d, mul3(outward, crossing));
        if (len3(rest) > 1e-6) {
            const scale = Math.sqrt(Math.max(0, 1 - limit * limit));
            d = norm3(add3(mul3(outward, limit), mul3(norm3(rest), scale)));
        }
    }

    const moved = add3(joints[cone.root], mul3(d, length));
    const shift = sub3(moved, joints[cone.joint]);
    if (len3(shift) < 1e-9) return;
    // The rest of the limb rides along, or the correction would break the bone
    // below it.
    for (const member of SUBTREES[cone.joint]) {
        joints[member] = add3(joints[member], shift);
    }
};

// How far the head may lean off the spine, and how far it may look away from
// the body's own front. The face is measured against the *front*, not against
// the skull's axis — the face already sits square to that axis, so capping it
// there would aim the nose at the ceiling. Eighty-five degrees is a head turned
// as far as a neck turns: "looking back" stays reachable, looking behind you
// does not.

const HEAD_CONE = 62;
const FACE_CONE = 85;

const constrainAim = (joints: Record<JointKey, Vec3>, key: JointKey, root: JointKey, axis: Vec3, cap: number): void => {
    const bone = sub3(joints[key], joints[root]);
    const length = len3(bone);
    if (length < 1e-6 || len3(axis) < 1e-6) return;
    const rest = norm3(axis);
    const d = norm3(bone);
    const off = Math.acos(Math.max(-1, Math.min(1, dot3(d, rest))));
    if (off <= cap * DEG) return;
    const sideways = sub3(d, mul3(rest, dot3(d, rest)));
    if (len3(sideways) < 1e-6) return;
    const aimed = norm3(add3(mul3(rest, Math.cos(cap * DEG)), mul3(norm3(sideways), Math.sin(cap * DEG))));
    const moved = add3(joints[root], mul3(aimed, length));
    const shift = sub3(moved, joints[key]);
    for (const member of SUBTREES[key]) {
        joints[member] = add3(joints[member], shift);
    }
};

// The nearest legal body to the one given. Cheap enough to run on every drag
// frame: a fixed handful of joints, no iteration, no search.
export const constrainFigure = (figure: PoseFigure): PoseFigure => {
    const joints = { ...figure.joints };
    // Proximal before distal, so a thigh pulled back into range carries its
    // shin with it and the knee is then judged in its corrected frame.
    for (const cone of CONES) constrainCone(joints, cone);
    for (const hinge of HINGES) constrainHinge(joints, hinge);
    constrainAim(joints, 'head', 'neck', sub3(joints.neck, joints.hip), HEAD_CONE);
    constrainAim(joints, 'face', 'head', bodyForwardOf(joints), FACE_CONE);
    return { ...figure, joints };
};

// The escape hatch (Alt): move one joint on its own, at the depth it already
// has. Proportions are the skeleton's job, not a law.
// The escape hatch is an escape from the *skeleton*, not from the rig: Alt lets
// a joint leave its bone length behind, because odd proportions are a drawing
// choice, but it still may not put a knee on backwards.
export const moveJoint = (figure: PoseFigure, key: JointKey, point: CanvasPoint): PoseFigure => constrainFigure({
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
// saved before the face joint existed gets one — even when the transform
// itself is a no-op.
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
// Which joints get a handle. Every joint except the hip root: dragging that
// one translates the whole figure, which dragging the *body* already does and
// does more discoverably — so it was a handle that did nothing new, sitting
// right between hipL and hipR, where it turned the pelvis into a pile of three
// dots you could not pick apart (principle 9).
export const HANDLE_KEYS: readonly JointKey[] = JOINT_KEYS.filter((key) => key !== 'hip');

// How far down the chain a joint is. Used to break a tie between two handles
// that land on top of each other: the one further out is the finer control and
// is always the one meant — a wrist over an elbow, the face over the head.
const CHAIN_DEPTH: Record<JointKey, number> = (() => {
    const depths = {} as Record<JointKey, number>;
    for (const key of JOINT_KEYS) {
        let steps = 0;
        let walk: JointKey | null = JOINT_PARENT[key];
        while (walk) {
            steps += 1;
            walk = JOINT_PARENT[walk];
        }
        depths[key] = steps;
    }
    return depths;
})();

// The face handle is pushed out to a minimum distance from the head's, in
// handle radii. Its bone points out of the skull, so the moment the head faces
// toward or away from the camera the two project onto the same pixel — on our
// own library that happens in more than half of all pose-and-view combinations,
// which is most of what "the handles are hard to grab" was.
const FACE_HANDLE_GAP = 2.4;

// The face bone's full length on screen, which is also the radius of the ball
// the drag reads depth inside. Distances are remapped into [gap, reach] for
// drawing and back again for dragging, so pushing the handle out costs none of
// that: dragging it to the gap still means "looking straight at the camera".
const faceDial = (figure: PoseFigure, handleRadius: number) => {
    const at = projectFigure(figure);
    const reach = Math.max(figureUnit(figure) * BONE.face * at.face.scale, 1);
    const gap = Math.min(handleRadius * FACE_HANDLE_GAP, reach * 0.75);
    let dx = at.face.x - at.head.x;
    let dy = at.face.y - at.head.y;
    let length = Math.hypot(dx, dy);
    if (length < 1e-6) {
        // Pointing straight at or away from the camera: there is no direction
        // on screen to use, so the dial parks above the crown. Which of the two
        // it is stays the same question it is everywhere else in this tool, and
        // has the same answer — Shift.
        dx = at.head.x - at.neck.x;
        dy = at.head.y - at.neck.y;
        length = Math.hypot(dx, dy);
        if (length < 1e-6) { dx = 0; dy = -1; length = 1; }
    }
    return { head: at.head, dir: { x: dx / length, y: dy / length }, out: length, reach, gap };
};

export const handlePointOf = (
    figure: PoseFigure,
    key: JointKey,
    handleRadius: number,
): CanvasPoint => {
    if (key !== 'face') return projectFigure(figure)[key];
    const { head, dir, out, reach, gap } = faceDial(figure, handleRadius);
    const drawn = gap + Math.min(out / reach, 1) * (reach - gap);
    return { x: head.x + dir.x * drawn, y: head.y + dir.y * drawn };
};

// The inverse: a pointer on the dial, back to the point `swingJoint` should aim
// at. Identity for every joint whose handle is drawn where it actually is.
export const swingTargetOf = (
    figure: PoseFigure,
    key: JointKey,
    pointer: CanvasPoint,
    handleRadius: number,
): CanvasPoint => {
    if (key !== 'face') return pointer;
    const { head, reach, gap } = faceDial(figure, handleRadius);
    const dx = pointer.x - head.x;
    const dy = pointer.y - head.y;
    const length = Math.hypot(dx, dy);
    if (length < 1e-6) return pointer;
    const inner = Math.max(0, (length - gap) * reach / Math.max(reach - gap, 1e-6));
    return { x: head.x + (dx / length) * inner, y: head.y + (dy / length) * inner };
};

export const hitTestJoint = (
    figure: PoseFigure,
    point: CanvasPoint,
    radius: number,
    handleRadius = radius / 2,
): JointKey | null => {
    const hits = HANDLE_KEYS
        .map((key) => {
            const at = handlePointOf(figure, key, handleRadius);
            return { key, gap: Math.hypot(point.x - at.x, point.y - at.y) };
        })
        .filter((hit) => hit.gap <= radius);
    if (hits.length === 0) return null;
    const closest = Math.min(...hits.map((hit) => hit.gap));
    // Anything this close to the closest is a tie rather than a choice — the
    // pointer is not accurate to a third of the pick radius anyway. Among ties
    // the joint further down the chain wins, then the nearer one, then the one
    // in front.
    const projected = projectFigure(figure);
    const ties = hits.filter((hit) => hit.gap <= closest + radius * 0.33);
    ties.sort((a, b) => (CHAIN_DEPTH[b.key] - CHAIN_DEPTH[a.key])
        || (a.gap - b.gap)
        || (projected[b.key].depth - projected[a.key].depth));
    return ties[0].key;
};

// True when the point is on the mannequin's silhouette, which is what "grab
// the body and move it" means. Tested against `figureParts` — the shapes that
// are actually drawn — rather than against a second table of limb widths: two
// tables of the same physical fact drift, and then what you can grab stops
// matching what you can see.
export const hitTestBody = (figure: PoseFigure, point: CanvasPoint, tolerance = 0): boolean => {
    const parts = figureParts(figure);
    // The torso is tested against the outline that is actually drawn, so what
    // you can grab and what you can see cannot drift apart.
    if (parts.torso.some((block) => inPolygon(point, block)) || inPolygon(point, parts.head)) return true;
    if (parts.limbs.some((limb) => limb.outlines.some((outline) => inPolygon(point, outline)))) return true;
    if (parts.sockets
        .some((ball) => Math.hypot(point.x - ball.center.x, point.y - ball.center.y) <= ball.radius + tolerance)) {
        return true;
    }
    return insideSegment(point, parts.neck, tolerance);
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

export interface FigureLimb {
    key: 'armL' | 'armR' | 'legL' | 'legR';
    // Closed, projected, and the same arrays the renderer fills and the hit
    // test runs on — so what you can grab cannot drift from what you can see.
    // One piece for a limb that reads as one run; two where it folds back on
    // itself far enough that a single outline would cross itself, which fills
    // as a fin. An artist splits there too: the forearm is drawn *over* the
    // upper arm, not merged into it.
    outlines: CanvasPoint[][];
    // The bend, as the two points where it crosses the limb plus a control
    // point bowed toward the far side. A full circle here reads as a dot drawn
    // on the arm; this reads as two turned pieces meeting. Absent once the
    // limb is split, where the overlap of the two pieces *is* the joint.
    seam?: { from: CanvasPoint; to: CanvasPoint; bow: CanvasPoint };
    depth: number;
    // Per piece, in the same order as `outlines`.
    depths: number[];
}

export type ShapeTone = 'body' | 'joint' | 'head';

// A drawable with the depth it is at, so the renderer can sort. Parts are
// grouped rather than sorted one by one: a body is a handful of solids that
// overlap each other, and a chest, its waist ball and its pelvis have a
// drawing order that comes from anatomy, not from depth.
// `tint` lightens a shape above its tone without adding a fourth colour to
// every shade table — used for the one surface that is not a volume, the flat
// plane of the face. `marking` says the same thing to the renderer: this is a
// mark *on* a form, not a form, so it gets the seam's hairline rather than the
// contour every solid part is drawn with. A full contour round the face turns
// the head into an egg with a ring on it.
export type FigureShape =
    | { kind: 'ellipse'; ellipse: Ellipse; tone: ShapeTone; depth: number; tint?: number; marking?: true }
    | { kind: 'segment'; segment: Segment; tone: ShapeTone; depth: number; tint?: number }
    | { kind: 'ball'; ball: Ball; tone: ShapeTone; depth: number; tint?: number }
    | { kind: 'polygon'; points: CanvasPoint[]; tone: ShapeTone; depth: number; tint?: number };

export interface FigureCluster {
    key: 'torso' | 'head' | 'armL' | 'armR' | 'legL' | 'legR';
    // Drawn as a hairline after the cluster's body, where it has one.
    seam?: { from: CanvasPoint; to: CanvasPoint; bow: CanvasPoint };
    depth: number;
    // Fixed for the torso, where the order encodes anatomy (hip balls under
    // the pelvis); by depth inside a limb, where a fold really does put the
    // forearm in front of the upper arm.
    order: 'authored' | 'depth';
    shapes: FigureShape[];
}

export interface FigureParts {
    // An ovoid with a jaw, as an outline. An ellipse is an egg, and an egg on
    // a peg is the single most toy-like thing a mannequin can have on its
    // shoulders — a head reads as a head because it is wide at the cranium and
    // narrows to a chin.
    head: CanvasPoint[];
    // The flat plane of the face, or null when the figure is looking away.
    // A wooden manikin has no features, and without this a back view is
    // pixel-for-pixel a front view — which makes "turn it round" a control
    // that does nothing a model could ever read.
    face: Ellipse | null;
    neck: Segment;
    // The torso, as one closed outline rather than a ribcage, a waist ball and
    // a pelvis stacked on a spine. That was the single biggest thing making
    // Two blocks: the chest down to the waist, and the pelvis bucket, with the
    // waist ball showing between them — a manikin's torso, and the only way
    // the waist actually *reads*. An earlier version was three convex blobs,
    // each shaded on its own, and announced its seams however carefully they
    // were fitted; the one after it was a single outline, which held together
    // but could only imply a waist with a line, and a line across a form is a
    // crease rather than a joint. Each block is a profile rather than an egg,
    // takes one light across the whole of it, and still twists: the chest's
    // top edge follows the shoulder line, the pelvis's bottom edge the hips.
    torso: CanvasPoint[][];
    // Each limb is one closed outline from the shoulder or hip all the way
    // into the mitten or the foot, with the bend showing only as a seam drawn
    // *across* it.
    //
    // This replaces a chain of tapered capsules with a ball at every joint,
    // which was the single thing making the mannequin read as assembled parts.
    // The ball is not a drawing convention at all — it is how a physical
    // wooden manikin has to be *manufactured* to rotate, and copying it is why
    // people say that drawing from one gives you wooden figures. The art-school
    // construction narrows a limb at its joints instead.
    limbs: FigureLimb[];
    // The ball-and-socket joints — two shoulders, two hips — drawn *under*
    // the torso so each shows only the sliver the limb leaves uncovered. The
    // hinges are not here: they are a narrowing in the limb's own outline.
    // Two shoulders, the waist, two hips.
    sockets: Ball[];
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
    headLong: 0.068, headWide: 0.046,
    // A column, not a peg. The old one was so thin, and the old head so large,
    // that no neck was visible at all and the skull sat straight on the chest.
    neck: 0.027,
    // The width of a limb at each node, from the shoulder or hip down. Two
    // rules from the art-school construction, both of which the first version
    // had backwards: an upper arm "stays about the same width from top to
    // bottom", and a lower leg is "widest at the calf, about two thirds of the
    // way up" rather than tapering straight from the knee to the ankle. And
    // limbs **narrow** at a joint rather than bulging — see `limbOutline`.
    upperArm: 0.031, elbow: 0.028, wrist: 0.019,
    thigh: 0.043, knee: 0.030, calf: 0.034, ankle: 0.023,
    // The two ball-and-socket joints a wooden manikin really does show as
    // balls. The hinges — elbow, knee, wrist, ankle — do not: look at a
    // manikin and the upper arm *narrows* into the elbow pin.
    hipBall: 0.031, shoulderBall: 0.030,
    // The waist ball, which on a manikin is the joint you see *through the
    // gap* between the chest block and the pelvis block.
    waistBall: 0.040,
    // Longer and narrower than they were: a mitten only a little wider than
    // the wrist reads as a hand, while one much wider reads as an oven glove.
    handLong: 0.052, handWide: 0.021,
    footLong: 0.058, footWide: 0.019,
    // A body is not a cut-out: seen from the side the chest and pelvis are as
    // deep as they are wide, so their on-screen width can never fall below
    // this however far the shoulders foreshorten.
    chestDepth: 0.058, pelvisDepth: 0.044,
} as const;

// The canon's unit of measure: crown to chin, as a fraction of the figure's
// height. Eight of these is the whole body, two of them the shoulders. Exported
// because it is the ruler the proportions are checked against, and a ruler kept
// privately is a ruler that drifts.
export const HEAD_LENGTH_RATIO = R.headLong * 2;

// A closed outline through the given points, smoothed. Catmull-Rom rather
// than straight edges because a torso has no corners, and sampled to a
// polygon rather than left as curves because the same array then serves both
// the fill and the hit test — one geometry, never two tables.
const SMOOTH_STEPS = 6;

const smoothLoop = (points: readonly CanvasPoint[]): CanvasPoint[] => {
    const count = points.length;
    if (count < 3) return [...points];
    const out: CanvasPoint[] = [];
    for (let i = 0; i < count; i += 1) {
        const p0 = points[(i - 1 + count) % count];
        const p1 = points[i];
        const p2 = points[(i + 1) % count];
        const p3 = points[(i + 2) % count];
        for (let step = 0; step < SMOOTH_STEPS; step += 1) {
            const t = step / SMOOTH_STEPS;
            const t2 = t * t;
            const t3 = t2 * t;
            out.push({
                x: 0.5 * ((2 * p1.x) + (-p0.x + p2.x) * t
                    + (2 * p0.x - 5 * p1.x + 4 * p2.x - p3.x) * t2
                    + (-p0.x + 3 * p1.x - 3 * p2.x + p3.x) * t3),
                y: 0.5 * ((2 * p1.y) + (-p0.y + p2.y) * t
                    + (2 * p0.y - 5 * p1.y + 4 * p2.y - p3.y) * t2
                    + (-p0.y + 3 * p1.y - 3 * p2.y + p3.y) * t3),
            });
        }
    }
    return out;
};

// The torso's profile, as fractions down the neck→hip axis and fractions of
// the half-width at that height. Read it as a body seen from the front: the
// trapezius tucking under the neck, the deltoid, the ribcage narrowing, the
// waist, the hip, the seat. The numbers are the shape of the mannequin, and
// they are the difference between a person and three stacked eggs.
// Crown to chin, as fractions of the head's length and of its half-width.
const HEAD_PROFILE: readonly (readonly [number, number])[] = [
    [0.00, 0.18],
    [0.10, 0.68],
    [0.28, 1.00],   // the cranium, the widest part
    [0.54, 0.96],
    [0.76, 0.72],   // the cheek
    [0.92, 0.40],   // the jaw
    [1.00, 0.15],   // the chin
];

// The torso is two blocks, not one form: a chest that carries the ribcage down
// to the waist, and a pelvis bucket under it, with the waist ball showing in
// the gap. That is what a manikin is, and it is what makes the waist *read* —
// one continuous outline can only imply it with a line, and a line across a
// form is a crease, not a joint.
//
// Both tables are fractions down the neck→hip axis, and fractions of that
// block's half-width at that height.
const CHEST_PROFILE: readonly (readonly [number, number])[] = [
    [-0.06, 0.50],              // behind the neck, so there is no notch there
    [0.02, 0.86],               // the slope of the trapezius
    [0.10, 1.00],               // the top corner, level with the shoulder joint
    [0.30, 0.94],
    [0.48, 0.78],               // the ribcage drawing in
    [0.62, 0.56],               // the waist — where the block stops
];

const PELVIS_PROFILE: readonly (readonly [number, number])[] = [
    [0.76, 0.58],               // the top of the bucket, narrow
    [0.90, 1.00],               // the iliac crest — the widest the pelvis gets
    [1.00, 0.94],
    // The pelvis *narrows onto* the hip joints (t≈1.06) and stops. Carrying it
    // wider and lower than the joints was the "skirt": the legs then left
    // through a slot in the middle of it instead of pivoting on its bottom
    // corners, which is what a manikin actually does.
    [1.05, 0.74],
    [1.09, 0.34],
];

// Where the waist ball sits on that axis — in the gap, bridging both blocks.
const WAIST_T = 0.69;

// One limb, as a closed outline through its nodes. Each side is offset by the
// node's radius along the bisector, and both ends are capped with a half
// circle — the torso's idea, applied to an arm.
interface LimbNode { point: CanvasPoint; radius: number }

const CAP_STEPS = 9;

// Seen end-on — a shin under a cross-legged figure viewed from above — a
// segment projects to almost nothing. Its direction is then noise, and
// offsetting along it throws the two rails across each other as a fin. So
// fold any node that lands on top of its predecessor into it, keeping the
// wider radius: a limb pointing at the camera is a disc, which is what it
// looks like.
const foldDegenerate = (nodes: readonly LimbNode[]): LimbNode[] => {
    const kept: LimbNode[] = [];
    for (const node of nodes) {
        const last = kept[kept.length - 1];
        if (last && Math.hypot(node.point.x - last.point.x, node.point.y - last.point.y)
            < Math.max(last.radius, node.radius) * 0.35) {
            kept[kept.length - 1] = {
                point: node.point,
                radius: Math.max(last.radius, node.radius),
            };
            continue;
        }
        kept.push(node);
    }
    return kept;
};

const limbOutline = (input: readonly LimbNode[]): {
    outline: CanvasPoint[];
    right: CanvasPoint[];
    left: CanvasPoint[];
} => {
    const nodes = foldDegenerate(input);
    if (nodes.length < 2) {
        // A limb aimed straight at the camera. One circle, no rails.
        const only = nodes[0] ?? input[0];
        const ring: CanvasPoint[] = [];
        for (let step = 0; step < CAP_STEPS * 2; step += 1) {
            const angle = (step / (CAP_STEPS * 2)) * Math.PI * 2;
            ring.push({
                x: only.point.x + Math.cos(angle) * only.radius,
                y: only.point.y + Math.sin(angle) * only.radius,
            });
        }
        return { outline: ring, right: [only.point], left: [only.point] };
    }
    const normals: CanvasPoint[] = [];
    for (let i = 0; i < nodes.length - 1; i += 1) {
        const dx = nodes[i + 1].point.x - nodes[i].point.x;
        const dy = nodes[i + 1].point.y - nodes[i].point.y;
        const length = Math.hypot(dx, dy) || 1;
        normals.push({ x: -dy / length, y: dx / length });
    }
    const sideways: CanvasPoint[] = nodes.map((_, i) => {
        if (i === 0) return normals[0];
        if (i === nodes.length - 1) return normals[normals.length - 1];
        const a = normals[i - 1];
        const b = normals[i];
        let x = a.x + b.x;
        let y = a.y + b.y;
        const length = Math.hypot(x, y);
        if (length < 1e-6) return b;
        x /= length;
        y /= length;
        // Mitred, with a limit — an arm folded right back would otherwise
        // throw a spike out of the inside of the elbow.
        const cos = Math.max(0.45, x * b.x + y * b.y);
        return { x: x / cos, y: y / cos };
    });
    const right = nodes.map((node, i) => ({
        x: node.point.x + sideways[i].x * node.radius,
        y: node.point.y + sideways[i].y * node.radius,
    }));
    const left = nodes.map((node, i) => ({
        x: node.point.x - sideways[i].x * node.radius,
        y: node.point.y - sideways[i].y * node.radius,
    }));
    const cap = (node: LimbNode, from: CanvasPoint): CanvasPoint[] => {
        const start = Math.atan2(from.y - node.point.y, from.x - node.point.x);
        const arc: CanvasPoint[] = [];
        for (let step = 1; step < CAP_STEPS; step += 1) {
            const angle = start - Math.PI * (step / CAP_STEPS);
            arc.push({
                x: node.point.x + Math.cos(angle) * node.radius,
                y: node.point.y + Math.sin(angle) * node.radius,
            });
        }
        return arc;
    };
    const last = nodes.length - 1;
    return {
        outline: [
            ...right,
            ...cap(nodes[last], right[last]),
            ...[...left].reverse(),
            ...[...cap(nodes[0], left[0])].reverse(),
        ],
        right,
        left,
    };
};

// How far a limb may turn on screen before one outline stops working. Straight
// is 1 and doubled right back is -1; this sits at about 110 degrees off
// straight, past which the two rails cross and the fill grows a fin.
const FOLD_LIMIT = -0.35;

const foldIndex = (nodes: readonly LimbNode[]): number => {
    for (let i = 1; i < nodes.length - 1; i += 1) {
        const inX = nodes[i].point.x - nodes[i - 1].point.x;
        const inY = nodes[i].point.y - nodes[i - 1].point.y;
        const outX = nodes[i + 1].point.x - nodes[i].point.x;
        const outY = nodes[i + 1].point.y - nodes[i].point.y;
        const lengths = Math.hypot(inX, inY) * Math.hypot(outX, outY);
        if (lengths < 1e-6) continue;
        if ((inX * outX + inY * outY) / lengths < FOLD_LIMIT) return i;
    }
    return -1;
};

// Which rail point stands for a given joint. Folding degenerate nodes away can
// shorten the rails, so the index is looked up rather than assumed.
const nearestRail = (rail: readonly CanvasPoint[], point: CanvasPoint): number => {
    let best = 0;
    let bestGap = Infinity;
    for (let i = 0; i < rail.length; i += 1) {
        const gap = Math.hypot(rail[i].x - point.x, rail[i].y - point.y);
        if (gap < bestGap) {
            bestGap = gap;
            best = i;
        }
    }
    return best;
};

export const inPolygon = (point: CanvasPoint, polygon: readonly CanvasPoint[]): boolean => {
    let inside = false;
    for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i, i += 1) {
        const a = polygon[i];
        const b = polygon[j];
        if ((a.y > point.y) !== (b.y > point.y)
            && point.x < ((b.x - a.x) * (point.y - a.y)) / (b.y - a.y) + a.x) {
            inside = !inside;
        }
    }
    return inside;
};

export const figureParts = (figure: PoseFigure): FigureParts => {
    const joints = figure.joints;
    const projection = projectionOf(figure);
    const at = projectFigure(figure);
    const unit = figureUnit(figure);
    // Girth scales with nearness; length is whatever the projection says.
    const u = (ratio: number, scale: number) => unit * ratio * scale;
    const place = (point: Vec3) => projectPoint(point, projection);

    const hipMid = place(mid3(joints.hipL, joints.hipR));

    const forward = bodyForwardOf(joints);

    // Where each limb's outline changes width. The bend node is deliberately
    // the *narrowest* point between the two segments: a limb narrows at a
    // joint. The calf node exists because a lower leg is widest two thirds of
    // the way up, not at the knee.
    const limbSpec = [
        ['armL', 'shoulderL', 'elbowL', 'wristL'],
        ['armR', 'shoulderR', 'elbowR', 'wristR'],
        ['legL', 'hipL', 'kneeL', 'ankleL'],
        ['legR', 'hipR', 'kneeR', 'ankleR'],
    ] as const;

    const limbs: FigureLimb[] = limbSpec.map(([key, root, bend, tip]) => {
        const leg = key.startsWith('leg');
        const scale = (joint: JointKey) => at[joint].scale;
        // The root node is pulled a little way down the limb so its cap tucks
        // under the torso instead of standing proud of the shoulder line as a
        // bump — the arm is its own cluster, so its outline would otherwise
        // draw a dome on top of the deltoid.
        const rootRadius = u(leg ? R.thigh : R.upperArm, scale(root));
        const inward = norm3(sub3(joints[bend], joints[root]));
        // An arm hangs off the *outer face* of its ball, not out of the joint's
        // centre: on a manikin the ball stands proud of the chest and the upper
        // arm rests against its outside. Started at the centre instead, half
        // the arm is sunk into the chest block and the shoulder reads as one
        // lump. Legs need none of this — a thigh really does come out from
        // under the pelvis.
        const outward = leg ? ORIGIN : mul3(
            norm3(sub3(joints[root], joints.neck)),
            unit * R.shoulderBall * 0.75,
        );
        const rootPoint = add3(add3(joints[root], outward), mul3(inward, rootRadius * (leg ? 0.25 : 0.18)));
        const nodes: LimbNode[] = [
            { point: place(rootPoint), radius: rootRadius },
            { point: at[bend], radius: u(leg ? R.knee : R.elbow, scale(bend)) },
        ];
        if (leg) {
            const calf = place(lerp3(joints[bend], joints[tip], 0.33));
            nodes.push({ point: calf, radius: u(R.calf, calf.scale) });
        }
        nodes.push({ point: at[tip], radius: u(leg ? R.ankle : R.wrist, scale(tip)) });

        // The hand and the foot are the end of the limb, not a lozenge stuck
        // on it: a mitten that swells past the wrist, a block that points the
        // way the body faces.
        const long = leg ? R.footLong : R.handLong;
        const wide = leg ? R.footWide : R.handWide;
        const direction = leg
            ? squareTo(forward, norm3(sub3(joints[tip], joints[bend])))
            : norm3(sub3(joints[tip], joints[bend]));
        const mid = place(add3(joints[tip], mul3(direction, unit * long * 0.45)));
        const far = place(add3(joints[tip], mul3(direction, unit * long * 0.95)));
        nodes.push({ point: mid, radius: u(wide, mid.scale) });
        nodes.push({ point: far, radius: u(wide, far.scale) * 0.82 });

        // Where does the chain double back on itself? Not at the joint's real
        // angle — a knee bent flat to the floor still reads as one run when
        // seen from the front — but at its angle *on screen*, which is what
        // the outline is built in.
        const fold = foldIndex(nodes);
        const depth = (at[root].depth + at[bend].depth + at[tip].depth) / 3;
        if (fold >= 0) {
            const near = limbOutline(nodes.slice(0, fold + 1));
            const far = limbOutline(nodes.slice(fold));
            return {
                key,
                outlines: [smoothLoop(near.outline), smoothLoop(far.outline)],
                depths: [
                    (at[root].depth + at[bend].depth) / 2,
                    (at[bend].depth + at[tip].depth) / 2,
                ],
                depth,
            };
        }
        const built = limbOutline(nodes);
        const bendRail = nearestRail(built.right, at[bend]);
        const after = nodes[Math.min(bendRail + 1, nodes.length - 1)].point;
        const bow = {
            x: at[bend].x + (after.x - at[bend].x) * 0.24,
            y: at[bend].y + (after.y - at[bend].y) * 0.24,
        };
        return {
            key,
            outlines: [smoothLoop(built.outline)],
            depths: [depth],
            seam: { from: built.right[bendRail], to: built.left[bendRail], bow },
            depth,
        };
    });

    // The torso, as one outline. No spine segment underneath any more: the
    // outline itself spans neck to hip, so the figure cannot come apart
    // The facial plane sits on the front of the skull and is simply absent
    // once it has turned away. It is lighter rather than darker: the light is
    // in front, so the flat of the face is the part of the head that catches
    // it — and a dark patch on a head reads as a mask.
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

    // however hard a shoulder is dragged, and there is nothing to show through.
    const { blocks: torso, waist: waistCentre } = (() => {
        const axis = { x: hipMid.x - at.neck.x, y: hipMid.y - at.neck.y };
        const axisLength = Math.hypot(axis.x, axis.y) || 1;
        const spineAt = (t: number): CanvasPoint => (
            { x: at.neck.x + axis.x * t, y: at.neck.y + axis.y * t }
        );
        // A block's half-width is measured *per side, toward the real joint*,
        // not as one span mirrored about the spine. Under perspective a turned
        // body's two shoulders are not equidistant from its spine, and mirroring
        // one number leaves the far ball floating off the edge as a loose disc.
        // Floored on the body's own depth, so a torso seen edge-on narrows to
        // its own thickness and no further — a person is not a cut-out.
        const toward = (joint: CanvasPoint, anchor: number, reach: number, floor: number, sign: number): CanvasPoint => {
            const spine = spineAt(anchor);
            const dx = (joint.x - spine.x) * reach;
            const dy = (joint.y - spine.y) * reach;
            if (Math.hypot(dx, dy) >= floor) return { x: dx, y: dy };
            return { x: (-axis.y / axisLength) * floor * sign, y: (axis.x / axisLength) * floor * sign };
        };
        // The block stops short of the joint so the ball stands proud at the
        // corner and the limb *rests on the edge* instead of sinking in. The
        // shoulders you see are the chest plus its two balls, not the chest.
        const CHEST_REACH = 0.92;
        const PELVIS_REACH = 1.28;
        const chestFloor = u(R.chestDepth, at.neck.scale) * 1.15;
        const pelvisFloor = u(R.pelvisDepth, hipMid.scale);
        const block = (
            profile: readonly (readonly [number, number])[],
            right: CanvasPoint,
            left: CanvasPoint,
        ) => {
            const near: CanvasPoint[] = [];
            const far: CanvasPoint[] = [];
            for (const [t, scale] of profile) {
                const spine = spineAt(t);
                near.push({ x: spine.x + right.x * scale, y: spine.y + right.y * scale });
                far.push({ x: spine.x + left.x * scale, y: spine.y + left.y * scale });
            }
            return smoothLoop([...near, ...far.reverse()]);
        };
        const chest = block(
            CHEST_PROFILE,
            toward(at.shoulderR, CHEST_PROFILE[2][0], CHEST_REACH, chestFloor, 1),
            toward(at.shoulderL, CHEST_PROFILE[2][0], CHEST_REACH, chestFloor, -1),
        );
        const pelvisAnchor = PELVIS_PROFILE[3][0];
        const pelvis = block(
            PELVIS_PROFILE,
            toward(at.hipR, pelvisAnchor, PELVIS_REACH, pelvisFloor, 1),
            toward(at.hipL, pelvisAnchor, PELVIS_REACH, pelvisFloor, -1),
        );
        return {
            blocks: [chest, pelvis],
            waist: place(lerp3(joints.neck, joints.hip, WAIST_T)),
        };
    })();

    const head: CanvasPoint[] = (() => {
        const scale = at.head.scale;
        const long = u(R.headLong, scale);
        const wide = u(R.headWide, scale);
        // Down the head's own axis, crown first. The width is constant about
        // that axis rather than following the face: a skull is about as deep
        // as it is broad, so turning it should not narrow it — the facial
        // plane is what says which way it looks.
        let ax = at.head.x - at.neck.x;
        let ay = at.head.y - at.neck.y;
        const length = Math.hypot(ax, ay);
        if (length < 1e-6) { ax = 0; ay = -1; } else { ax /= length; ay /= length; }
        const crown = { x: at.head.x + ax * long * 0.72, y: at.head.y + ay * long * 0.72 };
        const drop = long * 1.86;
        const right: CanvasPoint[] = [];
        const left: CanvasPoint[] = [];
        for (const [t, halfScale] of HEAD_PROFILE) {
            const spine = { x: crown.x - ax * drop * t, y: crown.y - ay * drop * t };
            const half = wide * halfScale;
            right.push({ x: spine.x - ay * half, y: spine.y + ax * half });
            left.push({ x: spine.x + ay * half, y: spine.y - ax * half });
        }
        return smoothLoop([...right, ...left.reverse()]);
    })();
    const neck: Segment = {
        from: at.neck,
        to: at.head,
        fromRadius: u(R.neck, at.neck.scale),
        toRadius: u(R.neck, at.head.scale),
    };
    // Drawn *under* the torso, always: a ball on top of the block reads as a
    // buttock or a pauldron, while one behind it shows only where the limb
    // comes out, which is what the wooden joint actually looks like.
    const sockets: Ball[] = [
        { center: at.shoulderL, radius: u(R.shoulderBall, at.shoulderL.scale) },
        { center: at.shoulderR, radius: u(R.shoulderBall, at.shoulderR.scale) },
        { center: waistCentre, radius: u(R.waistBall, waistCentre.scale) },
        { center: at.hipL, radius: u(R.hipBall, at.hipL.scale) },
        { center: at.hipR, radius: u(R.hipBall, at.hipR.scale) },
    ];

    const mean = (...depths: number[]) => depths.reduce((sum, d) => sum + d, 0) / depths.length;

    const clusters: FigureCluster[] = [
        {
            key: 'torso',
            order: 'authored',
            depth: mean(at.neck.depth, hipMid.depth),
            // Authored, not depth-sorted: every ball belongs *under* its
            // block whatever the depths say, or a shoulder ball lands on top
            // of the chest as a dark disc. Only the two blocks are sorted
            // against each other, which is what a bend forward or back needs.
            shapes: [
                { kind: 'ball', ball: sockets[0], tone: 'joint', depth: at.shoulderL.depth },
                { kind: 'ball', ball: sockets[1], tone: 'joint', depth: at.shoulderR.depth },
                { kind: 'ball', ball: sockets[2], tone: 'joint', depth: waistCentre.depth },
                { kind: 'ball', ball: sockets[3], tone: 'joint', depth: at.hipL.depth },
                { kind: 'ball', ball: sockets[4], tone: 'joint', depth: at.hipR.depth },
                ...([
                    { kind: 'polygon' as const, points: torso[0], tone: 'body' as const, depth: mean(at.neck.depth, waistCentre.depth) },
                    { kind: 'polygon' as const, points: torso[1], tone: 'body' as const, depth: mean(waistCentre.depth, hipMid.depth) },
                ].sort((a, b) => a.depth - b.depth)),
            ],
        },
        {
            key: 'head',
            order: 'authored',
            depth: at.head.depth,
            shapes: [
                { kind: 'segment', segment: neck, tone: 'body', depth: mean(at.neck.depth, at.head.depth) },
                { kind: 'polygon', points: head, tone: 'head', depth: at.head.depth },
                ...(face ? [{ kind: 'ellipse' as const, ellipse: face, tone: 'head' as const, tint: 0.34, marking: true as const, depth: at.head.depth }] : []),
            ],
        },
        ...limbs.map((limb): FigureCluster => ({
            key: limb.key,
            // A split limb's two pieces overlap, so which one is in front is a
            // question about depth — the whole reason the split is allowed to
            // look like an overlap rather than a merge.
            order: limb.outlines.length > 1 ? 'depth' : 'authored',
            depth: limb.depth,
            seam: limb.seam,
            shapes: limb.outlines.map((points, i): FigureShape => ({
                kind: 'polygon', points, tone: 'body', depth: limb.depths[i],
            })),
        })),
    ];

    return { head, face, neck, torso, limbs, sockets, clusters };
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
export interface FigureTone {
    body: string; joint: string; head: string;
    // The contour. Every part is drawn with one, and it — not the shading —
    // is what carries the form: an illustration of a manikin reads by its
    // line, and at thumbnail size a gradient is just grey.
    line: string;
    // The seam across a bend, drawn lighter than the contour so a plane change
    // never competes with a silhouette.
    rim: string;
}

// The joint tone is only a hair darker than the body. It used to be a full
// step, which turned every elbow, knee and ankle into a dark disc stuck on the
// limb — on a real manikin the joint is the same wood, and you see it because
// of its edge, not its colour.
const FIGURE_SHADES: readonly FigureTone[] = [
    { body: '#b9bdc2', joint: '#aeb2b7', head: '#c2c6cb', line: '#5b6066', rim: '#8b9096' },
    { body: '#9aa0a7', joint: '#90969d', head: '#a3a9b0', line: '#474c52', rim: '#70767d' },
    { body: '#ced3d8', joint: '#c3c8cd', head: '#d6dbe0', line: '#6e747a', rim: '#9ba0a6' },
];

const SELECTED_TONE: FigureTone = {
    body: '#aab3c4', joint: '#a0a9bb', head: '#b3bbcb', line: '#4e5568', rim: '#7b8598',
};

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

// Small on purpose. An earlier version leaned on the gradient for structure,
// which was right when the figure had no contour to lean on instead; with one,
// the same amount of shading only muddies it. The line says where the form is,
// the shading says which way it turns.
const LIT_AMOUNT = 0.13;
const DARK_AMOUNT = 0.15;

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

    if (shape.kind === 'polygon') {
        // One light across the whole torso, along the light direction. This is
        // the other half of drawing it as a single outline: a gradient per
        // volume is exactly what told the eye where the volumes were.
        let minX = Infinity; let maxX = -Infinity; let minY = Infinity; let maxY = -Infinity;
        for (const point of shape.points) {
            if (point.x < minX) minX = point.x;
            if (point.x > maxX) maxX = point.x;
            if (point.y < minY) minY = point.y;
            if (point.y > maxY) maxY = point.y;
        }
        const cx = (minX + maxX) / 2;
        const cy = (minY + maxY) / 2;
        const reach = Math.max(Math.hypot(maxX - minX, maxY - minY) * 0.42, 1);
        const gradient = ctx.createLinearGradient(
            cx + LIGHT_SCREEN.x * reach, cy + LIGHT_SCREEN.y * reach,
            cx - LIGHT_SCREEN.x * reach, cy - LIGHT_SCREEN.y * reach,
        );
        gradient.addColorStop(0, litColor);
        gradient.addColorStop(0.5, base);
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

// Grown outward by stroking the outline at twice the rim and filling: an
// exact offset, and simpler than offsetting the polygon by hand.
const fillPolygon = (ctx: CanvasRenderingContext2D, points: readonly CanvasPoint[], grow = 0): void => {
    if (points.length < 3) return;
    ctx.beginPath();
    ctx.moveTo(points[0].x, points[0].y);
    for (let i = 1; i < points.length; i += 1) ctx.lineTo(points[i].x, points[i].y);
    ctx.closePath();
    if (grow > 0) {
        const lineWidth = ctx.lineWidth;
        const lineJoin = ctx.lineJoin;
        ctx.lineWidth = grow * 2;
        ctx.lineJoin = 'round';
        ctx.strokeStyle = ctx.fillStyle as string;
        ctx.stroke();
        ctx.lineWidth = lineWidth;
        ctx.lineJoin = lineJoin;
    }
    ctx.fill();
};

const fillShape = (ctx: CanvasRenderingContext2D, shape: FigureShape, grow = 0): void => {
    if (shape.kind === 'segment') fillSegment(ctx, shape.segment, grow);
    else if (shape.kind === 'ball') fillBall(ctx, shape.ball, grow);
    else if (shape.kind === 'polygon') fillPolygon(ctx, shape.points, grow);
    else fillEllipse(ctx, shape.ellipse, grow);
};

// How thick the contour is, as a fraction of the figure's height. Thin enough
// to stay a line at dialog size, thick enough to survive a 46x64 thumbnail.
const LINE_RATIO = 0.0075;

// Back to front, one body part at a time, each part filled and then outlined.
// The contour is what makes this read as a manikin rather than as grey soup:
// an earlier version drew a pale halo around each group instead and leaned on
// gradients for the form, which dissolved at thumbnail size and looked muddy
// at full size. Grouping still matters for occlusion — a near arm crossing the
// chest has to cut into it, which is the most direct evidence of depth there
// is — and inside a group the drawing order puts each ball under its block.
export const drawFigure = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    options: { selected?: boolean } = {},
): void => {
    const parts = figureParts(figure);
    const tone = toneFor(figure, options.selected === true);
    const unit = figureUnit(figure);
    const line = Math.max(unit * LINE_RATIO, 1);
    ctx.save();
    ctx.globalCompositeOperation = 'source-over';
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';

    const clusters = [...parts.clusters].sort((a, b) => a.depth - b.depth);
    for (const cluster of clusters) {
        const ordered = cluster.order === 'depth'
            ? [...cluster.shapes].sort((a, b) => a.depth - b.depth)
            : cluster.shapes;
        for (const shape of ordered) {
            ctx.fillStyle = shapeFill(ctx, shape, tone, unit);
            // `fillShape` leaves its path current, so the contour is the same
            // path — the outline can never drift from the thing it outlines.
            fillShape(ctx, shape);
            const marking = shape.kind === 'ellipse' && shape.marking === true;
            ctx.strokeStyle = marking ? tone.rim : tone.line;
            ctx.lineWidth = marking ? line * 0.55 : line;
            ctx.stroke();
        }
        if (cluster.seam) {
            // Across the limb, bowed toward the far side of the bend: the
            // articulation as a plane change rather than a lump. Lighter than
            // the contour, so it never competes with a silhouette.
            ctx.strokeStyle = tone.rim;
            ctx.lineWidth = line * 0.7;
            ctx.beginPath();
            ctx.moveTo(cluster.seam.from.x, cluster.seam.from.y);
            ctx.quadraticCurveTo(cluster.seam.bow.x, cluster.seam.bow.y, cluster.seam.to.x, cluster.seam.to.y);
            ctx.stroke();
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
    const ordered = [...HANDLE_KEYS].sort((a, b) => projected[a].depth - projected[b].depth);
    for (const key of ordered) {
        const joint = handlePointOf(figure, key, handleRadius);
        const scaleOf = projected[key];
        // The face handle is smaller and hollow: it aims the head rather than
        // placing a bone, and it should not compete with the fifteen that
        // carry the pose (principle 9).
        const aim = key === 'face';
        const radius = handleRadius * (aim ? 0.66 : 1) * Math.max(0.7, Math.min(1.4, scaleOf.scale));
        ctx.fillStyle = aim ? HANDLE_STROKE : HANDLE_FILL;
        ctx.strokeStyle = aim ? HANDLE_FILL : HANDLE_STROKE;
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
