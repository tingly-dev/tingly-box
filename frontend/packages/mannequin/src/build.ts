// Changing who a figure is without changing what it is doing. The pose is the
// figure's bone *directions*; the build is its bone *lengths* (only the
// girdles differ — see `BONES`). So a build change is the same retarget the
// photograph import does: every direction kept, every length taken from the
// new skeleton, the figure left standing where it stood.
import { createFigure } from './figure';
import { retargetToBones } from './landmarks';
import { constrainFigure } from './rig';
import { figureBuild, figureTurn, figureUnit, type FigureBuild, type PoseFigure } from './skeleton';
import { mapJoints, scaleFigure } from './transform';
import { add3, sub3 } from './vec3';

export const setFigureBuild = (figure: PoseFigure, build: FigureBuild): PoseFigure => {
    if (figureBuild(figure) === build) return figure;
    // Any pose of the new build carries its lengths; scaled to this body so
    // only the girdles change, never the size.
    const fresh = createFigure('standing', { width: 1000, height: 1000 }, undefined, 0, figureTurn(figure), build);
    const reference = scaleFigure(fresh, figureUnit(figure) / figureUnit(fresh));
    const { joints } = retargetToBones(figure.joints, reference);
    // The retarget roots at the reference's hip; put it back on this one's,
    // depth included, so the figure does not jump on the canvas.
    const shift = sub3(figure.joints.hip, joints.hip);
    const placed = mapJoints({ ...figure, joints, build }, (p) => add3(p, shift));
    return constrainFigure(placed);
};
