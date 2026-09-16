// The joints, what hangs off what, and the one skeleton every pose is built
// from. Pure data and a few derivations; nothing here knows about a canvas.
import { add3, cross3, dist3, dot3, len3, mul3, norm3, sub3, type Vec3 } from './vec3';

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
export const HEAD_RADIUS_RATIO = 0.07;

// Every thickness in the manikin is a fraction of "how big is this person",
// and that number must NOT be the bounding box: a lying figure has the same
// body as a standing one but a fifth of the box, which would shrink its limbs
// into the stick figure this module exists to avoid. Nor can it be a projected
// length — a torso turned away from the camera is foreshortened to nothing and
// would take the whole body's thickness down with it. The torso bone measured
// *in 3D* is the one number no pose and no camera angle can change.
export const TORSO_HEIGHT_RATIO = 0.36;


export const figureTurn = (figure: PoseFigure): FigureTurn => figure.turn ?? { yaw: 0, pitch: 0 };

export const figureUnit = (figure: PoseFigure): number => {
    // hip → neck, the bone itself, in three dimensions. The hip *line's*
    // midpoint drifts with the torso's lean (the stubs to hipL/hipR rotate
    // with it), which would make the same body measure differently lying down
    // than standing up.
    const torso = dist3(figure.joints.neck, figure.joints.hip);
    return Math.max(torso / TORSO_HEIGHT_RATIO, 1);
};


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

export const SUBTREES = Object.fromEntries(JOINT_KEYS.map((key) => [key, subtreeOf(key)])) as Record<JointKey, JointKey[]>;


// One skeleton for every pose, in arbitrary units — the result is normalised.
export const BONE = {
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


// Which way the body faces: across the shoulders, crossed with the spine.
export const bodyForwardOf = (joints: Pick<Record<JointKey, Vec3>, 'neck' | 'hip' | 'shoulderL' | 'shoulderR'>): Vec3 => {
    const forward = cross3(sub3(joints.neck, joints.hip), sub3(joints.shoulderR, joints.shoulderL));
    return len3(forward) < 1e-9 ? { x: 0, y: 0, z: 1 } : norm3(forward);
};

// The part of `forward` that survives once the component along `axis` is taken
// out — "point this the way the body faces, but keep it square to the limb it
// hangs off". A raised foot carries its toes round with it this way, and a
// tipped head keeps its face on the front of the skull.
export const squareTo = (forward: Vec3, axis: Vec3): Vec3 => {
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
