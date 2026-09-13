// Turning a body found in a photograph into one of our mannequins.
//
// This module deliberately knows nothing about MediaPipe, TensorFlow, or any
// other estimator. It takes 33 landmarks in the shape every BlazePose-family
// model emits and hands back a `PoseFigure`. That boundary is the whole point:
// which model (if any) we ship, how big it is, and where it is downloaded from
// are open questions, and none of them should be able to reach into the
// mannequin. Anything here can be unit-tested without a model, a camera or a
// network — see `landmarkPose.test.ts`, which round-trips our own pose library
// through it.
//
// See `.design/pose-from-image.md` for why the landmark set is the one we want
// and what is still undecided.

import {
    centerFigureAt,
    completeFigure,
    figureCenter,
    figureUnit,
    JOINT_KEYS,
    JOINT_PARENT,
    scaleFigure,
    type FigureTurn,
    type JointKey,
    type PoseFigure,
    type Vec3,
} from './poseFigure';

// The 33 landmarks, by the index every BlazePose-family model uses. Named here
// once so nothing downstream has to remember that 31 is a toe.
export const LANDMARK = {
    nose: 0,
    eyeInnerL: 1, eyeL: 2, eyeOuterL: 3,
    eyeInnerR: 4, eyeR: 5, eyeOuterR: 6,
    earL: 7, earR: 8,
    mouthL: 9, mouthR: 10,
    shoulderL: 11, shoulderR: 12,
    elbowL: 13, elbowR: 14,
    wristL: 15, wristR: 16,
    pinkyL: 17, pinkyR: 18,
    indexL: 19, indexR: 20,
    thumbL: 21, thumbR: 22,
    hipL: 23, hipR: 24,
    kneeL: 25, kneeR: 26,
    ankleL: 27, ankleR: 28,
    heelL: 29, heelR: 30,
    toeL: 31, toeR: 32,
} as const;

export const LANDMARK_COUNT = 33;

export interface Landmark {
    // Normalized to the image: x by its width, y by its height, both 0..1.
    x: number;
    y: number;
    // Depth in roughly the same scale as x, origin at the midpoint of the hips,
    // and *smaller is nearer the camera* — the opposite of our own z, which is
    // why `toWorld` negates it.
    z: number;
    // How sure the estimator is that this landmark is visible and unoccluded.
    // Absent on some runtimes, in which case everything is trusted.
    visibility?: number;
}

export interface LandmarkFrame {
    width: number;
    height: number;
}

export interface RetargetOptions {
    // The image the landmarks came from. Needed because x and y are normalized
    // by *different* numbers: on a 16:9 photo, x = 0.1 and y = 0.1 are not the
    // same distance, and a body built from them straight comes out squashed.
    frame: LandmarkFrame;
    // A landmark the estimator is unsure about is worse than no landmark: it
    // produces a limb pointing somewhere the body never went. Below this, the
    // bone keeps the direction it already had.
    minVisibility?: number;
    // The mannequin to match: its bone lengths, its size and its place on the
    // canvas. Bone lengths are the important one — see `retargetToBones`.
    reference: PoseFigure;
}

export interface RetargetResult {
    figure: PoseFigure;
    // Joints whose landmarks were missing or not trusted, and which therefore
    // kept the reference's own direction. Worth surfacing: "we could not see
    // the left arm" is a thing the user needs told, not a thing to paper over.
    fellBack: JointKey[];
    // Mean visibility across the landmarks that were used, 0..1.
    confidence: number;
}

const DEFAULT_MIN_VISIBILITY = 0.5;

// --- vectors -----------------------------------------------------------------

const sub = (a: Vec3, b: Vec3): Vec3 => ({ x: a.x - b.x, y: a.y - b.y, z: a.z - b.z });
const add = (a: Vec3, b: Vec3): Vec3 => ({ x: a.x + b.x, y: a.y + b.y, z: a.z + b.z });
const mul = (a: Vec3, k: number): Vec3 => ({ x: a.x * k, y: a.y * k, z: a.z * k });
const mid = (a: Vec3, b: Vec3): Vec3 => mul(add(a, b), 0.5);
const len = (a: Vec3): number => Math.hypot(a.x, a.y, a.z);
const dot = (a: Vec3, b: Vec3): number => a.x * b.x + a.y * b.y + a.z * b.z;
const cross = (a: Vec3, b: Vec3): Vec3 => ({
    x: a.y * b.z - a.z * b.y,
    y: a.z * b.x - a.x * b.z,
    z: a.x * b.y - a.y * b.x,
});
const norm = (a: Vec3): Vec3 => {
    const length = len(a);
    return length < 1e-9 ? { x: 0, y: 0, z: 0 } : mul(a, 1 / length);
};

// --- the frame ---------------------------------------------------------------

// Landmarks into our own space: x and z stretched back out by the aspect ratio
// so a unit in x is a unit in y, and z flipped, because the estimator counts
// depth *away* from the camera while we count it toward the viewer.
const toWorld = (landmark: Landmark, aspect: number): Vec3 => ({
    x: landmark.x * aspect,
    y: landmark.y,
    z: -landmark.z * aspect,
});

// Anatomical left is not screen left. The estimator names the *subject's* own
// sides, while our joints are named for where they sit on a figure facing the
// viewer — so a person's left shoulder is our `shoulderR`. Getting this
// backwards produces a figure that is subtly, unfixably wrong (every pose
// mirrored) and looks almost right, so it is stated once, here.
const SIDE_OF: Partial<Record<JointKey, number>> = {
    shoulderL: LANDMARK.shoulderR, shoulderR: LANDMARK.shoulderL,
    elbowL: LANDMARK.elbowR, elbowR: LANDMARK.elbowL,
    wristL: LANDMARK.wristR, wristR: LANDMARK.wristL,
    hipL: LANDMARK.hipR, hipR: LANDMARK.hipL,
    kneeL: LANDMARK.kneeR, kneeR: LANDMARK.kneeL,
    ankleL: LANDMARK.ankleR, ankleR: LANDMARK.ankleL,
    // The sixteenth joint, and the one a photograph is uniquely good for:
    // our face joint sits in front of the skull, and so does a nose.
    face: LANDMARK.nose,
};

const visible = (landmark: Landmark | undefined, floor: number): boolean =>
    landmark !== undefined && (landmark.visibility ?? 1) >= floor;

// --- reading the body out of the landmarks -----------------------------------

interface RawRead {
    points: Partial<Record<JointKey, Vec3>>;
    seen: Partial<Record<JointKey, number>>;
}

// How far our neck sits above the shoulder line, and our hip root above the
// hip line, measured on the reference itself rather than hard-coded — the two
// numbers live in `poseFigure`'s BONE table and have no business being copied
// here.
interface TorsoOffsets { shoulder: number; hip: number }

const torsoOffsetsOf = (reference: PoseFigure): TorsoOffsets => {
    const up = norm(sub(reference.joints.neck, reference.joints.hip));
    const shoulderMid = mid(reference.joints.shoulderL, reference.joints.shoulderR);
    const hipMid = mid(reference.joints.hipL, reference.joints.hipR);
    return {
        shoulder: dot(sub(reference.joints.neck, shoulderMid), up),
        hip: dot(sub(reference.joints.hip, hipMid), up),
    };
};

// Where our fifteen joints sit in the landmark cloud. Ten are a landmark each.
// The other five are the interesting part, because the landmark set does not
// contain them:
//
// - There is **no neck landmark and no pelvis root**. Our skeleton hangs the
//   shoulders below a neck joint and the hip line below a hip root, so taking
//   the midpoints straight would shorten the torso at both ends and tilt every
//   bone hanging off it. Both are walked back up the torso axis by the offset
//   the reference mannequin itself uses.
// - There is **no crown landmark**. The ears are the closest thing to the
//   centre of the skull, which is where our head joint lives.
//
// This is the one place the mapping is an estimate rather than a rename, and
// it is an estimate of two small constants, not of the pose.
const readJoints = (
    landmarks: readonly Landmark[],
    aspect: number,
    floor: number,
    offsets: TorsoOffsets,
): RawRead => {
    const at = (index: number): Vec3 => toWorld(landmarks[index], aspect);
    const ok = (index: number): boolean => visible(landmarks[index], floor);
    const seenOf = (...indices: number[]): number => Math.min(
        ...indices.map((index) => landmarks[index].visibility ?? 1),
    );

    const points: Partial<Record<JointKey, Vec3>> = {};
    const seen: Partial<Record<JointKey, number>> = {};

    for (const key of JOINT_KEYS) {
        const index = SIDE_OF[key];
        if (index !== undefined && ok(index)) {
            points[key] = at(index);
            seen[key] = landmarks[index].visibility ?? 1;
        }
    }

    const hasHips = ok(LANDMARK.hipL) && ok(LANDMARK.hipR);
    const hasShoulders = ok(LANDMARK.shoulderL) && ok(LANDMARK.shoulderR);
    if (hasHips && hasShoulders) {
        const hipMid = mid(at(LANDMARK.hipL), at(LANDMARK.hipR));
        const shoulderMid = mid(at(LANDMARK.shoulderL), at(LANDMARK.shoulderR));
        const up = norm(sub(shoulderMid, hipMid));
        points.hip = add(hipMid, mul(up, offsets.hip));
        points.neck = add(shoulderMid, mul(up, offsets.shoulder));
        seen.hip = seenOf(LANDMARK.hipL, LANDMARK.hipR);
        seen.neck = seenOf(LANDMARK.shoulderL, LANDMARK.shoulderR);
    }
    if (ok(LANDMARK.earL) && ok(LANDMARK.earR)) {
        points.head = mid(at(LANDMARK.earL), at(LANDMARK.earR));
        seen.head = seenOf(LANDMARK.earL, LANDMARK.earR);
    } else if (ok(LANDMARK.nose)) {
        // Faces are lost to profile and to hair long before the nose is. Fall
        // back to it: the head bone's length is ours anyway (see
        // `retargetToBones`), so only its direction has to be roughly right.
        points.head = at(LANDMARK.nose);
        seen.head = landmarks[LANDMARK.nose].visibility ?? 1;
    }

    return { points, seen };
};

// Which way the body faces, from the shoulders crossed with the spine. The
// same construction the renderer uses, so "front" means one thing in this
// codebase.
export const bodyForward = (points: Partial<Record<JointKey, Vec3>>): Vec3 | null => {
    const { neck, hip, shoulderL, shoulderR } = points;
    if (!neck || !hip || !shoulderL || !shoulderR) return null;
    const forward = cross(sub(neck, hip), sub(shoulderR, shoulderL));
    return len(forward) < 1e-9 ? null : norm(forward);
};

// A body facing the camera has its nose in front of its ears. That one fact is
// enough to catch a depth axis that came back the other way round — from a
// runtime that signs z differently, from a mirrored selfie, or from our own
// misreading of a spec that does not actually state the direction. Cheaper and
// more honest than asserting a convention we cannot verify from the docs.
export const depthSignOf = (landmarks: readonly Landmark[], aspect: number, points: Partial<Record<JointKey, Vec3>>): number => {
    const forward = bodyForward(points);
    if (!forward) return 1;
    const nose = landmarks[LANDMARK.nose];
    const earL = landmarks[LANDMARK.earL];
    const earR = landmarks[LANDMARK.earR];
    if (!nose || !earL || !earR) return 1;
    const ahead = dot(sub(toWorld(nose, aspect), mid(toWorld(earL, aspect), toWorld(earR, aspect))), forward);
    // Near zero means the head is in profile and the test says nothing; leave
    // the depth as read rather than flipping the body on a coin toss.
    if (Math.abs(ahead) < 1e-6) return 1;
    return ahead > 0 ? 1 : -1;
};

// --- keeping our own body ----------------------------------------------------

// Parents before children, so a bone can be hung off a joint that has already
// been placed.
const WALK_ORDER: JointKey[] = (() => {
    const order: JointKey[] = [];
    const placed = new Set<JointKey>();
    while (order.length < JOINT_KEYS.length) {
        for (const key of JOINT_KEYS) {
            if (placed.has(key)) continue;
            const parent = JOINT_PARENT[key];
            if (parent === null || placed.has(parent)) {
                order.push(key);
                placed.add(key);
            }
        }
    }
    return order;
})();

const boneLengthsOf = (figure: PoseFigure): Partial<Record<JointKey, number>> => {
    const lengths: Partial<Record<JointKey, number>> = {};
    for (const key of JOINT_KEYS) {
        const parent = JOINT_PARENT[key];
        if (parent) lengths[key] = len(sub(figure.joints[key], figure.joints[parent]));
    }
    return lengths;
};

// The heart of it: take the *direction* of every bone from the photograph and
// its *length* from our own mannequin.
//
// Not an approximation to be improved later — the point. Using the estimator's
// lengths would mean every photo produced a differently proportioned person,
// applying a preset afterwards would resize them (the pose library is built on
// one skeleton), and the per-frame length jitter every estimator has would
// arrive with them. What a photograph is good for is the angles.
export const retargetToBones = (
    points: Partial<Record<JointKey, Vec3>>,
    reference: PoseFigure,
): { joints: Record<JointKey, Vec3>; fellBack: JointKey[] } => {
    const lengths = boneLengthsOf(reference);
    const joints = {} as Record<JointKey, Vec3>;
    const fellBack: JointKey[] = [];

    for (const key of WALK_ORDER) {
        const parent = JOINT_PARENT[key];
        if (parent === null) {
            // The root starts where the reference's root already is, depth
            // included. Starting at the origin instead leaves the rebuilt
            // figure sitting at a different z — invisible, because each figure
            // is projected about its own torso, but enough to make it scale
            // about the wrong point later and to make "same pose" compare
            // unequal for no reason.
            joints[key] = { ...reference.joints[key] };
            continue;
        }
        const bone = lengths[key] ?? 0;
        const from = points[parent];
        const to = points[key];
        const direction = from && to ? sub(to, from) : null;
        if (!direction || len(direction) < 1e-9) {
            // Nothing trustworthy to point at: keep the limb where the
            // reference already had it rather than collapsing it onto its
            // parent, which would read as an amputation.
            fellBack.push(key);
            joints[key] = add(joints[parent], sub(reference.joints[key], reference.joints[parent]));
            continue;
        }
        joints[key] = add(joints[parent], mul(norm(direction), bone));
    }

    return { joints, fellBack };
};

// --- which way it is being looked at -----------------------------------------

const toDegrees = (radians: number) => (radians * 180) / Math.PI;

// The camera angle the photograph was taken from, in the same two numbers the
// turn control shows. Recorded rather than zeroed: a figure lifted from a
// three-quarter photograph *is* at three-quarters, and a readout saying 0°
// would be a lie the moment the user opened the view menu.
//
// Pitch comes from the spine and yaw from the body's forward, so neither
// depends on the other. Any roll the photograph had stays in the joints, which
// is what we want — turning an imported figure should move the camera, not
// quietly stand the subject up straight.
export const turnOfBody = (joints: Record<JointKey, Vec3>): FigureTurn => {
    const up = norm(sub(joints.neck, joints.hip));
    const forward = bodyForward(joints) ?? { x: 0, y: 0, z: 1 };
    return {
        yaw: toDegrees(Math.atan2(forward.x, forward.z)),
        pitch: toDegrees(Math.atan2(-up.z, -up.y)),
    };
};

// --- the whole job -----------------------------------------------------------

export const figureFromLandmarks = (
    landmarks: readonly Landmark[],
    options: RetargetOptions,
): RetargetResult | null => {
    if (landmarks.length < LANDMARK_COUNT) return null;
    const floor = options.minVisibility ?? DEFAULT_MIN_VISIBILITY;
    const aspect = options.frame.height > 0 ? options.frame.width / options.frame.height : 1;

    const reference = completeFigure(options.reference);
    const offsets = torsoOffsetsOf(reference);
    const first = readJoints(landmarks, aspect, floor, offsets);
    // Without a torso there is no body to hang anything off, and no frame to
    // read a camera angle from. Better to say "no pose here" than to invent one.
    if (!first.points.hip || !first.points.neck) return null;

    // Re-read with the depth axis confirmed against the nose, if it disagreed.
    const sign = depthSignOf(landmarks, aspect, first.points);
    const read = sign > 0 ? first : readJoints(
        landmarks.map((landmark) => ({ ...landmark, z: -landmark.z })),
        aspect,
        floor,
        offsets,
    );

    const { joints, fellBack } = retargetToBones(read.points, reference);
    const turn = turnOfBody(joints);

    const raw: PoseFigure = {
        id: options.reference.id,
        joints,
        shade: options.reference.shade,
        turn,
    };
    // Our body, our size, where the figure already was: importing a pose
    // changes the pose, not who the person is or where they stand.
    const sized = scaleFigure(raw, figureUnit(reference) / figureUnit(raw));
    const placed = centerFigureAt(sized, figureCenter(reference));

    const used = JOINT_KEYS.map((key) => read.seen[key]).filter((v): v is number => v !== undefined);
    return {
        figure: placed,
        fellBack,
        confidence: used.length > 0 ? used.reduce((sum, v) => sum + v, 0) / used.length : 0,
    };
};

// --- going the other way -----------------------------------------------------

// Landmarks as a mannequin would produce them. Only the twenty-one landmarks
// our skeleton actually implies are real; the rest are filled in so the array
// is the right shape. Exists so the retarget can be tested — and, in time,
// calibrated — against poses whose correct answer we already know, without a
// model, a photograph, or a network.
export const landmarksFromFigure = (figure: PoseFigure, frame: LandmarkFrame): Landmark[] => {
    const aspect = frame.height > 0 ? frame.width / frame.height : 1;
    const out: Landmark[] = Array.from({ length: LANDMARK_COUNT }, () => ({ x: 0, y: 0, z: 0, visibility: 0 }));
    const put = (index: number, point: Vec3, visibility = 1) => {
        out[index] = {
            x: point.x / aspect,
            y: point.y,
            z: -point.z / aspect,
            visibility,
        };
    };

    const whole = completeFigure(figure);
    for (const key of JOINT_KEYS) {
        const index = SIDE_OF[key];
        if (index !== undefined) put(index, whole.joints[key]);
    }

    const across = norm(sub(whole.joints.shoulderR, whole.joints.shoulderL));
    const unit = figureUnit(whole);
    // The ears straddle the head joint; the nose is the face joint, already
    // written above. Together they are the three points the depth check needs.
    put(LANDMARK.earL, add(whole.joints.head, mul(across, unit * 0.045)));
    put(LANDMARK.earR, add(whole.joints.head, mul(across, -unit * 0.045)));

    return out;
};
