import { figureBuild, figureUnit, bodyForwardOf, squareTo, type FigureBuild, type PoseFigure } from './skeleton';
import { add3, cross3, dot3, len3, lerp3, mul3, norm3, sub3, type Vec3 } from './vec3';

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
//
// Two builds, one shape language. Both are the same dozen spheres, tapers and
// two turned blocks; what changes is proportion, and only where an art-school
// mannequin changes it:
//
// - male: the chest is the widest thing on the body and tapers hard to the
//   waist (the V); the pelvis is narrower than the shoulders; neck, arms and
//   joints a size up.
// - female: shoulders and hips close to the same width; a narrower waist; the
//   pelvis wider and deeper; slimmer neck, arms and wrists, fuller thighs; and
//   two forms the male build has none of — the bust on the chest block and
//   the gluteal mass behind the pelvis. Both kept at reference-mannequin size:
//   enough that a silhouette reads as female from any angle, not more. The
//   model is told who the person is by the prompt; the mannequin only has to
//   carry the structure.
export interface ManikinShape {
    head: { wide: number; long: number; deep: number };
    neck: number;
    upperArm: number; elbow: number; wrist: number;
    thigh: number; knee: number; calf: number; ankle: number;
    shoulderBall: number; hipBall: number; waistBall: number;
    hand: number;
    foot: { long: number; wide: number };
    // Both blocks are turned from a profile — (height along the axis, half
    // width), as fractions of the figure's height, measured from the block's
    // base. The chest runs from the waist up to the neck, widest at the
    // shoulder line and closing just above it: shoulders are the *top* of a
    // manikin's chest, not a ledge under the neck. The pelvis is a bucket,
    // widest at the crest and closing onto the hip joints.
    chest: { base: number; depth: number; profile: readonly (readonly [number, number])[] };
    pelvis: { base: number; depth: number; profile: readonly (readonly [number, number])[] };
    waist: number;
    // Secondary forms on the blocks, or null for none. `height` is measured up
    // the block from its base, `spread` out from the midline, `out` from the
    // spine toward the front (bust) or the back (glutes); all in body height.
    bust: { height: number; spread: number; out: number; radius: number; flat: number } | null;
    glutes: { height: number; spread: number; out: number; radius: number; flat: number } | null;
}

export const MANIKINS: Record<FigureBuild, ManikinShape> = {
    male: {
        head: { wide: 0.058, long: 0.066, deep: 0.061 },
        neck: 0.030,
        upperArm: 0.033, elbow: 0.028, wrist: 0.021,
        thigh: 0.043, knee: 0.031, calf: 0.035, ankle: 0.024,
        shoulderBall: 0.031, hipBall: 0.028, waistBall: 0.031,
        // A hand is a small ball just past the wrist, not a blade: the blade
        // read as a spike. A foot is a rounded pad pointing the way the body
        // faces.
        hand: 0.027,
        foot: { long: 0.065, wide: 0.026 },
        chest: {
            base: 0.32, depth: 0.58,
            profile: [[0, 0], [0.006, 0.042], [0.05, 0.060], [0.10, 0.077], [0.15, 0.090], [0.20, 0.099], [0.225, 0.097], [0.24, 0.071], [0.248, 0]],
        },
        pelvis: {
            base: -0.13, depth: 0.62,
            profile: [[0, 0], [0.008, 0.044], [0.03, 0.068], [0.06, 0.076], [0.085, 0.073], [0.11, 0.058], [0.13, 0.032], [0.14, 0]],
        },
        waist: 0.275,
        bust: null,
        glutes: null,
    },
    female: {
        head: { wide: 0.055, long: 0.066, deep: 0.058 },
        neck: 0.023,
        upperArm: 0.026, elbow: 0.022, wrist: 0.016,
        thigh: 0.046, knee: 0.027, calf: 0.031, ankle: 0.019,
        shoulderBall: 0.024, hipBall: 0.031, waistBall: 0.025,
        hand: 0.022,
        foot: { long: 0.056, wide: 0.021 },
        chest: {
            base: 0.33, depth: 0.58,
            profile: [[0, 0], [0.006, 0.034], [0.05, 0.047], [0.10, 0.061], [0.15, 0.072], [0.20, 0.080], [0.222, 0.078], [0.236, 0.056], [0.244, 0]],
        },
        pelvis: {
            base: -0.13, depth: 0.66,
            profile: [[0, 0], [0.012, 0.050], [0.03, 0.078], [0.055, 0.089], [0.08, 0.088], [0.105, 0.078], [0.125, 0.060], [0.14, 0.036], [0.15, 0]],
        },
        waist: 0.285,
        bust: { height: 0.150, spread: 0.034, out: 0.036, radius: 0.032, flat: 0.85 },
        glutes: { height: 0.040, spread: 0.030, out: 0.036, radius: 0.042, flat: 0.80 },
    },
};

// The legacy table, for whoever only needs a size both builds share
// (`HEAD_LENGTH_RATIO`) or the build every pre-build sketch already is.
export const MANIKIN: ManikinShape = MANIKINS.male;

export const manikinFor = (figure: PoseFigure): ManikinShape => MANIKINS[figureBuild(figure)];

// The canon's unit of measure: crown to chin, as a fraction of the figure's
// height. Eight of these is the whole body, two of them the shoulders. Head
// length is the one size both builds share exactly, so this is one number.
export const HEAD_LENGTH_RATIO = MANIKIN.head.long * 2;

export type Solid =
    | { kind: 'sphere'; center: Vec3; radius: number; scale?: Vec3; axis?: Vec3 }
    | { kind: 'capsule'; from: Vec3; to: Vec3; fromRadius: number; toRadius: number }
    | { kind: 'block'; base: Vec3; axis: Vec3; profile: readonly (readonly [number, number])[]; unit: number; depth: number };

// Radii are in world pixels here; the renderer and the hit test both scale
// them by perspective the same way they scale positions.
export const figureSolids = (figure: PoseFigure): Solid[] => {
    const J = figure.joints;
    const u = figureUnit(figure);
    const M = manikinFor(figure);
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

    // The two secondary forms sit on the blocks, in the blocks' own frame:
    // up the spine, across the shoulders (or hips) squared to it, and out
    // along the body's front. Hung off the same three axes the blocks are
    // turned on, they go wherever the torso goes — bent, twisted, lying down.
    const front = squareTo(bodyForwardOf(J), spine);
    const acrossOf = (left: Vec3, right: Vec3): Vec3 => {
        const span = sub3(right, left);
        const flat = sub3(span, mul3(spine, dot3(span, spine)));
        return len3(flat) > 1e-6 ? norm3(flat) : norm3(cross3(spine, front));
    };
    const pair = (
        form: NonNullable<ManikinShape['bust']>,
        base: Vec3,
        across: Vec3,
        facing: Vec3,
    ) => {
        for (const side of [-1, 1]) {
            const center = add3(add3(add3(base, mul3(spine, form.height * u)), mul3(across, side * form.spread * u)), mul3(facing, form.out * u));
            const r = form.radius * u;
            sphere(center, 1, { x: r, y: r * form.flat, z: r }, facing);
        }
    };
    if (M.bust) pair(M.bust, lerp3(J.hip, J.neck, M.chest.base), acrossOf(J.shoulderL, J.shoulderR), front);
    if (M.glutes) pair(M.glutes, lerp3(J.hip, J.neck, M.pelvis.base), acrossOf(J.hipL, J.hipR), mul3(front, -1));

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

