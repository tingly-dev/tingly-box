// The figure as a model reads a pose: an OpenPose skeleton.
//
// The shaded manikin is for the person posing it — volume, occlusion and light
// are what make a 3D pose legible to a human. A model reads the same pixels as
// *a body*: a smooth grey person the prompt then has to dress, and one it
// often simply keeps. The OpenPose skeleton (COCO-18 keypoints, the fixed
// colours every ControlNet-era tool draws) is the one pose picture image
// models have learned as "a pose diagram, not content". Same joints, drawn
// for a different reader.
import { lensOf, projectionOf, projectPoint, type LensKey, type ProjectedPoint } from './camera';
import { bodyForwardOf, figureTurn, figureUnit, type PoseFigure } from './skeleton';
import { add3, cross3, dot3, mul3, norm3, sub3, type Vec3 } from './vec3';
import { cameraHeightOf } from './view';

// COCO-18, in OpenPose's order. R/L are the *subject's* sides.
export const OPENPOSE_KEYPOINTS = [
    'nose', 'neck',
    'shoulderR', 'elbowR', 'wristR',
    'shoulderL', 'elbowL', 'wristL',
    'hipR', 'kneeR', 'ankleR',
    'hipL', 'kneeL', 'ankleL',
    'eyeR', 'eyeL', 'earR', 'earL',
] as const;

// Limb i is drawn in colour i; keypoint i in colour i. These are the exact
// values of the reference renderer — a model that has seen OpenPose maps has
// seen these colours, and the colour is how it tells left from right.
const COLORS: readonly (readonly [number, number, number])[] = [
    [255, 0, 0], [255, 85, 0], [255, 170, 0], [255, 255, 0], [170, 255, 0], [85, 255, 0],
    [0, 255, 0], [0, 255, 85], [0, 255, 170], [0, 255, 255], [0, 170, 255], [0, 85, 255],
    [0, 0, 255], [85, 0, 255], [170, 0, 255], [255, 0, 255], [255, 0, 170], [255, 0, 85],
];

const LIMBS: readonly (readonly [number, number])[] = [
    [1, 2], [1, 5], [2, 3], [3, 4], [5, 6], [6, 7],
    [1, 8], [8, 9], [9, 10], [1, 11], [11, 12], [12, 13],
    [1, 0], [0, 14], [14, 16], [0, 15], [15, 17],
];

// Where the face features sit on the skull, in figure units. Only as precise
// as a keypoint detector is — which is the precision the model was trained on.
const FACE = { nose: 0.06, eyeForward: 0.045, eyeUp: 0.018, eyeSide: 0.024, ear: 0.058 };

// The camera looks down −z; +z is toward the lens.
const TO_CAMERA: Vec3 = { x: 0, y: 0, z: 1 };

// Keypoints in canvas pixels, or null where a detector would not have found
// one. That absence is information, not a gap: a skeleton with no nose and no
// eyes is how an OpenPose map says "seen from behind", and one eye and one ear
// is a profile. Drawing them anyway would turn every back view into a front
// view.
export const openposeKeypoints = (figure: PoseFigure): (ProjectedPoint | null)[] => {
    const j = figure.joints;
    const unit = figureUnit(figure);
    const up = norm3(sub3(j.head, j.neck));
    const aim = norm3(sub3(j.face, j.head));
    const forward = norm3(sub3(aim, mul3(up, dot3(aim, up))));
    // Our joints are named for where they sit on a figure facing the viewer,
    // so our `shoulderL` is the subject's right (see landmarks.ts). The head's
    // right is fixed against the body's rather than trusted to a cross
    // product's handedness — canvas y points down and flips it.
    const bodyRight = sub3(j.shoulderL, j.shoulderR);
    const handed = Math.sign(dot3(cross3(norm3(sub3(j.neck, j.hip)), bodyForwardOf(j)), bodyRight)) || 1;
    const right = mul3(norm3(cross3(up, forward)), handed);

    const facing = dot3(forward, TO_CAMERA);
    const at = (...parts: Vec3[]) => parts.reduce(add3, j.head);
    const seen = (normal: Vec3) => dot3(norm3(normal), TO_CAMERA) > -0.25;
    const eye = (side: 1 | -1) => at(mul3(forward, unit * FACE.eyeForward), mul3(up, unit * FACE.eyeUp), mul3(right, side * unit * FACE.eyeSide));
    const eyeSeen = (side: 1 | -1) => facing > -0.1 && seen(add3(forward, mul3(right, side * 0.6)));

    const points: (Vec3 | null)[] = [
        facing > -0.2 ? at(mul3(forward, unit * FACE.nose)) : null,
        j.neck,
        j.shoulderL, j.elbowL, j.wristL,
        j.shoulderR, j.elbowR, j.wristR,
        j.hipL, j.kneeL, j.ankleL,
        j.hipR, j.kneeR, j.ankleR,
        eyeSeen(1) ? eye(1) : null,
        eyeSeen(-1) ? eye(-1) : null,
        seen(right) ? at(mul3(right, unit * FACE.ear)) : null,
        seen(mul3(right, -1)) ? at(mul3(right, -unit * FACE.ear)) : null,
    ];
    const projection = projectionOf(figure);
    return points.map((point) => (point ? projectPoint(point, projection) : null));
};

export const OPENPOSE_BACKGROUND = '#000000';

// The reference renderer's stick is 4px on a ~512px image with the person
// most of its height: about 1% of the body. Tied to the body rather than the
// canvas so a small figure in a wide shot is not drawn in marker pen.
const STICK_RATIO = 0.011;

export const drawOpenpose = (ctx: CanvasRenderingContext2D, figure: PoseFigure): void => {
    const points = openposeKeypoints(figure);
    const stick = Math.max(figureUnit(figure) * STICK_RATIO, 1.5);
    ctx.save();
    LIMBS.forEach(([a, b], i) => {
        const p = points[a];
        const q = points[b];
        if (!p || !q) return;
        const [r, g, bl] = COLORS[i];
        ctx.globalAlpha = 0.6;
        ctx.fillStyle = `rgb(${r},${g},${bl})`;
        ctx.beginPath();
        ctx.ellipse(
            (p.x + q.x) / 2, (p.y + q.y) / 2,
            Math.hypot(q.x - p.x, q.y - p.y) / 2, stick,
            Math.atan2(q.y - p.y, q.x - p.x), 0, Math.PI * 2,
        );
        ctx.fill();
    });
    ctx.globalAlpha = 1;
    points.forEach((p, i) => {
        if (!p) return;
        const [r, g, b] = COLORS[i];
        ctx.fillStyle = `rgb(${r},${g},${b})`;
        ctx.beginPath();
        ctx.arc(p.x, p.y, stick, 0, Math.PI * 2);
        ctx.fill();
    });
    ctx.restore();
};


// --- the camera, in words ----------------------------------------------------
//
// A skeleton is flat: it carries no depth, so it cannot say whether it was
// seen from above or below, or through which lens. Those are exactly the
// numbers the figure already has, and the words a prompt uses for them. The
// buckets are photographers' names, and the boundaries sit between the grid's
// rows and columns so every grid cell lands in its own word.
export type ShotHeight = 'overhead' | 'high' | 'eye' | 'low' | 'worm';
export type ShotSide = 'front' | 'threeQuarter' | 'side' | 'threeQuarterBack' | 'back';

export interface ShotDescription { height: ShotHeight; side: ShotSide; lens: LensKey | null }

export const describeShot = (figure: PoseFigure): ShotDescription => {
    const turn = figureTurn(figure);
    const h = cameraHeightOf(turn);
    const height: ShotHeight = h > 52 ? 'overhead' : h > 15 ? 'high' : h >= -15 ? 'eye' : h >= -45 ? 'low' : 'worm';
    const yaw = Math.abs(turn.yaw);
    const side: ShotSide = yaw < 20 ? 'front' : yaw < 67.5 ? 'threeQuarter' : yaw < 112.5 ? 'side' : yaw < 160 ? 'threeQuarterBack' : 'back';
    return { height, side, lens: lensOf(figure) };
};
