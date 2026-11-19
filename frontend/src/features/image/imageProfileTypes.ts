// Type-only: a profile stores reference images in the playground's shape, which is defined next to the
// component that edits them. No runtime edge, so the nav can import this module without the page.
// oxlint-disable-next-line no-restricted-imports
import type { ReferenceImage } from '@/pages/image/components/ImageGenReferenceImages';
import type { ImageReasoningEffort, Quality } from './ImageGenPlayground.types';

// One of a profile's saved prompts. The profile page has a single prompt
// field; these are what it switches between.
export interface ProfilePrompt {
    id: string;
    // '' = not named yet: the tab shows "Prompt N" (by position).
    name: string;
    text: string;
}

// An image profile pins one piece of ongoing work: the reference images
// (picked by the user, within the run's limit), the prompts used with them,
// and the request settings. Its page is the playground with all of that
// already in place.
// See .design/image-profile.md.
//
// Kept in the browser for now (imageProfileStore.ts → imageWorkspaceDb.ts);
// no backend yet.
export interface ImageProfile {
    id: string;
    name: string;
    refs: ReferenceImage[];
    prompts: ProfilePrompt[];
    // The prompt the field shows when the page opens — the last one used.
    activePromptId: string;
    // '' = the first available image model.
    model: string;
    size: string;
    quality: Quality;
    reasoningEffort?: ImageReasoningEffort;
    count: number;
    updatedAt: number;
}

export const newPromptId = () => `p-${Math.random().toString(36).slice(2, 8)}`;
