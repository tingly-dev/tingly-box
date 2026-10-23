import i18n from '@/i18n';
import { getOpenAIClient } from '@/services/modelApi';
import { createWorkspaceCollection, type Collection } from '../imageWorkspaceDb';
import { cleanPromptReply } from './imagePromptTemplates';

// The scenario both prompt tools (Image to Prompt, Idea to Prompt) send
// through: a chat model behind a rule, on the real gateway path.
export const PROMPT_TOOLS_SCENARIO = 'imageprompt';

// What every prompt-tool run has, whatever its input (a picture, a
// description): the model and instructions it was sent with, and the prompt
// that came back, then as the user edited it.
export interface PromptRunBase {
    id: string;
    createdAt: number;
    // The rule name sent as `model` on /tingly/imageprompt.
    model: string;
    instructions: string;
    status: 'pending' | 'completed' | 'failed';
    prompt: string;
    error?: string;
}

type ChatMessage = { role: 'system' | 'user'; content: any };

// One run store per prompt tool: its own IndexedDB collection, one request
// shape. `userMessage` turns a run's own input into the user turn; the
// instructions always travel as the system turn.
export const createPromptRunStore = <Run extends PromptRunBase>(
    collection: Collection,
    seed: (() => Promise<Run[]>) | null,
    userMessage: (run: Run) => ChatMessage['content'],
) => {
    // A run stored as pending lost its request with the page that sent it.
    const runStore = createWorkspaceCollection<Run>(
        collection,
        seed,
        (run) => (run.status === 'pending'
            ? { ...run, status: 'failed', error: i18n.t('playground.interruptedByReload', { defaultValue: 'Interrupted by a page reload' }) }
            : run),
    );

    const update = (id: string, fields: Partial<Run>) => {
        runStore.set(runStore.get().map((run) => (run.id === id ? { ...run, ...fields } : run)));
    };

    // Requests belong to the tab, not to the page: leaving for the Playground
    // and coming back finds a run still running, and removing a run stops it.
    const inFlight = new Map<string, AbortController>();

    const remove = (id: string) => {
        inFlight.get(id)?.abort();
        runStore.set(runStore.get().filter((run) => run.id !== id));
    };

    const clear = () => {
        inFlight.forEach((controller) => controller.abort());
        runStore.set([]);
    };

    // One path for a new run and a retry (same id). Newest first: the list
    // reads top-down, and the run just started is the one being waited on.
    const start = async (inputs: Omit<Run, keyof PromptRunBase> & Pick<Run, 'model' | 'instructions'>, id?: string) => {
        const runId = id ?? `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
        const run = { ...inputs, id: runId, createdAt: Date.now(), status: 'pending', prompt: '' } as Run;
        runStore.set([run, ...runStore.get().filter((item) => item.id !== runId)]);
        const controller = new AbortController();
        inFlight.set(runId, controller);
        try {
            const client = await getOpenAIClient(PROMPT_TOOLS_SCENARIO);
            const messages: ChatMessage[] = [
                ...(run.instructions.trim() ? [{ role: 'system' as const, content: run.instructions }] : []),
                { role: 'user' as const, content: userMessage(run) },
            ];
            const response = await client.chat.completions.create({ model: run.model, messages }, { signal: controller.signal });
            const prompt = cleanPromptReply(response.choices?.[0]?.message?.content ?? '');
            update(runId, (prompt
                ? { status: 'completed', prompt, error: undefined }
                : { status: 'failed', error: i18n.t('imagePrompt.emptyReply', { defaultValue: 'The model answered with no text' }) }) as Partial<Run>);
        } catch (error: any) {
            if (controller.signal.aborted) return;
            const status = error?.status ? `${error.status}: ` : '';
            const message = error?.error?.message || error?.message || i18n.t('playground.requestFailed', { defaultValue: 'Request failed' });
            update(runId, { status: 'failed', error: `${status}${message}` } as Partial<Run>);
        } finally {
            inFlight.delete(runId);
        }
    };

    return { useRuns: runStore.useItems, update, remove, clear, start };
};
