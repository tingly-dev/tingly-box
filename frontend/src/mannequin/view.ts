import { projectionOf, rotateView } from './camera';
import { JOINT_KEYS, figureBounds, figureCenter, centerFigureAt, figureTurn, mapJoints, type FigureTurn, type JointKey, type PoseFigure } from './internal';
import { add3, rad, sub3, zOf, type Vec3 } from './vec3';

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
