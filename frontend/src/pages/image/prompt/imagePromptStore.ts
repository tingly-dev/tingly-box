import i18n from '@/i18n';
import { getOpenAIClient } from '@/services/modelApi';
import { USE_MOCK, createWorkspaceCollection } from '../imageWorkspaceDb';
import { USER_REQUEST, cleanPromptReply } from './imagePromptTemplates';

export const IMAGE_PROMPT_SCENARIO = 'imageprompt';

// One Image to Prompt request and what came back. The picture is kept as the
// data URL that was sent, so a run can be re-run and handed on after a reload.
export interface ImagePromptRun {
    id: string;
    createdAt: number;
    image: string;
    imageName: string;
    // The rule name sent as `model` on /tingly/imageprompt.
    model: string;
    instructions: string;
    status: 'pending' | 'completed' | 'failed';
    // The prompt as the model wrote it, then as the user edited it.
    prompt: string;
    error?: string;
}

export type ImagePromptInputs = Pick<ImagePromptRun, 'image' | 'imageName' | 'model' | 'instructions'>;

// Browser-local like the rest of the image workspace (imageWorkspaceDb.ts).
// A run stored as pending lost its request with the page that sent it.
const runStore = createWorkspaceCollection<ImagePromptRun>(
    'imagePrompts',
    USE_MOCK ? () => import('./mockImagePrompts').then((module) => module.mockImagePromptRuns) : null,
    (run) => (run.status === 'pending'
        ? { ...run, status: 'failed', error: i18n.t('playground.interruptedByReload', { defaultValue: 'Interrupted by a page reload' }) }
        : run),
);

export const useImagePromptRuns = runStore.useItems;

export const updateImagePromptRun = (id: string, fields: Partial<ImagePromptRun>) => {
    runStore.set(runStore.get().map((run) => (run.id === id ? { ...run, ...fields } : run)));
};

// Requests belong to the tab, not to the page: leaving for the Playground and
// coming back finds a run still running, and removing a run stops its request.
const inFlight = new Map<string, AbortController>();

export const removeImagePromptRun = (id: string) => {
    inFlight.get(id)?.abort();
    runStore.set(runStore.get().filter((run) => run.id !== id));
};

export const clearImagePromptRuns = () => {
    inFlight.forEach((controller) => controller.abort());
    runStore.set([]);
};

// One path for a new run and a retry (same id). Newest first: the list reads
// top-down, and the run just started is the one being waited on.
export const startImagePromptRun = async (inputs: ImagePromptInputs, id?: string) => {
    const runId = id ?? `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const run: ImagePromptRun = { ...inputs, id: runId, createdAt: Date.now(), status: 'pending', prompt: '' };
    runStore.set([run, ...runStore.get().filter((item) => item.id !== runId)]);
    const controller = new AbortController();
    inFlight.set(runId, controller);
    try {
        const client = await getOpenAIClient(IMAGE_PROMPT_SCENARIO);
        const response = await client.chat.completions.create({
            model: inputs.model,
            messages: [
                ...(inputs.instructions.trim() ? [{ role: 'system' as const, content: inputs.instructions }] : []),
                {
                    role: 'user' as const,
                    content: [
                        { type: 'image_url' as const, image_url: { url: inputs.image } },
                        { type: 'text' as const, text: USER_REQUEST },
                    ],
                },
            ],
        }, { signal: controller.signal });
        const prompt = cleanPromptReply(response.choices?.[0]?.message?.content ?? '');
        updateImagePromptRun(runId, prompt
            ? { status: 'completed', prompt, error: undefined }
            : { status: 'failed', error: i18n.t('imagePrompt.emptyReply', { defaultValue: 'The model answered with no text' }) });
    } catch (error: any) {
        if (controller.signal.aborted) return;
        const status = error?.status ? `${error.status}: ` : '';
        const message = error?.error?.message || error?.message || i18n.t('playground.requestFailed', { defaultValue: 'Request failed' });
        updateImagePromptRun(runId, { status: 'failed', error: `${status}${message}` });
    } finally {
        inFlight.delete(runId);
    }
};
