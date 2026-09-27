import type { SketchLayers } from './SketchCanvasDialog';

// What a reference image is *for*. With more than one image the model has to
// guess which one is the character and which one only lends an expression,
// and when it guesses wrong it copies a face, an outfit or a background it
// was never meant to. The role is how the user says it; the model hears it
// only through a sentence offered beside the prompt (promptSuggestions.ts),
// never through anything written in behind the user's back.
//
// An attribute of the image, like its mask: it travels with the image when
// the row is reordered and goes when the image goes.
export type ReferenceRole = 'character' | 'expression' | 'pose' | 'outfit' | 'style' | 'scene';

// Menu order: who, then what they do, then what they wear, then how it looks.
export const REFERENCE_ROLES: readonly ReferenceRole[] = ['character', 'expression', 'pose', 'outfit', 'style', 'scene'];

interface RoleCarrier {
    source: 'upload' | 'sketch' | 'expression';
    layers?: SketchLayers;
    // `undefined`: never set, so the default applies. `null`: the user took
    // the role off on purpose, and a default must not quietly put it back.
    role?: ReferenceRole | null;
}

// A posed sketch is a pose reference without anyone having to say so — that
// is the only thing a mannequin or a skeleton can be — and a face made in the
// expression dialog is an expression reference, for the same reason. Everything else starts
// with no role: guessing "character" for a photo would be wrong as often as
// right, and a wrong label is worse than none.
export const effectiveRole = (ref: RoleCarrier): ReferenceRole | null => {
    if (ref.role !== undefined) return ref.role;
    if (ref.source === 'expression') return 'expression';
    return ref.source === 'sketch' && (ref.layers?.figures.length ?? 0) > 0 ? 'pose' : null;
};
