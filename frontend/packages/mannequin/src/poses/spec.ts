// How a pose is written, and how a written pose becomes joints. The specs
// themselves live in `library.ts`; this is the grammar.
import { BONE, JOINT_KEYS, TORSO_HEIGHT_RATIO, bodyForwardOf, derivedFace, type JointKey } from '../skeleton';
import { add3, cross3, norm3, rad, rotateAxis, sub3, zOf, type Vec3 } from '../vec3';

export type PresetPoints = Record<JointKey, readonly [number, number, number]>;

// Width of the unit box relative to its height. Arms out to the side need more
// room than a body is wide — and the box has to hold the *drawn* figure, not
// just its joints: the deltoid and the shoulder ball both sit outboard of the
// shoulder joint, so a pose with its arms straight up is wider than its
// skeleton. Changing this only changes how much room is reserved; bone lengths
// are unaffected, because the same factor divides out when a preset is
// normalised (see `POSE_SCALE * FIGURE_ASPECT` below).
export const FIGURE_ASPECT = 0.48;


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
export type Angle = number | readonly [number, number];

const angleOfBone = (angle: Angle): readonly [number, number] => (
    typeof angle === 'number' ? [angle, 0] : angle
);

export interface PoseSpec {
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


export const buildPose = (spec: PoseSpec): PresetPoints => {
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
