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

// A tapered capsule on screen: the thing a projected bone is.
export interface Segment { from: CanvasPoint; to: CanvasPoint; fromRadius: number; toRadius: number }

const insideSegment = (point: CanvasPoint, segment: Segment, tolerance: number): boolean => {
    const { t, distance } = projectOnSegment(point, segment.from, segment.to);
    return distance <= segment.fromRadius + (segment.toRadius - segment.fromRadius) * t + tolerance;
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
// the body and move it" means. Tested against the same solids the renderer
// draws, projected the same way, so what you can grab cannot drift from what
// you can see. A block is tested as the capsule along its axis at its widest,
// which errs a hair generous — the right way for a grab to err.
export const hitTestBody = (figure: PoseFigure, point: CanvasPoint, tolerance = 0): boolean => {
    const projection = projectionOf(figure);
    const at = (p: Vec3) => projectPoint(p, projection);
    for (const solid of figureSolids(figure)) {
        if (solid.kind === 'sphere') {
            const c = at(solid.center);
            const radius = (solid.scale ? Math.max(solid.scale.x, solid.scale.y) : solid.radius) * c.scale;
            if (Math.hypot(point.x - c.x, point.y - c.y) <= radius + tolerance) return true;
            continue;
        }
        const [from, to, fromRadius, toRadius] = solid.kind === 'capsule'
            ? [at(solid.from), at(solid.to), solid.fromRadius, solid.toRadius]
            : (() => {
                const top = solid.profile[solid.profile.length - 1][0] * solid.unit;
                const widest = Math.max(...solid.profile.map(([, w]) => w)) * solid.unit;
                return [at(solid.base), at(add3(solid.base, mul3(solid.axis, top))), widest, widest] as const;
            })();
        const segment: Segment = {
            from, to,
            fromRadius: fromRadius * from.scale,
            toRadius: toRadius * to.scale,
        };
        if (insideSegment(point, segment, tolerance)) return true;
    }
    return false;
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

// --- the manikin -------------------------------------------------------------
//
// What the figure is made of, as solids in world space: spheres, tapered
// capsules and two turned blocks. This list is the *only* description of the
// body. The renderer (`poseFigure3d`) turns it into meshes, hit testing
// projects it, and the bounds pad by it — one geometry, never two tables, so
// what you can grab cannot drift from what you can see.
//
// It used to be drawn by hand in 2D: outlines offset from the bones, depth
// sorted, shaded with gradients, seamed at the joints. Every one of those was
// a way of faking a third dimension the data already had, and every "that
// looks odd" was that fake showing through. A wooden manikin *is* a dozen
// spheres and cones in real light; drawing it as such is the whole fix.

// Sizes as fractions of the figure's height, so a body keeps its build at any
// scale. Slim on purpose: a manikin is a light thing, and the first 3D pass
// was heavy enough in the torso to read as armour.
export const MANIKIN = {
    head: { wide: 0.058, long: 0.066, deep: 0.060 },
    neck: 0.027,
    upperArm: 0.030, elbow: 0.026, wrist: 0.019,
    thigh: 0.041, knee: 0.029, calf: 0.033, ankle: 0.022,
    shoulderBall: 0.027, hipBall: 0.028, waistBall: 0.030,
    // A hand is a small ball just past the wrist, not a blade: the blade read
    // as a spike. A foot is a rounded pad pointing the way the body faces.
    hand: 0.025,
    foot: { long: 0.062, wide: 0.024 },
    // Both blocks are turned from a profile — (height along the axis, half
    // width), as fractions of the figure's height, measured from the block's
    // base. The chest runs from the waist up to the neck, widest at the
    // shoulder line and closing just above it: shoulders are the *top* of a
    // manikin's chest, not a ledge under the neck. The pelvis is a bucket,
    // widest at the crest and closing onto the hip joints.
    chest: {
        base: 0.32, depth: 0.56,
        profile: [[0, 0], [0.006, 0.040], [0.05, 0.058], [0.10, 0.072], [0.15, 0.084], [0.20, 0.092], [0.225, 0.090], [0.24, 0.066], [0.248, 0]],
    },
    pelvis: {
        base: -0.13, depth: 0.60,
        profile: [[0, 0], [0.008, 0.046], [0.03, 0.072], [0.06, 0.080], [0.085, 0.076], [0.11, 0.058], [0.13, 0.032], [0.14, 0]],
    },
    waist: 0.275,
} as const;

// The canon's unit of measure: crown to chin, as a fraction of the figure's
// height. Eight of these is the whole body, two of them the shoulders.
export const HEAD_LENGTH_RATIO = MANIKIN.head.long * 2;

const lerp3 = (a: Vec3, b: Vec3, t: number): Vec3 => ({
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
    z: zOf(a) + (zOf(b) - zOf(a)) * t,
});

export type Solid =
    | { kind: 'sphere'; center: Vec3; radius: number; scale?: Vec3; axis?: Vec3 }
    | { kind: 'capsule'; from: Vec3; to: Vec3; fromRadius: number; toRadius: number }
    | { kind: 'block'; base: Vec3; axis: Vec3; profile: readonly (readonly [number, number])[]; unit: number; depth: number };

// Radii are in world pixels here; the renderer and the hit test both scale
// them by perspective the same way they scale positions.
export const figureSolids = (figure: PoseFigure): Solid[] => {
    const J = figure.joints;
    const u = figureUnit(figure);
    const M = MANIKIN;
    const out: Solid[] = [];
    const sphere = (center: Vec3, radius: number, scale?: Vec3, axis?: Vec3) => out.push({ kind: 'sphere', center, radius: radius * u, scale, axis });
    const capsule = (from: Vec3, to: Vec3, fromRadius: number, toRadius: number) => out.push({
        kind: 'capsule', from, to, fromRadius: fromRadius * u, toRadius: toRadius * u,
    });
    const spine = norm3(sub3(J.neck, J.hip));

    out.push({ kind: 'block', base: lerp3(J.hip, J.neck, M.chest.base), axis: spine, profile: M.chest.profile, unit: u, depth: M.chest.depth });
    sphere(lerp3(J.hip, J.neck, M.waist), M.waistBall);
    out.push({ kind: 'block', base: lerp3(J.hip, J.neck, M.pelvis.base), axis: spine, profile: M.pelvis.profile, unit: u, depth: M.pelvis.depth });

    capsule(J.neck, J.head, M.neck, M.neck);
    sphere(J.head, 1, { x: M.head.wide * u, y: M.head.long * u, z: M.head.deep * u }, norm3(sub3(J.head, J.neck)));

    for (const key of ['shoulderL', 'shoulderR'] as const) sphere(J[key], M.shoulderBall);
    for (const key of ['hipL', 'hipR'] as const) sphere(J[key], M.hipBall);

    const forward = bodyForwardOf(J);
    for (const side of ['L', 'R'] as const) {
        const shoulder = J[`shoulder${side}`], elbow = J[`elbow${side}`], wrist = J[`wrist${side}`];
        capsule(shoulder, elbow, M.upperArm, M.elbow);
        sphere(elbow, M.elbow);
        capsule(elbow, wrist, M.elbow, M.wrist);
        sphere(wrist, M.wrist);
        const reach = norm3(sub3(wrist, elbow));
        sphere(add3(wrist, mul3(reach, M.hand * u * 0.9)), M.hand);

        const hip = J[`hip${side}`], knee = J[`knee${side}`], ankle = J[`ankle${side}`];
        capsule(hip, knee, M.thigh, M.knee);
        sphere(knee, M.knee);
        const calf = lerp3(knee, ankle, 0.33);
        capsule(knee, calf, M.knee, M.calf);
        capsule(calf, ankle, M.calf, M.ankle);
        sphere(ankle, M.ankle);
        // The foot points the way the body faces, squared to the shin, and
        // sits a little below the ankle.
        const shin = norm3(sub3(ankle, knee));
        const toe = squareTo(forward, shin);
        const foot = add3(add3(ankle, mul3(toe, M.foot.long * u * 0.42)), mul3(shin, M.ankle * u * 0.9));
        sphere(foot, 1, { x: M.foot.wide * u, y: M.foot.long * u * 0.5, z: M.foot.wide * u * 0.75 }, toe);
    }
    return out;
};

// --- drawing -----------------------------------------------------------------
//
// Colour lives here; the drawing itself lives in `poseFigure3d`, which needs
// three.js and is kept out of this module so the geometry stays cheap to test.
//
// Neutral grey, no wood: the manikin's structure is the message, and a wood
// colour only invites the model to paint a wooden doll. Three lightnesses
// exist for crowds — two figures in one grey merge into a single blob where
// they overlap, and the model then cannot tell how many people are in the
// frame — and the selected figure wears its own cool tint on top.
export interface FigureTone { body: string }

const FIGURE_SHADES: readonly FigureTone[] = [
    { body: '#c3c7cc' },
    { body: '#a4aab1' },
    { body: '#d8dce1' },
];

const SELECTED_TONE: FigureTone = { body: '#b3bccf' };

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

export const toneFor = (figure: PoseFigure, selected = false): FigureTone => (selected
    ? SELECTED_TONE
    : FIGURE_SHADES[(figure.shade ?? 0) % FIGURE_SHADES.length]);

const HANDLE_FILL = '#2563eb';
const HANDLE_STROKE = '#ffffff';

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
