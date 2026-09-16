// Editing gestures: dragging joints, grabbing bodies, the handles and where a
// new figure lands. Everything here is in screen space and goes through the
// projection, so what the pointer touches is what the eye sees.
import { projectFigure, projectionOf, projectPoint, unprojectPoint } from './camera';
import { BONE, JOINT_KEYS, JOINT_PARENT, SUBTREES, figureUnit, type JointKey, type PoseFigure } from './skeleton';
import { constrainFigure } from './rig';
import { figureCenter, figureVisualBounds, translateFigure } from './transform';
import { figureSolids } from './body';
import { add3, cross3, dist3, dot3, len3, mul3, norm3, rotateAxis, sub3, zOf, type Vec3 } from './vec3';
import type { Point, Size } from './types';

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
    target: Point,
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


// The escape hatch is an escape from the *skeleton*, not from the rig: Alt lets
// a joint leave its bone length behind, because odd proportions are a drawing
// choice, but it still may not put a knee on backwards.
export const moveJoint = (figure: PoseFigure, key: JointKey, point: Point): PoseFigure => constrainFigure({
    ...figure,
    joints: {
        ...figure.joints,
        [key]: unprojectPoint(point, zOf(figure.joints[key]), projectionOf(figure)),
    },
});


// --- hit testing -------------------------------------------------------------

// Where the point projects onto the segment (`t`), and how far it is from it.
// A tapered limb needs both: its half-width at the point of closest approach
// is the radius interpolated at that same `t`.
const projectOnSegment = (point: Point, a: Point, b: Point): { t: number; distance: number } => {
    const vx = b.x - a.x;
    const vy = b.y - a.y;
    const lengthSq = vx * vx + vy * vy;
    if (lengthSq === 0) return { t: 0, distance: Math.hypot(point.x - a.x, point.y - a.y) };
    const t = Math.max(0, Math.min(1, ((point.x - a.x) * vx + (point.y - a.y) * vy) / lengthSq));
    return { t, distance: Math.hypot(point.x - (a.x + vx * t), point.y - (a.y + vy * t)) };
};

export const distanceToSegment = (point: Point, a: Point, b: Point): number =>
    projectOnSegment(point, a, b).distance;

// A tapered capsule on screen: the thing a projected bone is.
export interface Segment { from: Point; to: Point; fromRadius: number; toRadius: number }

const insideSegment = (point: Point, segment: Segment, tolerance: number): boolean => {
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
): Point => {
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
    pointer: Point,
    handleRadius: number,
): Point => {
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
    point: Point,
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
export const hitTestBody = (figure: PoseFigure, point: Point, tolerance = 0): boolean => {
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
export const scaleHandlePoint = (figure: PoseFigure): Point => {
    const bounds = figureVisualBounds(figure);
    return { x: bounds.x + bounds.width, y: bounds.y + bounds.height };
};

export const isScaleHandleHit = (figure: PoseFigure, point: Point, radius: number): boolean => {
    const handle = scaleHandlePoint(figure);
    return Math.hypot(point.x - handle.x, point.y - handle.y) <= radius;
};

// Bottom-left, opposite the scale grip: turning the figure is the other thing
// you do to the whole body, and it gets the same kind of control rather than a
// mode to enter. Drag sideways to spin the body, up and down to raise or drop
// the camera.
export const turnHandlePoint = (figure: PoseFigure): Point => {
    const bounds = figureVisualBounds(figure);
    return { x: bounds.x, y: bounds.y + bounds.height };
};

export const isTurnHandleHit = (figure: PoseFigure, point: Point, radius: number): boolean => {
    const handle = turnHandlePoint(figure);
    return Math.hypot(point.x - handle.x, point.y - handle.y) <= radius;
};

// How far the body turns per pixel dragged. A full turn in rather less than a
// canvas width: the useful range is the first 45°, and a slow handle makes
// finding it a chore.
export const TURN_DEGREES_PER_PIXEL = 0.55;


// Where the next figure should land. Dropping every one at the canvas centre
// stacks them exactly on top of each other, which looks like nothing happened
// and leaves the buried figure unreachable. Candidates walk outward from the
// centre; the first one clear of the figures already placed wins, and when a
// crowd has taken them all the figure cascades so it is at least grabbable.
const PLACEMENT_CANDIDATES: readonly (readonly [number, number])[] = [
    [0.5, 0.5], [0.26, 0.5], [0.74, 0.5], [0.38, 0.44], [0.62, 0.56],
    [0.14, 0.46], [0.86, 0.54], [0.5, 0.38], [0.5, 0.62],
];

export const placeNewFigure = (existing: readonly PoseFigure[], dims: Size): Point => {
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
    point: Point,
    tolerance = 0,
): PoseFigure[] => figures.filter((figure) => hitTestBody(figure, point, tolerance));

// Clicking a pile of overlapping figures walks down it instead of always
// returning the top one, which would leave anything underneath unreachable.
export const nextFigureAt = (
    figures: readonly PoseFigure[],
    point: Point,
    selectedId: string | null,
    tolerance = 0,
): PoseFigure | null => {
    const hits = figuresAt(figures, point, tolerance);
    if (hits.length === 0) return null;
    const index = hits.findIndex((figure) => figure.id === selectedId);
    if (index < 0) return hits[hits.length - 1];
    return hits[(index + hits.length - 1) % hits.length];
};


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
