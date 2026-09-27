import type { TFunction } from 'i18next';
import { describeShot } from '@tingly/mannequin';
import type { ReferenceImage } from './ImageGenReferenceImages';
import { effectiveRole } from './referenceRoles';
import { describeExpression } from './expression/expressionState';

// A sentence the prompt could use, offered next to it and never written into
// it. The prompt is the user's: filling it in unasked would mean every run
// carries text nobody typed, and a test of "does the model follow the
// skeleton without being told" would be impossible to run. So a suggestion is
// one click away, visible in full before it is taken, and gone once taken.
//
// The shape is deliberately generic — anything the panel knows that the model
// can't see (what a reference image *is*, what a mask means) can be offered
// the same way.
export interface PromptSuggestion {
    id: string;
    // Short, for the chip.
    label: string;
    // Exactly what gets inserted, shown in full on hover.
    text: string;
}

// The pose skeleton is flat: it cannot say from how high or through which
// lens it was seen. The figure knows both, in numbers, so the suggestion puts
// them into words — the half of the pose the picture cannot carry.
const posedSketchText = (ref: ReferenceImage, n: number, t: TFunction): string | null => {
    const figures = ref.layers?.figures ?? [];
    if (ref.source !== 'sketch' || figures.length === 0) return null;
    // A sketch saved before the choice existed was sent as the mannequin.
    const poseAs = ref.layers?.poseAs ?? 'mannequin';
    const shot = describeShot(figures[0]);
    const camera = [
        t(`playground.suggest.shot.height.${shot.height}`),
        t(`playground.suggest.shot.side.${shot.side}`),
        shot.lens && shot.lens !== 'standard' ? t(`playground.suggest.shot.lens.${shot.lens}`) : null,
    ].filter(Boolean).join(t('playground.suggest.shot.separator'));
    const pose = poseAs === 'skeleton'
        ? t('playground.suggest.pose.skeleton', { n })
        : t('playground.suggest.pose.mannequin', { n });
    // Two sentences, joined the way the language joins them (a space in
    // English, nothing in Chinese).
    return t('playground.suggest.pose.withCamera', { pose, camera: t('playground.suggest.shot.camera', { camera }) });
};

// A face from the expression dialog is a stylised anime face with no hair or
// body. Said plainly, because the one thing the model must not take from it is
// the look — and the expression in words, the half a picture can be misread on.
const expressionFaceText = (ref: ReferenceImage, n: number, t: TFunction): string | null => {
    if (ref.source !== 'expression' || !ref.expression) return null;
    const { preset, parts } = describeExpression(ref.expression);
    const expression = preset || parts.length === 0
        ? t(`playground.expression.preset.${preset ?? 'neutral'}`)
        : parts.map((part) => t(`playground.suggest.expressionWord.${part}`)).join(t('playground.suggest.shot.separator'));
    return t('playground.suggest.role.expressionFace', { n, expression });
};

// One sentence per image that has a role: what to take from it, and — the
// half that actually prevents the failure — what *not* to take. "Expression
// only" without "not the face" is how a reference photo's identity ends up
// on the character.
const roleSuggestion = (ref: ReferenceImage, index: number, t: TFunction): PromptSuggestion | null => {
    const role = effectiveRole(ref);
    if (!role) return null;
    const n = index + 1;
    const text = (role === 'pose' ? posedSketchText(ref, n, t) : null)
        ?? (role === 'expression' ? expressionFaceText(ref, n, t) : null)
        ?? t(`playground.suggest.role.${role}`, { n });
    return {
        // The role is part of the identity: changing it offers a new chip
        // rather than hiding behind the old one having been used.
        id: `${role}-${index}`,
        label: t('playground.suggest.role.label', { role: t(`playground.referenceRole.${role}`), n }),
        text,
    };
};

export const promptSuggestionsFor = (referenceImages: readonly ReferenceImage[], t: TFunction): PromptSuggestion[] =>
    referenceImages.flatMap((ref, index) => roleSuggestion(ref, index, t) ?? []);

// Appended as its own paragraph, never spliced into what is there.
export const insertSuggestion = (prompt: string, text: string): string => {
    const head = prompt.trimEnd();
    return head ? `${head}\n${text}` : text;
};
