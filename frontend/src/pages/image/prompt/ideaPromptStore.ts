import { USE_MOCK } from '../imageWorkspaceDb';
import { ideaUserMessage } from './ideaPromptTemplates';
import { createPromptRunStore, type PromptRunBase } from './promptRunStore';

// One Idea to Prompt request: the description it expanded, and the prompt
// that came back (.design/idea-to-prompt.md).
export interface IdeaPromptRun extends PromptRunBase {
    description: string;
}

export type IdeaPromptInputs = Pick<IdeaPromptRun, 'description' | 'model' | 'instructions'>;

const store = createPromptRunStore<IdeaPromptRun>(
    'ideaPrompts',
    USE_MOCK ? () => import('./mockIdeaPrompts').then((module) => module.mockIdeaPromptRuns) : null,
    (run) => ideaUserMessage(run.description),
);

export const useIdeaPromptRuns = store.useRuns;
export const updateIdeaPromptRun = store.update;
export const removeIdeaPromptRun = store.remove;
export const clearIdeaPromptRuns = store.clear;
export const startIdeaPromptRun = (inputs: IdeaPromptInputs, id?: string) => store.start(inputs, id);
