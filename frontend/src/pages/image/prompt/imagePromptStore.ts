import { USE_MOCK } from '../imageWorkspaceDb';
import { USER_REQUEST } from './imagePromptTemplates';
import { createPromptRunStore, type PromptRunBase } from './promptRunStore';

// One Image to Prompt request and what came back. The picture is kept as the
// data URL that was sent, so a run can be re-run and handed on after a reload.
export interface ImagePromptRun extends PromptRunBase {
    image: string;
    imageName: string;
}

// Browser-local like the rest of the image workspace (imageWorkspaceDb.ts).
const store = createPromptRunStore<ImagePromptRun>(
    'imagePrompts',
    USE_MOCK ? () => import('./mockImagePrompts').then((module) => module.mockImagePromptRuns) : null,
    (run) => [
        { type: 'image_url', image_url: { url: run.image } },
        { type: 'text', text: USER_REQUEST },
    ],
);

export const useImagePromptRuns = store.useRuns;
export const updateImagePromptRun = store.update;
export const removeImagePromptRun = store.remove;
export const clearImagePromptRuns = store.clear;
export const startImagePromptRun = store.start;
