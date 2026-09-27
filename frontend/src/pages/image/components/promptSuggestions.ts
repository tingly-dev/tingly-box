import type { TFunction } from 'i18next';
import { describeShot } from '@tingly/mannequin';
import type { ReferenceImage } from './ImageGenReferenceImages';

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
const poseSuggestion = (ref: ReferenceImage, index: number, t: TFunction): PromptSuggestion | null => {
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
    const n = index + 1;
    const pose = poseAs === 'skeleton'
        ? t('playground.suggest.pose.skeleton', { n })
        : t('playground.suggest.pose.mannequin', { n });
    return {
        id: `pose-${index}`,
        label: t('playground.suggest.pose.label', { n }),
        // Two sentences, joined the way the language joins them (a space in
        // English, nothing in Chinese).
        text: t('playground.suggest.pose.withCamera', { pose, camera: t('playground.suggest.shot.camera', { camera }) }),
    };
};

export const promptSuggestionsFor = (referenceImages: readonly ReferenceImage[], t: TFunction): PromptSuggestion[] =>
    referenceImages.flatMap((ref, index) => poseSuggestion(ref, index, t) ?? []);

// Appended as its own paragraph, never spliced into what is there.
export const insertSuggestion = (prompt: string, text: string): string => {
    const head = prompt.trimEnd();
    return head ? `${head}\n${text}` : text;
};
