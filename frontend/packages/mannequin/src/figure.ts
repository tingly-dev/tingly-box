// Making figures: from a preset, from a saved sketch, or by swapping the pose
// of one that exists. The one place the library, the rig and the view meet.
import { JOINT_KEYS, figureTurn, figureUnit, derivedFace, type FigureTurn, type JointKey, type PoseFigure } from './skeleton';
import { constrainFigure } from './rig';
import { DEFAULT_VIEW, setFigureTurn } from './view';
import { centerFigureAt, figureCenter, fitFigureInto, mapJoints, scaleFigure } from './transform';
import { FIGURE_ASPECT } from './poses/spec';
import { POSE_PRESETS, type PosePresetKey } from './poses/library';
import type { Point, Size } from './types';
import { zOf, type Vec3 } from './vec3';

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


// A figure lands at 70% of the canvas height, centred. Big enough to read as
// the subject, small enough to leave room for the scene around it.
const FIGURE_HEIGHT_RATIO = 0.7;

let figureCounter = 0;

export const createFigure = (
    preset: PosePresetKey,
    dims: Size,
    center?: Point,
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


// Swaps the pose while keeping the figure where it is, roughly how big it is,
// and which way it is being looked at: re-entry (principle 10) applies inside
// the dialog too, and the camera is not part of the pose (principle 4) — it
// would be a nasty surprise for picking "sitting" to also spin the model round.
export const applyPreset = (figure: PoseFigure, preset: PosePresetKey, dims: Size): PoseFigure => {
    const fresh = createFigure(preset, dims, figureCenter(figure), figure.shade, figureTurn(figure));
    // Matched on `figureUnit`, the body, not on the bounding box: box height
    // changes with the pose (a crouch is shorter than a stand), so matching
    // boxes would resize the person every time the pose changed.
    return { ...scaleFigure(fresh, figureUnit(figure) / figureUnit(fresh)), id: figure.id };
};


// Thumbnails: fitted to the tile, but never *inflated* past the size a
// standing figure gets in the same tile. Every thickness scales with the body
// (`figureUnit`), so letting a crouch grow until its small box fills the tile
// makes it 40% thicker than the stand beside it — and a grid where the manikin
// changes build from cell to cell reads as thirty different people rather than
// as one person in thirty poses.
export const fitFigureIntoTile = (figure: PoseFigure, box: Size, pad = 0): PoseFigure => {
    const fitted = fitFigureInto(figure, box, pad);
    const cap = figureUnit(fitFigureInto(createFigure('standing', box, undefined, 0, figureTurn(figure)), box, pad));
    const unit = figureUnit(fitted);
    if (unit <= cap) return fitted;
    const center = figureCenter(fitted);
    return centerFigureAt(scaleFigure(fitted, cap / unit), center);
};

// Used when a saved sketch is re-opened on a differently sized canvas. Goes
// through the same uniform transform as the strokes, so the drawing and the
// figures standing in it stay in register. Depth rides the same scale — it is
// measured in canvas pixels like everything else, and leaving it behind would
// flatten a reopened sketch.
// Also the door every foreign figure comes through, so it is where a sketch
// saved before the face joint existed gets one — even when the transform
// itself is a no-op.
// The shape of a uniform canvas transform, kept structural so the library
// never imports the sketch canvas: the dialog hands in its own transform.
export interface PlanarTransform { scale: number; dx: number; dy: number }

export const transformFigures = (
    figures: readonly PoseFigure[],
    transform: PlanarTransform,
): PoseFigure[] => figures.map((figure) => {
    const whole = completeFigure(figure);
    const identity = transform.scale === 1 && transform.dx === 0 && transform.dy === 0;
    return identity ? whole : mapJoints(whole, (p) => ({
        x: p.x * transform.scale + transform.dx,
        y: p.y * transform.scale + transform.dy,
        z: zOf(p) * transform.scale,
    }));
});

