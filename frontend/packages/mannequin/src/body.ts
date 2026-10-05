import { MODEL } from './model/makehuman';
import { figureBuild, figureUnit, squareTo, type FigureBuild, type JointKey, type PoseFigure } from './skeleton';
import { cross3, dot3, len3, mul3, norm3, sub3, add3, type Vec3 } from './vec3';

// --- the white model ---------------------------------------------------------
//
// What the figure looks like is a real human base mesh, not something built
// here. It used to be: first a stack of spheres and cones, then hand-tuned
// sweeps — and both read as a toy, because sculpting a body is a craft and a
// table of radii is not one. The mesh is MakeHuman's (CC0), baked offline by
// `scripts/bake-makehuman.mjs` into one female and one male body at
// MakeHuman's "ideal proportions", the artistic canon a drawing mannequin is
// meant to show.
//
// The rig stays ours. Sixteen joints still define the pose; the mesh is
// skinned onto twelve segments (pelvis, ribcage, neck, head, and two per
// limb) and each segment follows the frame its joints define. Hands ride the
// forearm and feet the shin, as on any jointed mannequin.
//
// This surface is the *only* description of the body: the renderer draws it,
// hit testing projects it, so what you can grab cannot drift from what you
// can see.

type SegmentName = (typeof MODEL.segments)[number];

interface Decoded {
    index: Uint16Array | Uint32Array;
    segments: Uint8Array;
    weights: Uint8Array;
    rest: Record<FigureBuild, { positions: Float32Array; joints: Record<JointKey, Vec3>; frames: Frame[] }>;
}

const bytes = (base64: string): Uint8Array => {
    const raw = atob(base64);
    const out = new Uint8Array(raw.length);
    for (let i = 0; i < raw.length; i += 1) out[i] = raw.charCodeAt(i);
    return out;
};

interface Frame { origin: Vec3; axis: Vec3; across: Vec3; forward: Vec3; length: number }

// --- frames: how each segment is oriented, from joints alone -----------------
//
// Computed the same way for the rest pose and for any figure, so the
// rotation between the two is exactly the rotation the joints describe.
// Twist about a limb is read off the joint it bends at — a bent elbow says
// which way the forearm faces — and falls back to the body's front while the
// limb is straight, blended rather than switched so nothing flips.
const orthTo = (v: Vec3, axis: Vec3): Vec3 => sub3(v, mul3(axis, dot3(v, axis)));
const frameOf = (origin: Vec3, tip: Vec3, reference: Vec3, fallback: Vec3): Frame => {
    const along = sub3(tip, origin);
    const length = len3(along);
    const axis = length > 1e-9 ? mul3(along, 1 / length) : { x: 0, y: 1, z: 0 };
    let ref = add3(orthTo(reference, axis), mul3(orthTo(fallback, axis), 0.3));
    if (len3(ref) < 1e-6) ref = orthTo({ x: 0, y: 0, z: 1 }, axis);
    if (len3(ref) < 1e-6) ref = orthTo({ x: 1, y: 0, z: 0 }, axis);
    const forward = norm3(ref);
    return { origin, axis, forward, across: norm3(cross3(forward, axis)), length };
};

const SEGMENT_ORDER = MODEL.segments as readonly SegmentName[];

export const segmentFrames = (J: Record<JointKey, Vec3>): Frame[] => {
    const up = norm3(sub3(J.neck, J.hip));
    const flat = (v: Vec3) => {
        const f = orthTo(v, up);
        return len3(f) > 1e-9 ? norm3(f) : { x: 1, y: 0, z: 0 };
    };
    const pelvisAcross = flat(sub3(J.hipR, J.hipL));
    const chestAcross = flat(sub3(J.shoulderR, J.shoulderL));
    const pelvisFront = norm3(cross3(up, pelvisAcross));
    const chestFront = norm3(cross3(up, chestAcross));
    const torso = len3(sub3(J.neck, J.hip));
    const trunk = (origin: Vec3, front: Vec3): Frame => ({ origin, axis: up, forward: front, across: norm3(cross3(front, up)), length: torso });
    const faceDir = sub3(J.face, J.head);
    const frames: Record<SegmentName, Frame> = {
        pelvis: trunk(J.hip, pelvisFront),
        chest: trunk(J.neck, chestFront),
        neck: frameOf(J.neck, J.head, chestFront, chestFront),
        head: frameOf(J.neck, J.head, faceDir, chestFront),
    } as Record<SegmentName, Frame>;
    // The head turns about its own centre, not the base of the neck.
    frames.head = { ...frames.head, origin: J.head };
    for (const side of ['L', 'R'] as const) {
        const shoulder = J[`shoulder${side}`], elbow = J[`elbow${side}`], wrist = J[`wrist${side}`];
        const hip = J[`hip${side}`], knee = J[`knee${side}`], ankle = J[`ankle${side}`];
        const upper = norm3(sub3(elbow, shoulder));
        const fore = norm3(sub3(wrist, elbow));
        const thigh = norm3(sub3(knee, hip));
        const shin = norm3(sub3(ankle, knee));
        // The forearm folds toward the front of the upper arm; the shin folds
        // toward the back of the thigh.
        frames[`upperArm${side}`] = frameOf(shoulder, elbow, fore, chestFront);
        frames[`foreArm${side}`] = frameOf(elbow, wrist, mul3(upper, -1), chestFront);
        frames[`thigh${side}`] = frameOf(hip, knee, mul3(shin, -1), pelvisFront);
        frames[`shin${side}`] = frameOf(knee, ankle, thigh, pelvisFront);
    }
    return SEGMENT_ORDER.map((name) => frames[name]);
};

let decoded: Decoded | null = null;
const model = (): Decoded => {
    if (decoded) return decoded;
    const indexBytes = bytes(MODEL.index.data);
    const index = (MODEL.index.bits as number) === 32
        ? new Uint32Array(indexBytes.buffer, indexBytes.byteOffset, indexBytes.byteLength / 4)
        : new Uint16Array(indexBytes.buffer, indexBytes.byteOffset, indexBytes.byteLength / 2);
    const rest = {} as Decoded['rest'];
    for (const build of ['female', 'male'] as const) {
        const b = MODEL.builds[build];
        const q = bytes(b.positions);
        const ints = new Int16Array(q.buffer, q.byteOffset, q.byteLength / 2);
        const positions = new Float32Array(ints.length);
        for (let i = 0; i < ints.length; i += 1) {
            const k = i % 3;
            positions[i] = b.lo[k] + ((ints[i] + 32768) / 65535) * (b.hi[k] - b.lo[k]);
        }
        const joints = {} as Record<JointKey, Vec3>;
        for (const [key, p] of Object.entries(b.joints)) joints[key as JointKey] = { x: p[0], y: p[1], z: p[2] };
        joints.face = add3(joints.head, { x: 0, y: 0, z: 0.075 });
        rest[build] = { positions, joints, frames: segmentFrames(joints) };
    }
    decoded = { index, segments: bytes(MODEL.skin.segments), weights: bytes(MODEL.skin.weights), rest };
    return decoded;
};

export interface Surface {
    // World positions (canvas px, y down, z toward the viewer), xyz per vertex.
    positions: Float32Array;
    index: Uint16Array | Uint32Array;
}

// Skinning: each vertex is carried by up to four segments, each moving it by
// the rigid motion from its rest frame to its posed frame (stretched along
// the bone when a figure's bone is longer than the model's — a sketch saved
// on another skeleton still gets a body that meets its joints).
const surfaces = new WeakMap<PoseFigure, Surface>();
export const figureSurface = (figure: PoseFigure): Surface => {
    const cached = surfaces.get(figure);
    if (cached) return cached;
    const m = model();
    const rest = m.rest[figureBuild(figure)];
    const u = figureUnit(figure);
    const posed = segmentFrames(figure.joints);
    // Per segment, a 3×4 matrix: world = origin + R_posed · S · R_restᵀ · (v − rest origin), in units of u.
    const mats = posed.map((frame, s) => {
        const r = rest.frames[s];
        const stretch = s >= 4 && r.length > 1e-9 ? frame.length / (r.length * u) : 1;
        const P = [frame.across, frame.axis, frame.forward];
        const R = [r.across, r.axis, r.forward];
        const scale = [u, u * stretch, u];
        // M = Σ_k P_k ⊗ R_k · scale_k
        const M = new Float64Array(12);
        for (let row = 0; row < 3; row += 1) {
            for (let col = 0; col < 3; col += 1) {
                let sum = 0;
                for (let k = 0; k < 3; k += 1) {
                    const p = [P[k].x, P[k].y, P[k].z ?? 0][row];
                    const q = [R[k].x, R[k].y, R[k].z ?? 0][col];
                    sum += p * q * scale[k];
                }
                M[row * 4 + col] = sum;
            }
        }
        const o = [r.origin.x, r.origin.y, r.origin.z ?? 0];
        const t = [frame.origin.x, frame.origin.y, frame.origin.z ?? 0];
        for (let row = 0; row < 3; row += 1) {
            M[row * 4 + 3] = t[row] - (M[row * 4] * o[0] + M[row * 4 + 1] * o[1] + M[row * 4 + 2] * o[2]);
        }
        return M;
    });
    const src = rest.positions;
    const count = src.length / 3;
    const out = new Float32Array(src.length);
    for (let i = 0; i < count; i += 1) {
        const x = src[i * 3], y = src[i * 3 + 1], z = src[i * 3 + 2];
        let ox = 0, oy = 0, oz = 0;
        for (let k = 0; k < 4; k += 1) {
            const w = m.weights[i * 4 + k];
            if (w === 0) continue;
            const M = mats[m.segments[i * 4 + k]];
            const f = w / 255;
            ox += f * (M[0] * x + M[1] * y + M[2] * z + M[3]);
            oy += f * (M[4] * x + M[5] * y + M[6] * z + M[7]);
            oz += f * (M[8] * x + M[9] * y + M[10] * z + M[11]);
        }
        out[i * 3] = ox; out[i * 3 + 1] = oy; out[i * 3 + 2] = oz;
    }
    const surface = { positions: out, index: m.index };
    surfaces.set(figure, surface);
    return surface;
};

// The canon's unit of measure: crown to chin, as a fraction of the figure's
// height unit, read off the male model at rest — the crown is the top of the
// mesh, the chin the lowest point the skull carries in front of the neck.
export const HEAD_LENGTH_RATIO = (() => {
    const m = model();
    const { positions, joints } = m.rest.male;
    const head = SEGMENT_ORDER.indexOf('head');
    let crown = Infinity, chin = -Infinity;
    for (let i = 0; i < positions.length / 3; i += 1) {
        const y = positions[i * 3 + 1];
        crown = Math.min(crown, y);
        const onSkull = m.segments[i * 4] === head && m.weights[i * 4] > 240;
        if (onSkull && positions[i * 3 + 2] > joints.head.z) chin = Math.max(chin, y);
    }
    return chin - crown;
})();

void squareTo;

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

