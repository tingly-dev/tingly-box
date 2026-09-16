import { SUBTREES, bodyForwardOf, type JointKey, type PoseFigure } from './skeleton';
import { add3, cross3, dot3, len3, mul3, norm3, sub3, type Vec3 } from './vec3';

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
